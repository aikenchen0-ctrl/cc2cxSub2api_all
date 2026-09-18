const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const { execFileSync } = require('node:child_process');
const { randomUUID, createHmac, createHash } = require('node:crypto');
const { join } = require('node:path');
const { mkdirSync } = require('node:fs');
const assert = require('node:assert/strict');
const output=process.env.SSO_TEST_OUTPUT||join(require('node:os').tmpdir(),'ppt-deployed-smoke');
const subject='deployment-smoke:'+randomUUID();
const nonce=randomUUID();
const username='sub2api-'+createHash('sha256').update(subject).digest('hex');
function docker(args) {return execFileSync('docker',args,{encoding:'utf8',stdio:['ignore','pipe','pipe']}).trim();}
let browser,context,presentationId;
(async()=>{
 const config=JSON.parse(docker(['inspect','ppt-production-1']))[0];
 const secret=config.Config.Env.find(v=>v.startsWith('SUB2API_SSO_SECRET=')).slice('SUB2API_SSO_SECRET='.length);
 const portal=JSON.parse(docker(['inspect','sub2api']))[0];
 assert.ok(portal.Config.Env.includes('SUB2API_SSO_SECRET='+secret));
 assert.ok(portal.Config.Env.includes('PPT_SSO_CALLBACK_URL=http://localhost:8341/api/v1/auth/sso/callback'));
 const now=Math.floor(Date.now()/1000);
 const body=Buffer.from(JSON.stringify({aud:'presenton',sub:subject,jti:nonce,iat:now,exp:now+120,next:'/upload'})).toString('base64url');
 const ticket=body+'.'+createHmac('sha256',secret).update(body).digest('base64url');
 browser=await chromium.launch({headless:true});
 context=await browser.newContext({viewport:{width:1440,height:1000}});
 const page=await context.newPage();
 const errors=[];
 page.on('pageerror',e=>errors.push(e.message));
 await page.goto('http://localhost:8341/api/v1/auth/sso/callback?ticket='+ticket,{waitUntil:'domcontentloaded'});
 await page.waitForURL('http://localhost:8341/upload');
 await page.getByRole('button',{name:'Generate presentation',exact:true}).waitFor({timeout:60000});
 const status=await (await context.request.get('http://localhost:8341/api/v1/auth/status')).json();
 assert.equal(status.role,'user');
 const runtime=await (await context.request.get('http://localhost:8341/api/runtime-config')).json();
 assert.equal(runtime.configured,true);
 assert.equal(runtime.config.CUSTOM_LLM_API_KEY,'__configured__');
 assert.equal(runtime.config.CUSTOM_LLM_URL,'http://host.docker.internal:18080/v1');
 assert.equal((await context.request.get('http://localhost:8341/api/user-config')).status(),403);
 await page.reload({waitUntil:'domcontentloaded'});
 await page.getByRole('button',{name:'Generate presentation',exact:true}).waitFor();
 mkdirSync(output,{recursive:true});
 await page.screenshot({path:join(output,'deployed-ppt-desktop.png'),fullPage:true});
 await page.setViewportSize({width:390,height:844});
 await page.screenshot({path:join(output,'deployed-ppt-mobile.png'),fullPage:true});
 assert.equal(errors.length,0,errors.join('\n'));
 console.log('PASS deployed callback, shared secret, ordinary session, masked relay settings, reload and desktop/mobile');
 const create=await context.request.post('http://localhost:8341/api/v1/ppt/presentation/create',{data:{content:'Create a one-slide outline about teamwork.',n_slides:1,language:'English'}});
 assert.equal(create.status(),200);
 presentationId=(await create.json()).id;
 const response=await context.request.get('http://localhost:8341/api/v1/ppt/outlines/stream/'+presentationId,{timeout:120000});
 const text=await response.text();
 console.log('OUTLINE_HTTP',response.status());
 const events=text.split('\n').filter(l=>l.startsWith('data:')).map(l=>{try{return JSON.parse(l.slice(5))}catch{return null}}).filter(Boolean);
 console.log('OUTLINE_EVENTS',JSON.stringify(events).slice(0,2500));
 if(!response.ok()||events.some(event=>event.type==='error')) {
  console.error('BLOCKED real outline generation: relay/upstream returned an error');
  process.exitCode=2;
 } else {
  assert.ok(events.some(event=>event.type==='complete'),'Outline completion event missing');
  console.log('PASS real outline generation');
 }
 console.log('ARTIFACTS',output);
})().catch(error=>{console.error('FAILED',String(error.message).replace(/ticket=[^\s]+/g,'ticket=[redacted]'));process.exitCode=1;}).finally(async()=>{
 if(presentationId&&context) {const result=await context.request.delete('http://localhost:8341/api/v1/ppt/presentation/'+presentationId);assert.equal(result.status(),204);}
 if(browser) await browser.close();
 // Delete only this run's synthetic ordinary identity and nonce, never real accounts.
 const cleanup="import sqlite3,uuid,hashlib; c=sqlite3.connect('/app_data/fastapi.db'); c.execute('PRAGMA foreign_keys=ON'); ns=uuid.UUID('ae47cb68-42e0-44cd-9572-e905c6684da9'); ids=[uuid.uuid5(ns,'identity:'+"+JSON.stringify(subject)+").hex,uuid.uuid5(ns,'nonce:'+hashlib.sha256("+JSON.stringify(nonce)+".encode()).hexdigest()).hex]; c.execute('DELETE FROM keyvaluesqlmodel WHERE id IN (?,?)',ids); c.execute('DELETE FROM user WHERE username=? AND is_superuser=0',("+JSON.stringify(username)+",)); c.commit(); print('PASS synthetic smoke identity cleaned')";
 console.log(docker(['exec','ppt-production-1','/opt/venv/bin/python','-c',cleanup]));
});
