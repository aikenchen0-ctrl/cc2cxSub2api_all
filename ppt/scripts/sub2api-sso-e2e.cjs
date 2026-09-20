const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const { randomBytes, createHmac } = require('node:crypto');
const { execFileSync, spawnSync } = require('node:child_process');
const { mkdirSync } = require('node:fs');
const { join } = require('node:path');
const assert = require('node:assert/strict');
const id = 'ppt-sso-e2e-' + randomBytes(4).toString('hex');
const secret = randomBytes(32).toString('hex');
const password = randomBytes(24).toString('hex');
const output = process.env.SSO_TEST_OUTPUT || join(require('node:os').tmpdir(), id);
const created = [];
let browser, popup, networkCreated = false;
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
function docker(args) { return execFileSync('docker', args, { encoding:'utf8', stdio:['ignore','pipe','pipe'] }).trim(); }
function run(suffix,args) { const name=id+'-'+suffix; docker(['run','-d','--name',name,'--network',id,...args]); created.push(name); return name; }
function baseFor(name,port) { const info=JSON.parse(docker(['inspect',name]))[0]; return 'http://127.0.0.1:'+info.NetworkSettings.Ports[port+'/tcp'][0].HostPort; }
async function ready(url) { for(let i=0;i<300;i++) { try { if((await fetch(url,{signal:AbortSignal.timeout(2000)})).ok) return; } catch {} await sleep(1000); } throw Error('Service not ready: '+new URL(url).pathname); }
function ticket(sub) {
 const now=Math.floor(Date.now()/1000);
 const body=Buffer.from(JSON.stringify({aud:'presenton',sub,jti:randomBytes(18).toString('hex'),iat:now,exp:now+120,next:'/upload'})).toString('base64url');
 return body+'.'+createHmac('sha256',secret).update(body).digest('base64url');
}
(async()=>{
 mkdirSync(output,{recursive:true});
 docker(['network','create',id]); networkCreated=true;
 const db=run('db',['--network-alias','db','-e','POSTGRES_PASSWORD='+password,'-e','POSTGRES_DB=sub2api','postgres:18-alpine']);
 run('redis',['--network-alias','redis','redis:8-alpine']);
 const ppt=run('ppt',['-p','127.0.0.1::80','-e','SUB2API_SSO_SECRET='+secret,'-e','AUTH_USERNAME=administrator','-e','AUTH_PASSWORD='+password,'-e','SUB2API_APP_CREDENTIAL=test-app-credential','-e','SUB2API_BASE_URL=http://host.docker.internal:18080','-e','SUB2API_MODEL=gpt-5.5','-e','DISABLE_ANONYMOUS_TRACKING=true','ppt:sub2api-integration']);
 const pptBase=baseFor(ppt,80);
 await ready(pptBase+'/api/v1/auth/status');
 const portal=run('portal',['-p','127.0.0.1::8080','-e','AUTO_SETUP=true','-e','DATABASE_HOST=db','-e','DATABASE_USER=postgres','-e','DATABASE_PASSWORD='+password,'-e','DATABASE_DBNAME=sub2api','-e','DATABASE_SSLMODE=disable','-e','REDIS_HOST=redis','-e','REDIS_MIN_IDLE_CONNS=1','-e','REDIS_POOL_SIZE=10','-e','ADMIN_EMAIL=portal-test@example.invalid','-e','ADMIN_PASSWORD='+password,'-e','JWT_SECRET='+randomBytes(32).toString('hex'),'-e','SUB2API_SSO_SECRET='+secret,'-e','PPT_SSO_CALLBACK_URL='+pptBase+'/api/v1/auth/sso/callback','-e','SERVER_HOST=0.0.0.0','-e','SERVER_PORT=8080','-e','SERVER_MODE=release','sub2api:ppt-integration']);
 const portalBase=baseFor(portal,8080);
 await ready(portalBase+'/health');
 console.log('PASS isolated portal and PPT ready');
 docker(['exec',db,'psql','-U','postgres','-d','sub2api','-v','ON_ERROR_STOP=1','-c',"INSERT INTO users (email,password_hash,role,status) SELECT 'ppt-user@example.invalid',password_hash,'user','active' FROM users WHERE email='portal-test@example.invalid';"]);
 browser=await chromium.launch({headless:true});
 const context=await browser.newContext({viewport:{width:1440,height:1000},locale:'zh-CN'});
 const errors=[];
 const page=await context.newPage();
 page.on('pageerror',e=>errors.push(e.message));
 await page.goto(portalBase+'/login',{waitUntil:'domcontentloaded'});
 await page.locator('#email').fill('ppt-user@example.invalid');
 await page.locator('#password').fill(password);
 const loginResponse=page.waitForResponse(r=>r.url().endsWith('/api/v1/auth/login')&&r.request().method()==='POST');
 await page.locator('button[type="submit"]').click();
 assert.equal((await loginResponse).status(),200);
 const link=page.locator('a[href*="/auth/integrations/ppt/start"]').first();
 await link.waitFor({state:'visible'});
 const signedResponse=page.waitForResponse(r=>r.url().includes('/auth/integrations/ppt/start'));
 const popupPromise=page.waitForEvent('popup');
 await link.click();
 popup=await popupPromise;
 popup.on('pageerror',e=>errors.push(e.message));
 const signed=await (await signedResponse).json();
 assert.equal(signed.code,0);
 const redirect=new URL(signed.data.redirect_url);
 assert.equal(redirect.origin,pptBase);
 assert.deepEqual([...redirect.searchParams.keys()],['ticket']);
 await popup.waitForURL(pptBase+'/upload',{timeout:60000});
 await popup.getByRole('button',{name:'Generate presentation',exact:true}).waitFor({timeout:60000});
 assert.equal(await popup.evaluate(()=>window.opener===null),true);
 const status=await (await context.request.get(pptBase+'/api/v1/auth/status')).json();
 assert.equal(status.authenticated,true); assert.equal(status.role,'user');
 const runtime=await (await context.request.get(pptBase+'/api/runtime-config')).json();
 assert.equal(runtime.configured,true);
 assert.equal(runtime.config.LLM,'custom');
 assert.equal(runtime.config.CUSTOM_LLM_URL,'http://host.docker.internal:18080/v1');
 assert.equal(runtime.config.CUSTOM_LLM_API_KEY,'__configured__');
 assert.ok(!JSON.stringify(runtime).includes('test-server-only-key'));
 assert.equal((await context.request.get(pptBase+'/api/user-config')).status(),403);
 assert.equal((await context.request.get(pptBase+'/api/v1/admin/provider-settings')).status(),403);
 await popup.screenshot({path:join(output,'ppt-desktop.png'),fullPage:true});
 await popup.reload({waitUntil:'domcontentloaded'});
 await popup.getByRole('button',{name:'Generate presentation',exact:true}).waitFor();
 assert.equal(await popup.getByText('Could not verify provider settings',{exact:true}).count(),0);
 await popup.setViewportSize({width:390,height:844});
 await popup.screenshot({path:join(output,'ppt-mobile.png'),fullPage:true});
 console.log('PASS portal click -> ordinary PPT session -> configured upload page -> reload/mobile');
 const createdPresentation=await context.request.post(pptBase+'/api/v1/ppt/presentation/create',{data:{content:'SSO ownership smoke test',n_slides:1,language:'English'}});
 assert.equal(createdPresentation.status(),200);
 const presentation=await createdPresentation.json();
 const owner=docker(['exec',ppt,'/opt/venv/bin/python','-c',"import sqlite3; c=sqlite3.connect('/app_data/fastapi.db'); print(c.execute('SELECT owner_id FROM presentations WHERE id=?', ('"+presentation.id.replaceAll('-','')+"',)).fetchone()[0])"]);
 assert.equal(owner.replaceAll('-',''),status.user_id.replaceAll('-',''));
 const second=await browser.newContext();
 const secondTicket=ticket('another-user');
 assert.equal((await second.request.get(pptBase+'/api/v1/auth/sso/callback?ticket='+secondTicket,{maxRedirects:0})).status(),303);
 const access=await second.request.get(pptBase+'/api/v1/ppt/presentation/'+presentation.id);
 assert.ok([403,404].includes(access.status()));
 const replay=await second.request.get(signed.data.redirect_url,{maxRedirects:0});
 assert.equal(replay.headers().location,'/login?error=SSO_failed');
 console.log('PASS presentation ownership isolated; ticket replay rejected');
 await context.request.get(pptBase+'/api/v1/auth/status',{headers:{'X-Sub2API-API-Key':'attacker-key','X-Sub2API-Base-URL':'http://attacker.invalid'}});
 assert.deepEqual(await (await context.request.get(pptBase+'/api/runtime-config')).json(),runtime);
 const logResult=spawnSync('docker',['logs',ppt],{encoding:'utf8'});
 assert.equal(logResult.status,0);
 const logs=logResult.stdout+logResult.stderr;
 assert.ok(!logs.includes(redirect.searchParams.get('ticket'))&&!logs.includes(secondTicket));
 assert.equal(errors.length,0,errors.join('\n'));
 console.log('PASS credentials hidden, header override disabled, no JS errors or ticket log leaks');
 console.log('ARTIFACTS '+output);
})().catch(async error=>{
 console.error('FAILED',String(error.message).replace(/ticket=[^\s]+/g,'ticket=[redacted]'));
 if(popup&&!popup.isClosed()) console.log('PPT_PAGE',(await popup.locator('body').innerText()).slice(0,1800));
 process.exitCode=1;
}).finally(async()=>{
 if(browser) await browser.close();
 for(const name of created.reverse()) {spawnSync('docker',['stop',name],{stdio:'ignore'});spawnSync('docker',['rm','-v',name],{stdio:'ignore'});}
 if(networkCreated) spawnSync('docker',['network','rm',id],{stdio:'ignore'});
});
