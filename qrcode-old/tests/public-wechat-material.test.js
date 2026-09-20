import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

test("front end should expose WeChat material controls gated by generated image", async () => {
  const html = await readFile(new URL("../public/index.html", import.meta.url), "utf8");
  const js = await readFile(new URL("../public/app.js", import.meta.url), "utf8");
  const css = await readFile(new URL("../public/styles.css", import.meta.url), "utf8");

  assert.match(html, /id="wechatAppid"/);
  assert.match(html, /id="wechatSecret"/);
  assert.doesNotMatch(html, /id="wechatAppid"[^>]*value="/);
  assert.match(html, /id="wechatSecret"[^>]*type="password"/);
  assert.doesNotMatch(html, /id="wechatSecret"[^>]*value="/);
  assert.doesNotMatch(html, /id="getWechatTokenButton"/);
  assert.doesNotMatch(html, /id="wechatAccessToken"/);
  assert.doesNotMatch(html, /id="wechatExpiresIn"/);
  assert.match(html, /id="wechatUploadName"/);
  assert.match(html, /id="uploadWechatMaterialButton"/);
  assert.match(html, /id="outputImageSizeStatus"/);
  assert.match(html, /id="prepareWechatImageButton"/);
  assert.match(html, /id="wechatMaterialSearchName"/);
  assert.match(html, /id="wechatMaterialPreview"/);
  assert.match(html, /id="wechatMaterialTable"/);
  assert.doesNotMatch(html, /id="wechatMaterialOffset"/);
  assert.doesNotMatch(html, /id="wechatMaterialCount"/);
  assert.doesNotMatch(html, /id="fetchWechatMaterialsButton"/);
  assert.match(js, /let generatedImageDataUrl = null/);
  assert.ok(
    js.indexOf("let generatedImageDataUrl = null") < js.indexOf("updateWechatUploadState();"),
    "generatedImageDataUrl should be initialized before updateWechatUploadState is called"
  );
  assert.match(js, /正在获取 access_token/);
  assert.match(js, /正在上传图片到微信素材库/);
  assert.match(js, /WECHAT_MATERIAL_LIST_OFFSET = 0/);
  assert.match(js, /WECHAT_MATERIAL_LIST_COUNT = 10000/);
  assert.match(js, /WECHAT_TOKEN_CACHE_KEY = "wechat_access_token_cache_v1"/);
  assert.match(js, /function getCachedWechatAccessToken\(\)/);
  assert.match(js, /function cacheWechatAccessToken\(data\)/);
  assert.match(js, /function clearWechatAccessTokenCache\(\)/);
  assert.match(js, /localStorage\.removeItem\(WECHAT_TOKEN_CACHE_KEY\)/);
  assert.match(js, /window\.addEventListener\("beforeunload", clearWechatAccessTokenCache\)/);
  assert.match(js, /async function getValidWechatAccessToken\(\)/);
  assert.match(js, /expiresAt/);
  assert.match(js, /Number\(data\.expires_in \|\| 0\) \* 1000/);
  assert.match(js, /await fetchWechatMaterialItems\(accessToken\)/);
  assert.match(js, /正在获取素材列表并匹配名称/);
  assert.match(js, /filterWechatMaterialsByName\(items, searchName\)/);
  assert.match(js, /item\.name\?\.includes\(keyword\)/);
  assert.match(js, /查看图片/);
  assert.match(js, /const accessToken = await getValidWechatAccessToken\(\)/);
  assert.match(js, /renderWechatMaterialRows\(matchedItems\)/);
  assert.match(js, /viewWechatMaterialImage\(item, button\)/);
  assert.match(js, /正在根据 media_id 获取图片/);
  assert.match(js, /await getValidWechatAccessToken\(\)/);
  assert.match(js, /postJson\(apiPath\("wechat\/material\/get"\)/);
  assert.match(js, /wechatMaterialPreview\.src = data\.imageDataUrl/);
  assert.doesNotMatch(js, /wechatMaterialPreview\.src = item\.url/);
  assert.match(js, /uploadWechatMaterialButton\.disabled = !generatedImageDataUrl/);
  assert.match(js, /WECHAT_IMAGE_MAX_BYTES = 10 \* 1024 \* 1024/);
  assert.match(js, /generatedImageBytes > WECHAT_IMAGE_MAX_BYTES/);
  assert.match(js, /apiPath\("wechat\/material\/prepare"\)/);
  assert.match(js, /图片大小/);
  assert.match(js, /超过微信永久图片素材 10MB 限制/);
  assert.match(js, /localStorage\.setItem\(WECHAT_MATERIAL_CACHE_KEY/);
  assert.match(js, /function getApiBasePath\(\)/);
  assert.match(js, /window\.QR_API_BASE_PATH/);
  assert.match(js, /qr-api-base-path/);
  assert.match(html, /name="qr-api-base-path" content="\.\/api"/);
  assert.match(js, /assertApiHealth\(\)/);
  assert.match(js, /apiPath\("health"\)/);
  assert.match(js, /QR_CORS_ORIGIN/);
  assert.match(js, /apiPath\("wechat\/material\/upload"\)/);
  assert.match(js, /getGeneratedMediaFile\(filename, selectedResult\)/);
  assert.match(js, /outputDownloadLink\?\.href/);
  assert.match(js, /outputPreview\?\.currentSrc/);
  assert.match(js, /文件内容为空，无法上传到微信素材库/);
  assert.match(js, /formData\.append\("media", mediaFile, uploadFilename\)/);
  assert.doesNotMatch(js, /formData\.append\("imageDataUrl"/);
  assert.match(js, /服务器返回了非 JSON 内容/);
  assert.match(css, /@media \(max-width: 640px\)/);
  assert.match(css, /\.wechat-search-grid/);
  assert.match(css, /\.wechat-material-panel/);
  assert.match(css, /\.wechat-material-table/);
  assert.match(css, /\.credential-card/);
  assert.match(css, /grid-template-columns: 1fr/);
});

test("front end should lock stylize action and show artistic progress", async () => {
  const html = await readFile(new URL("../public/index.html", import.meta.url), "utf8");
  const js = await readFile(new URL("../public/app.js", import.meta.url), "utf8");
  const css = await readFile(new URL("../public/styles.css", import.meta.url), "utf8");

  assert.match(html, /id="stylizeProgress"/);
  assert.match(html, /id="stylizeProgressBar"/);
  assert.match(html, /id="stylizeProgressText"/);
  assert.match(js, /let isStylizingQr = false/);
  assert.match(js, /if \(isStylizingQr\)/);
  assert.match(js, /setStylizeProgress\(12, "正在上传二维码"/);
  assert.match(js, /setStylizeProgress\(100, "艺术化完成"/);
  assert.match(js, /stylizeButton\.setAttribute\("aria-busy", "true"\)/);
  assert.match(js, /stylizeButton\.removeAttribute\("aria-busy"\)/);
  assert.match(css, /\.stylize-progress/);
  assert.match(css, /\.stylize-progress-bar/);
  assert.match(css, /transition: width 240ms ease/);
  assert.match(js, /let stylizeProgressTimer = null/);
  assert.match(js, /function startStylizeWaitingProgress\(\)/);
  assert.match(js, /function stopStylizeWaitingProgress\(\)/);
  assert.match(js, /正在艺术化，已等待/);
  assert.match(js, /Math\.min\(88/);
  assert.match(css, /@keyframes progress-stripes/);
  assert.match(css, /animation: progress-stripes/);
});

test("WeChat material operations should show visible loading feedback", async () => {
  const html = await readFile(new URL("../public/index.html", import.meta.url), "utf8");
  const js = await readFile(new URL("../public/app.js", import.meta.url), "utf8");
  const css = await readFile(new URL("../public/styles.css", import.meta.url), "utf8");

  assert.match(html, /id="wechatOperationProgress"/);
  assert.match(html, /id="wechatOperationProgressBar"/);
  assert.match(html, /id="wechatOperationProgressText"/);
  assert.match(js, /const wechatOperationProgress = document\.querySelector\("#wechatOperationProgress"\)/);
  assert.match(js, /function setWechatOperationProgress\(percent, message\)/);
  assert.match(js, /function startWechatOperation\(message\)/);
  assert.match(js, /function finishWechatOperation\(message/);
  assert.match(js, /function setButtonBusy\(button, label\)/);
  assert.match(js, /function clearButtonBusy\(button\)/);
  assert.match(js, /setButtonBusy\(uploadWechatMaterialButton, "上传中\.\.\."\)/);
  assert.match(js, /setButtonBusy\(searchWechatMaterialButton, "查询中\.\.\."\)/);
  assert.match(js, /setButtonBusy\(button, "查看中\.\.\."\)/);
  assert.match(js, /function setWechatPreviewLoading\(message\)/);
  assert.match(js, /wechatMaterialPreview\.removeAttribute\("src"\)/);
  assert.match(js, /wechatMaterialPreviewEmpty\.textContent = message/);
  assert.match(js, /function clearWechatPreviewLoading\(\)/);
  assert.match(css, /\.button-loading::before/);
  assert.match(css, /\.wechat-operation-progress/);
  assert.match(css, /\.wechat-operation-progress-bar/);
  assert.match(css, /\.wechat-preview-loading/);
  assert.match(css, /@keyframes button-spin/);
});

test("desktop layout should use wide screens without large empty side gutters", async () => {
  const css = await readFile(new URL("../public/styles.css", import.meta.url), "utf8");

  assert.match(css, /\.page[^{]*{[\s\S]*width: min\(1600px, calc\(100% - 64px\)\)/);
  assert.match(css, /\.workspace[^{]*{[\s\S]*grid-template-columns: 420px minmax\(0, 1fr\)/);
  assert.match(css, /@media \(min-width: 1280px\)/);
  assert.match(css, /grid-template-columns: repeat\(3, minmax\(0, 1fr\)\)/);
  assert.match(css, /@media \(max-width: 960px\)/);
});
