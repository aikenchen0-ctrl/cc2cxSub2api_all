const normalQrInput = document.querySelector("#normalQrImage");
const templateInput = document.querySelector("#templateImage");
const referenceInput = document.querySelector("#referenceImages");
const positivePromptInput = document.querySelector("#positivePrompt");
const negativePromptInput = document.querySelector("#negativePrompt");
const generateButton = document.querySelector("#generateButton");
const generateCountInput = document.querySelector("#generateCount");
const stylizeButton = document.querySelector("#stylizeButton");
const stylizeProgress = document.querySelector("#stylizeProgress");
const stylizeProgressBar = document.querySelector("#stylizeProgressBar");
const stylizeProgressText = document.querySelector("#stylizeProgressText");
const convertUrlButton = document.querySelector("#convertUrlButton");
const urlInput = document.querySelector("#urlInput");
const fileInputSection = document.querySelector("#fileInputSection");
const urlInputSection = document.querySelector("#urlInputSection");
const maskXInput = document.querySelector("#maskX");
const maskYInput = document.querySelector("#maskY");
const maskSizeInput = document.querySelector("#maskSize");
const maskShapeInputs = Array.from(document.querySelectorAll('input[name="maskShape"]'));
const cropCanvasContainer = document.querySelector("#cropCanvasContainer");
const cropCanvas = document.querySelector("#cropCanvas");
const cropXSpan = document.querySelector("#cropX");
const cropYSpan = document.querySelector("#cropY");
const cropWidthSpan = document.querySelector("#cropWidth");
const cropHeightSpan = document.querySelector("#cropHeight");
const cropEmpty = document.querySelector("#cropEmpty");
const statusNode = document.querySelector("#status");
const inputPreview = document.querySelector("#inputPreview");
const artisticPreview = document.querySelector("#artisticPreview");
const templatePreview = document.querySelector("#templatePreview");
const templatePresetCard = document.querySelector("[data-template-preset]");
const outputPreview = document.querySelector("#outputPreview");
const batchResultGrid = document.querySelector("#batchResultGrid");
const inputDownloadLink = document.querySelector("#downloadInputImage");
const artisticDownloadLink = document.querySelector("#downloadArtisticImage");
const collageDownloadLink = document.querySelector("#downloadCollageImage");
const outputDownloadLink = document.querySelector("#downloadOutputImage");
const outputImageSizeStatus = document.querySelector("#outputImageSizeStatus");
const prepareWechatImageButton = document.querySelector("#prepareWechatImageButton");
const inputEmpty = document.querySelector("#inputEmpty");
const artisticEmpty = document.querySelector("#artisticEmpty");
const templateEmpty = document.querySelector("#templateEmpty");
const outputEmpty = document.querySelector("#outputEmpty");
const referencePreview = document.querySelector("#referencePreview");
const referenceEmpty = document.querySelector("#referenceEmpty");
const collagePreview = document.querySelector("#collagePreview");
const collageEmpty = document.querySelector("#collageEmpty");
const debugOverlayPreview = document.querySelector("#debugOverlayPreview");
const debugOverlayEmpty = document.querySelector("#debugOverlayEmpty");
const revisedPromptNode = document.querySelector("#revisedPrompt");
const pipelineThoughtsNode = document.querySelector("#pipelineThoughts");
const wechatAppidInput = document.querySelector("#wechatAppid");
const wechatSecretInput = document.querySelector("#wechatSecret");
const wechatUploadNameInput = document.querySelector("#wechatUploadName");
const uploadWechatMaterialButton = document.querySelector("#uploadWechatMaterialButton");
const wechatMaterialSearchNameInput = document.querySelector("#wechatMaterialSearchName");
const searchWechatMaterialButton = document.querySelector("#searchWechatMaterialButton");
const wechatMaterialStatus = document.querySelector("#wechatMaterialStatus");
const wechatMaterialResult = document.querySelector("#wechatMaterialResult");
const wechatMaterialTable = document.querySelector("#wechatMaterialTable");
const wechatMaterialPreview = document.querySelector("#wechatMaterialPreview");
const wechatMaterialPreviewEmpty = document.querySelector("#wechatMaterialPreviewEmpty");
const wechatOperationProgress = document.querySelector("#wechatOperationProgress");
const wechatOperationProgressBar = document.querySelector("#wechatOperationProgressBar");
const wechatOperationProgressText = document.querySelector("#wechatOperationProgressText");
const API_BASE_PATH = getApiBasePath();
const WECHAT_IMAGE_MAX_BYTES = 10 * 1024 * 1024;
const WECHAT_MATERIAL_LIST_OFFSET = 0;
const WECHAT_MATERIAL_LIST_COUNT = 10000;
const WECHAT_TOKEN_CACHE_KEY = "wechat_access_token_cache_v1";
const WECHAT_TOKEN_EXPIRE_BUFFER_MS = 60 * 1000;
const MAX_UPLOAD_IMAGE_BYTES = 15 * 1024 * 1024;
const DEFAULT_GENERATE_COUNT = 3;
const DEFAULT_MASK_X = 250;
const DEFAULT_MASK_Y = 270;
const DEFAULT_MASK_SIZE = 950;
let generatedImageDataUrl = null;
let generatedImageBytes = 0;
let generatedResults = [];
let selectedGeneratedImageIndex = -1;
let isStylizingQr = false;
let stylizeProgressTimer = null;
let wechatOperationProgressTimer = null;
let wechatOperationProgressHideTimer = null;
let defaultTemplateFile = null;

function getTemplateFile() {
  return templateInput.files?.[0] || defaultTemplateFile;
}

positivePromptInput.value =
  "延展盘子空白处周围荷花纹理，严格锁定中心方形内全部像素与四角定位图形，不移动、不重绘、不改对比度；仅延展盘子空白处周围荷花纹理、色调与光影，让非定位区域边缘自然融合、无缝衔接不要有白边。把正方形的效果明显的边边角角也要融合到延展中融为一体。";

negativePromptInput.value =
  "不要改变中心方形二维码结构，不要重画二维码，不要移动二维码，不要裁剪二维码，不要破坏二维码可扫描性，不要改变原图光影关系，不要改变盘子透视，不要改变玻璃柜和阴影关系，不要在二维码区域新增明显图案，只允许在盘子四周空白区域填充与参考图相似的花纹。";

const FIXED_PROMPT_TEXT = [
  "【正向 Prompt】",
  positivePromptInput.value,
  "",
  "【负向 Prompt】",
  negativePromptInput.value
].join("\n");

const FIXED_PIPELINE_TEXT = [
  "固定链路：",
  "1. 前端先上传普通二维码，再点击“艺术化二维码”。",
  "2. 服务端先按当前框选区域裁掉二维码周围白边，再通过 Sub2API 会话中继调用图像模型。",
  "3. 固定遮罩参数：x 默认 250, y 默认 270, size 默认 950x950，可页面调节；二维码不按模板等比缩放。",
  "4. 如果空白盘模板像素尺寸不足，服务端只放大模板到可容纳固定框；再把 950x950 二维码粘贴进去。",
  "5. “艺术化二维码”固定通过 Sub2API /v1/images/edits 调用公开模型 gpt-image-2。",
  "6. 实际提交给接口的上下文图就是服务端本地处理后的盘中二维码图。",
  "7. 点击生成时先秒出上下文图，再调用 gpt-image-2 edits 生成最终图片。",
  "8. 本区域是写死说明，不再依赖接口返回。"
].join("\n");

revisedPromptNode.textContent = FIXED_PROMPT_TEXT;
pipelineThoughtsNode.textContent = FIXED_PIPELINE_TEXT;
updateWechatUploadState();
renderGeneratedImageSize();
let previewMaskTimer = null;
const GENERATE_TIMEOUT_MS = 900000;
const GENERATE_POLL_INTERVAL_MS = 2000;
const MASK_NUDGE_STEP = 10;
const WECHAT_MATERIAL_CACHE_KEY = "wechat_material_cache_v1";

function getApiBasePath() {
  const fromWindow = window.QR_API_BASE_PATH;
  const fromMeta = document.querySelector('meta[name="qr-api-base-path"]')?.content;
  const value = String(fromWindow || fromMeta || "/api").trim() || "/api";
  return value.replace(/\/+$/, "");
}

function apiPath(path) {
  return `${API_BASE_PATH}/${path.replace(/^\/+/, "")}`;
}

// Canvas框选相关变量
let cropImage = null;
let isDrawing = false;
let startX = 0;
let startY = 0;
let currentRect = { x: 0, y: 0, width: 0, height: 0 };
let cropContext = null;
let workingQrFile = null;
let cropTarget = "normal";
let cropDrawBox = { x: 0, y: 0, width: 0, height: 0 };

function setStatus(message, tone = "idle") {
  statusNode.textContent = message;
  statusNode.dataset.tone = tone;
}

function setWechatStatus(message, tone = "idle") {
  if (!wechatMaterialStatus) {
    return;
  }

  wechatMaterialStatus.textContent = message;
  wechatMaterialStatus.dataset.tone = tone;
}

function setWechatResult(value) {
  if (wechatMaterialTable) {
    wechatMaterialTable.hidden = true;
    wechatMaterialTable.replaceChildren();
  }
  if (!wechatMaterialResult) {
    return;
  }
  wechatMaterialResult.hidden = false;
  wechatMaterialResult.textContent = typeof value === "string"
    ? value
    : JSON.stringify(value, null, 2);
}

function beginWechatOperation(message, detail, { progress = true } = {}) {
  setWechatStatus(message, "working");
  setWechatResult(detail);
  if (progress) {
    startWechatOperation(message);
  }
}

function setWechatOperationProgress(percent, message) {
  if (!wechatOperationProgress || !wechatOperationProgressBar || !wechatOperationProgressText) {
    return;
  }

  const safePercent = Math.max(0, Math.min(100, Number(percent) || 0));
  wechatOperationProgress.hidden = false;
  wechatOperationProgressBar.style.width = `${safePercent}%`;
  wechatOperationProgressText.textContent = message;
}

function startWechatOperation(message) {
  stopWechatOperationProgress();
  if (wechatOperationProgressHideTimer) {
    clearTimeout(wechatOperationProgressHideTimer);
    wechatOperationProgressHideTimer = null;
  }

  const startedAt = Date.now();
  let visibleProgress = 12;
  if (wechatOperationProgress) {
    wechatOperationProgress.dataset.tone = "working";
  }
  setWechatOperationProgress(visibleProgress, message);

  wechatOperationProgressTimer = setInterval(() => {
    const elapsedSeconds = Math.max(1, Math.round((Date.now() - startedAt) / 1000));
    visibleProgress = Math.min(88, visibleProgress + (elapsedSeconds < 8 ? 6 : 2));
    setWechatOperationProgress(visibleProgress, `${message} 已等待 ${elapsedSeconds} 秒`);
  }, 1000);
}

function stopWechatOperationProgress() {
  if (wechatOperationProgressTimer) {
    clearInterval(wechatOperationProgressTimer);
    wechatOperationProgressTimer = null;
  }
}

function finishWechatOperation(message, tone = "success") {
  stopWechatOperationProgress();
  setWechatOperationProgress(100, message);
  if (wechatOperationProgress) {
    wechatOperationProgress.dataset.tone = tone;
  }

  if (wechatOperationProgressHideTimer) {
    clearTimeout(wechatOperationProgressHideTimer);
  }

  wechatOperationProgressHideTimer = setTimeout(() => {
    if (!wechatOperationProgress || !wechatOperationProgressBar || !wechatOperationProgressText) {
      return;
    }
    wechatOperationProgress.hidden = true;
    wechatOperationProgress.dataset.tone = "idle";
    wechatOperationProgressBar.style.width = "0%";
    wechatOperationProgressText.textContent = "等待操作";
  }, 1400);
}

function setButtonBusy(button, label) {
  if (!button) {
    return;
  }

  if (!button.dataset.originalText) {
    button.dataset.originalText = button.textContent || "";
  }
  button.textContent = label;
  button.disabled = true;
  button.classList.add("button-loading");
  button.setAttribute("aria-busy", "true");
}

function clearButtonBusy(button) {
  if (!button) {
    return;
  }

  if (button.dataset.originalText) {
    button.textContent = button.dataset.originalText;
    delete button.dataset.originalText;
  }
  button.classList.remove("button-loading");
  button.removeAttribute("aria-busy");
  button.disabled = false;
}

function setWechatPreviewLoading(message) {
  if (!wechatMaterialPreview || !wechatMaterialPreviewEmpty) {
    return;
  }

  wechatMaterialPreview.removeAttribute("src");
  wechatMaterialPreviewEmpty.hidden = false;
  wechatMaterialPreviewEmpty.textContent = message;
  wechatMaterialPreview.closest(".image-frame")?.classList.add("wechat-preview-loading");
}

function clearWechatPreviewLoading() {
  if (!wechatMaterialPreviewEmpty) {
    return;
  }

  wechatMaterialPreviewEmpty.textContent = "查询素材后在这里显示";
  wechatMaterialPreview?.closest(".image-frame")?.classList.remove("wechat-preview-loading");
}

function setStylizeProgress(percent, message) {
  if (!stylizeProgress || !stylizeProgressBar || !stylizeProgressText) {
    return;
  }

  const safePercent = Math.max(0, Math.min(100, Number(percent) || 0));
  stylizeProgress.hidden = false;
  stylizeProgressBar.style.width = `${safePercent}%`;
  stylizeProgressText.textContent = message;
}

function resetStylizeProgress() {
  stopStylizeWaitingProgress();
  if (!stylizeProgress || !stylizeProgressBar || !stylizeProgressText) {
    return;
  }

  stylizeProgress.hidden = true;
  stylizeProgressBar.style.width = "0%";
  stylizeProgressText.textContent = "等待艺术化";
}

function startStylizeWaitingProgress() {
  stopStylizeWaitingProgress();
  const startedAt = Date.now();
  let visibleProgress = 38;

  stylizeProgressTimer = setInterval(() => {
    const elapsedSeconds = Math.max(1, Math.round((Date.now() - startedAt) / 1000));
    visibleProgress = Math.min(88, visibleProgress + (elapsedSeconds < 12 ? 4 : 1));
    setStylizeProgress(visibleProgress, `正在艺术化，已等待 ${elapsedSeconds} 秒，请保持页面打开`);
  }, 1000);
}

function stopStylizeWaitingProgress() {
  if (stylizeProgressTimer) {
    clearInterval(stylizeProgressTimer);
    stylizeProgressTimer = null;
  }
}

function validateImageFile(file, label, { required = true, maxBytes = MAX_UPLOAD_IMAGE_BYTES } = {}) {
  if (!file) {
    if (required) {
      throw new Error(`请先选择${label}。`);
    }
    return;
  }

  const allowedTypes = new Set(["image/png", "image/jpeg", "image/webp"]);
  if (!allowedTypes.has(file.type)) {
    throw new Error(`${label}只支持 PNG、JPG 或 WebP 图片。`);
  }

  if (file.size > maxBytes) {
    throw new Error(`${label}不能超过 ${formatBytes(maxBytes)}。`);
  }
}

function updateWechatUploadState() {
  if (uploadWechatMaterialButton) {
    uploadWechatMaterialButton.disabled = !generatedImageDataUrl || generatedImageBytes > WECHAT_IMAGE_MAX_BYTES;
  }
}

function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return "0 B";
  }

  const units = ["B", "KB", "MB"];
  let value = bytes;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  return `${value.toFixed(unitIndex === 0 ? 0 : 2)} ${units[unitIndex]}`;
}

function getMimeExtension(mimeType) {
  const normalized = String(mimeType || "").toLowerCase();
  if (normalized.includes("jpeg") || normalized.includes("jpg")) {
    return "jpg";
  }
  if (normalized.includes("png")) {
    return "png";
  }
  if (normalized.includes("gif")) {
    return "gif";
  }
  if (normalized.includes("bmp")) {
    return "bmp";
  }
  return "png";
}

function normalizeFilenameForMime(filename, mimeType) {
  const fallbackName = "generated-qrcode";
  const cleanName = String(filename || "").trim() || `${fallbackName}.png`;
  const extension = getMimeExtension(mimeType);
  const withoutExtension = cleanName.replace(/\.(bmp|png|jpe?g|gif)$/i, "");
  return `${withoutExtension || fallbackName}.${extension}`;
}

function getDataUrlByteSize(dataUrl) {
  const base64 = String(dataUrl || "").split(",")[1] || "";
  const padding = (base64.match(/=+$/)?.[0]?.length) || 0;
  return Math.max(0, Math.floor((base64.length * 3) / 4) - padding);
}

function renderGeneratedImageSize() {
  if (!outputImageSizeStatus) {
    return;
  }

  if (!generatedImageDataUrl) {
    outputImageSizeStatus.textContent = "生成后显示图片大小";
    outputImageSizeStatus.dataset.tone = "idle";
    if (prepareWechatImageButton) {
      prepareWechatImageButton.hidden = true;
    }
    return;
  }

  if (generatedImageBytes > WECHAT_IMAGE_MAX_BYTES) {
    outputImageSizeStatus.textContent = `图片大小 ${formatBytes(generatedImageBytes)}，超过微信永久图片素材 10MB 限制。建议先压缩 PNG；如果仍超限，再降低最终图尺寸重新生成，二维码区域不做重绘。`;
    outputImageSizeStatus.dataset.tone = "error";
    setWechatResult({
      size: formatBytes(generatedImageBytes),
      limit: "10 MB",
      uploadBlocked: true,
      plan: [
        "先使用 PNG 压缩，不改变二维码结构。",
        "如果压缩后仍超限，再按比例降低整张图尺寸，保留二维码整体几何关系。",
        "仍无法达标时，建议重新生成更低细节或更小尺寸图片。"
      ]
    });
    if (prepareWechatImageButton) {
      prepareWechatImageButton.hidden = false;
      prepareWechatImageButton.disabled = false;
    }
    return;
  }

  outputImageSizeStatus.textContent = `图片大小 ${formatBytes(generatedImageBytes)}，符合微信永久图片素材 10MB 限制。`;
  outputImageSizeStatus.dataset.tone = "success";
  if (prepareWechatImageButton) {
    prepareWechatImageButton.hidden = true;
  }
}

function getGenerateCount() {
  const count = Number(generateCountInput?.value || DEFAULT_GENERATE_COUNT);
  if (!Number.isInteger(count) || count < 1) {
    return DEFAULT_GENERATE_COUNT;
  }
  return Math.min(count, 12);
}

function getSelectedGeneratedResult() {
  return generatedResults[selectedGeneratedImageIndex] || null;
}

function setGeneratedImageDataUrl(imageDataUrl, {
  filename = "generated-qrcode.png",
  index = 0,
  syncResults = true
} = {}) {
  generatedImageDataUrl = imageDataUrl || null;
  generatedImageBytes = generatedImageDataUrl ? getDataUrlByteSize(generatedImageDataUrl) : 0;
  selectedGeneratedImageIndex = generatedImageDataUrl ? index : -1;
  if (syncResults) {
    generatedResults = generatedImageDataUrl
      ? [{ imageDataUrl: generatedImageDataUrl, filename, bytes: generatedImageBytes }]
      : [];
    if (batchResultGrid && !generatedImageDataUrl) {
      batchResultGrid.replaceChildren();
      batchResultGrid.hidden = true;
    }
  }
  renderGeneratedImageSize();
  updateWechatUploadState();
}

function selectGeneratedResult(index) {
  const result = generatedResults[index];
  if (!result?.imageDataUrl) {
    return;
  }

  selectedGeneratedImageIndex = index;
  outputPreview.src = result.imageDataUrl;
  outputEmpty.hidden = true;
  generatedImageDataUrl = result.imageDataUrl;
  generatedImageBytes = result.bytes ?? getDataUrlByteSize(result.imageDataUrl);
  updateImageDownloadLink(outputDownloadLink, result.imageDataUrl, result.filename || `generated-qrcode-${index + 1}.png`);
  renderGeneratedImageSize();
  updateWechatUploadState();

  batchResultGrid?.querySelectorAll(".generated-result-card").forEach((card, cardIndex) => {
    card.className = cardIndex === index
      ? "generated-result-card selected"
      : "generated-result-card";
    card.setAttribute("aria-pressed", cardIndex === index ? "true" : "false");
  });
}

function renderBatchGeneratedResults(results) {
  generatedResults = results.map((result, index) => {
    const filename = result.filename || `generated-qrcode-${index + 1}.png`;
    return {
      ...result,
      filename,
      bytes: getDataUrlByteSize(result.imageDataUrl)
    };
  });

  if (!batchResultGrid) {
    selectGeneratedResult(0);
    return;
  }

  batchResultGrid.replaceChildren();
  batchResultGrid.hidden = generatedResults.length <= 1;

  generatedResults.forEach((result, index) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "generated-result-card";
    button.setAttribute("aria-pressed", "false");
    button.addEventListener("click", () => selectGeneratedResult(index));

    const image = document.createElement("img");
    image.src = result.imageDataUrl;
    image.alt = `生成结果 ${index + 1}`;

    const label = document.createElement("span");
    label.textContent = `结果 ${index + 1}`;

    button.append(image, label);
    batchResultGrid.appendChild(button);
  });

  selectGeneratedResult(0);
}

function getGenerateStatusLabel(status) {
  if (status === "queued") return "排队中";
  if (status === "running") return "生成中";
  if (status === "failed") return "生成失败";
  if (status === "succeeded") return "已完成";
  return "等待中";
}

function renderGeneratedResultCards(totalCount = generatedResults.length) {
  if (!batchResultGrid) {
    return;
  }

  batchResultGrid.replaceChildren();
  batchResultGrid.hidden = totalCount <= 1;

  for (let index = 0; index < totalCount; index += 1) {
    const result = generatedResults[index] || { status: "pending" };
    const button = document.createElement("button");
    button.type = "button";
    button.className = index === selectedGeneratedImageIndex
      ? "generated-result-card selected"
      : "generated-result-card";
    button.setAttribute("aria-pressed", index === selectedGeneratedImageIndex ? "true" : "false");
    button.disabled = !result.imageDataUrl;

    if (result.imageDataUrl) {
      button.addEventListener("click", () => selectGeneratedResult(index));

      const image = document.createElement("img");
      image.src = result.imageDataUrl;
      image.alt = `生成结果 ${index + 1}`;

      const label = document.createElement("span");
      label.textContent = `结果 ${index + 1}`;

      button.append(image, label);
    } else {
      const label = document.createElement("span");
      label.textContent = `结果 ${index + 1}`;

      const status = document.createElement("span");
      status.textContent = getGenerateStatusLabel(result.status);

      button.append(label, status);
    }

    batchResultGrid.appendChild(button);
  }
}

function updateGeneratedResultStatus(index, status, message = "", totalCount = generatedResults.length) {
  generatedResults[index] = {
    ...(generatedResults[index] || {}),
    status,
    message
  };
  renderGeneratedResultCards(totalCount);
}

function appendGeneratedResult(result, index, totalCount = generatedResults.length) {
  if (!result?.imageDataUrl) {
    return;
  }

  const filename = result.filename || `generated-qrcode-${index + 1}.png`;
  generatedResults[index] = {
    ...result,
    filename,
    bytes: getDataUrlByteSize(result.imageDataUrl),
    status: "succeeded"
  };

  if (!batchResultGrid) {
    if (selectedGeneratedImageIndex < 0) {
      selectGeneratedResult(index);
    }
    return;
  }

  renderGeneratedResultCards(totalCount);

  if (selectedGeneratedImageIndex < 0 || !generatedImageDataUrl) {
    selectGeneratedResult(index);
    renderPipeline(result?.pipeline);
  }
}

async function postJson(url, payload) {
  let response;
  try {
    response = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload)
    });
  } catch (error) {
    throw new Error(
      `无法访问后端 API：${url}。请确认线上部署运行了 Node 服务，并且 API 地址配置正确；如果前后端不同域名，请在后端设置 QR_CORS_ORIGIN。原始错误：${error?.message || "网络请求失败"}`
    );
  }
  const { data, text } = await readResponsePayload(response);

  if (!response.ok) {
    throw new Error(
      data?.error
        || `请求失败：HTTP ${response.status} ${response.statusText} ${text?.slice(0, 300) || ""}`.trim()
    );
  }

  if (!data) {
    throw new Error(`服务器返回了非 JSON 内容：${text?.slice(0, 300) || "空响应"}`);
  }

  return data;
}

async function assertApiHealth() {
  let response;
  try {
    response = await fetch(apiPath("health"), { method: "GET" });
  } catch (error) {
    throw new Error(
      `无法访问后端 API：${apiPath("health")}。上线时不能只部署 public 静态目录，需要运行 npm start 对应的 Node 服务；如果前后端分开部署，请通过 window.QR_API_BASE_PATH 或 meta[name="qr-api-base-path"] 配置 API 地址，并在后端设置 QR_CORS_ORIGIN。原始错误：${error?.message || "网络请求失败"}`
    );
  }

  if (!response.ok) {
    throw new Error(`后端 API 健康检查失败：HTTP ${response.status}。请确认部署服务把 /api 转发到 Node 后端。`);
  }
}

function getWechatCredentialPayload() {
  const appid = wechatAppidInput?.value.trim() || "";
  const secret = wechatSecretInput?.value.trim() || "";

  if (!appid || !secret) {
    throw new Error("请先填写 AppID 和 Secret，系统会在上传或查询时自动获取 access_token。");
  }

  return { appid, secret };
}

function getCachedWechatAccessToken() {
  const rawCache = localStorage.getItem(WECHAT_TOKEN_CACHE_KEY);
  if (!rawCache) {
    return "";
  }

  try {
    const cache = JSON.parse(rawCache);
    const accessToken = String(cache?.accessToken || "").trim();
    const expiresAt = Number(cache?.expiresAt || 0);

    if (accessToken && expiresAt > Date.now() + WECHAT_TOKEN_EXPIRE_BUFFER_MS) {
      return accessToken;
    }
  } catch {
    // 缓存结构损坏时直接清理，避免继续使用不可信 token。
  }

  clearWechatAccessTokenCache();
  return "";
}

function cacheWechatAccessToken(data) {
  const accessToken = String(data?.access_token || "").trim();
  const expiresInMs = Number(data.expires_in || 0) * 1000;

  if (!accessToken || !Number.isFinite(expiresInMs) || expiresInMs <= 0) {
    throw new Error("微信接口没有返回有效 access_token 或 expires_in。");
  }

  const expiresAt = Date.now() + expiresInMs;
  localStorage.setItem(WECHAT_TOKEN_CACHE_KEY, JSON.stringify({ accessToken, expiresAt }));
  return accessToken;
}

function clearWechatAccessTokenCache() {
  localStorage.removeItem(WECHAT_TOKEN_CACHE_KEY);
}

async function getValidWechatAccessToken() {
  const cachedToken = getCachedWechatAccessToken();
  if (cachedToken) {
    setWechatStatus("已使用本次浏览器会话中未过期的 access_token。", "ready");
    return cachedToken;
  }

  const credentials = getWechatCredentialPayload();
  beginWechatOperation("正在获取 access_token...", {
    action: "get_access_token",
    endpoint: apiPath("wechat/token"),
    appid: credentials.appid
  });
  setWechatOperationProgress(24, "正在校验后端服务...");
  await assertApiHealth();
  setWechatOperationProgress(38, "正在向微信接口获取 access_token...");
  const data = await postJson(apiPath("wechat/token"), credentials);
  const accessToken = cacheWechatAccessToken(data);
  setWechatOperationProgress(52, "access_token 已获取，继续执行当前操作...");
  setWechatStatus("access_token 已自动获取，本次浏览器会话内会复用。", "success");
  return accessToken;
}

window.addEventListener("beforeunload", clearWechatAccessTokenCache);

function storeWechatMaterials(items) {
  localStorage.setItem(WECHAT_MATERIAL_CACHE_KEY, JSON.stringify(items));
}

async function fetchWechatMaterialItems(accessToken) {
  const data = await postJson(apiPath("wechat/material/list"), {
    accessToken,
    offset: WECHAT_MATERIAL_LIST_OFFSET,
    count: WECHAT_MATERIAL_LIST_COUNT
  });
  const items = Array.isArray(data.item) ? data.item : [];
  storeWechatMaterials(items);

  return { data, items };
}

function filterWechatMaterialsByName(items, searchName) {
  const keyword = String(searchName || "").trim();
  if (!keyword) {
    return [];
  }

  return items.filter((item) => item.name?.includes(keyword));
}

function formatWechatMaterialTime(value) {
  const timestamp = Number(value);
  if (!Number.isFinite(timestamp) || timestamp <= 0) {
    return "-";
  }

  return new Date(timestamp * 1000).toLocaleString("zh-CN", { hour12: false });
}

async function viewWechatMaterialImage(item, button) {
  if (!item?.media_id) {
    setWechatStatus("这个素材缺少 media_id，无法获取图片。", "error");
    return;
  }

  setButtonBusy(button, "查看中...");
  startWechatOperation("正在获取素材图片...");
  setWechatPreviewLoading("正在获取图片，请稍候...");
  setWechatStatus("正在根据 media_id 获取图片...", "working");

  try {
    const accessToken = await getValidWechatAccessToken();
    setWechatOperationProgress(62, "正在根据 media_id 请求图片...");
    const data = await postJson(apiPath("wechat/material/get"), {
      accessToken,
      mediaId: item.media_id
    });

    if (!data?.imageDataUrl) {
      throw new Error("素材接口未返回可展示的图片。");
    }

    setWechatOperationProgress(88, "图片已返回，正在渲染预览...");
    clearWechatPreviewLoading();
    wechatMaterialPreview.src = data.imageDataUrl;
    wechatMaterialPreview.alt = item.name ? `微信素材预览：${item.name}` : "微信素材预览";
    wechatMaterialPreviewEmpty.hidden = true;
    setWechatStatus(`已展示素材：${item.name || item.media_id || "-"}`, "success");
    finishWechatOperation("图片展示完成");
  } catch (error) {
    setWechatStatus(error.message || "获取素材图片失败。", "error");
    clearWechatPreviewLoading();
    if (wechatMaterialPreview) {
      wechatMaterialPreview.removeAttribute("src");
    }
    if (wechatMaterialPreviewEmpty) {
      wechatMaterialPreviewEmpty.hidden = false;
      wechatMaterialPreviewEmpty.textContent = "获取素材图片失败，请检查素材或重新点击查看。";
    }
    finishWechatOperation("图片获取失败", "error");
  } finally {
    clearButtonBusy(button);
  }
}

function renderWechatMaterialRows(items) {
  if (!wechatMaterialTable) {
    return;
  }

  if (wechatMaterialResult) {
    wechatMaterialResult.hidden = true;
  }
  wechatMaterialTable.hidden = false;
  wechatMaterialTable.replaceChildren();

  const table = document.createElement("table");
  table.className = "wechat-material-table";

  const thead = document.createElement("thead");
  const headerRow = document.createElement("tr");
  ["序号", "素材名称", "media_id", "更新时间", "图片 URL", "操作"].forEach((label) => {
    const th = document.createElement("th");
    th.textContent = label;
    headerRow.appendChild(th);
  });
  thead.appendChild(headerRow);
  table.appendChild(thead);

  const tbody = document.createElement("tbody");
  if (items.length === 0) {
    const row = document.createElement("tr");
    const cell = document.createElement("td");
    cell.colSpan = 6;
    cell.textContent = "未找到匹配素材";
    cell.className = "wechat-material-empty-cell";
    row.appendChild(cell);
    tbody.appendChild(row);
  }

  items.forEach((item, index) => {
    const row = document.createElement("tr");
    const values = [
      String(index + 1),
      item.name || "-",
      item.media_id || "-",
      formatWechatMaterialTime(item.update_time),
      item.url || "-"
    ];

    values.forEach((value, valueIndex) => {
      const cell = document.createElement("td");
      if (valueIndex === 4 && item.url) {
        const link = document.createElement("a");
        link.href = item.url;
        link.target = "_blank";
        link.rel = "noopener noreferrer";
        link.textContent = item.url;
        cell.appendChild(link);
      } else {
        cell.textContent = value;
      }
      row.appendChild(cell);
    });

    const actionCell = document.createElement("td");
    const button = document.createElement("button");
    button.type = "button";
    button.className = "secondary-button wechat-view-button";
    button.textContent = "查看图片";
    button.disabled = !item.media_id;
    button.addEventListener("click", () => viewWechatMaterialImage(item, button));
    actionCell.appendChild(button);
    row.appendChild(actionCell);
    tbody.appendChild(row);
  });

  table.appendChild(tbody);
  wechatMaterialTable.appendChild(table);
}

function updateImageDownloadLink(link, imageUrl, filename) {
  if (!link) {
    return;
  }

  if (!imageUrl) {
    link.removeAttribute("href");
    link.hidden = true;
    return;
  }

  link.href = imageUrl;
  link.download = filename;
  link.hidden = false;
}

function renderPipeline(pipeline) {
  if (!pipeline) {
    return;
  }

  if (pipeline.debugOverlayDataUrl) {
    if (debugOverlayPreview && debugOverlayEmpty) {
      debugOverlayPreview.src = pipeline.debugOverlayDataUrl;
      debugOverlayEmpty.hidden = true;
    }
    collagePreview.src = pipeline.debugOverlayDataUrl;
    collageEmpty.hidden = true;
    updateImageDownloadLink(collageDownloadLink, pipeline.debugOverlayDataUrl, "generation-context.png");
    return;
  }
  if (pipeline.collagePreviewDataUrl) {
    collagePreview.src = pipeline.collagePreviewDataUrl;
    collageEmpty.hidden = true;
    updateImageDownloadLink(collageDownloadLink, pipeline.collagePreviewDataUrl, "generation-context.png");
  }
}

function getActiveQrFile() {
  return workingQrFile || normalQrInput.files?.[0] || null;
}

function getMaskPlacement() {
  return {
    x: Number(maskXInput?.value || DEFAULT_MASK_X),
    y: Number(maskYInput?.value || DEFAULT_MASK_Y),
    size: Number(maskSizeInput?.value || DEFAULT_MASK_SIZE)
  };
}

function getMaskShape() {
  return maskShapeInputs.find((input) => input.checked)?.value === "circle" ? "circle" : "square";
}

async function dataUrlToFile(dataUrl, filename) {
  const response = await fetch(dataUrl);
  if (!response.ok) {
    throw new Error(`读取图片失败：HTTP ${response.status} ${response.statusText}`.trim());
  }
  const blob = await response.blob();
  if (!blob.size) {
    throw new Error("文件内容为空，无法上传到微信素材库。");
  }
  const mimeType = blob.type || "image/png";
  return new File([blob], normalizeFilenameForMime(filename, mimeType), { type: mimeType });
}

async function imageUrlToFile(imageUrl, filename) {
  if (!imageUrl) {
    throw new Error("生成完成后才可以上传到素材库。");
  }

  const response = await fetch(imageUrl);
  if (!response.ok) {
    throw new Error(`读取生成图片失败：HTTP ${response.status} ${response.statusText}`.trim());
  }

  const blob = await response.blob();
  if (!blob.size) {
    throw new Error("文件内容为空，无法上传到微信素材库。");
  }

  const mimeType = blob.type || "image/png";
  return new File([blob], normalizeFilenameForMime(filename, mimeType), { type: mimeType });
}

async function getGeneratedMediaFile(filename, selectedResult = getSelectedGeneratedResult()) {
  const imageUrl = selectedResult?.imageDataUrl
    || outputDownloadLink?.href
    || outputPreview?.currentSrc
    || outputPreview?.src
    || generatedImageDataUrl;
  return imageUrlToFile(imageUrl, filename || selectedResult?.filename);
}

async function readResponsePayload(response) {
  const text = await response.text();
  let data = null;

  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = null;
  }

  return { text, data };
}

function waitForGeneratePoll(signal) {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(new DOMException("生成请求已取消。", "AbortError"));
      return;
    }

    const timer = setTimeout(resolve, GENERATE_POLL_INTERVAL_MS);
    signal?.addEventListener("abort", () => {
      clearTimeout(timer);
      reject(new DOMException("生成请求已取消。", "AbortError"));
    }, { once: true });
  });
}

async function pollGenerateJob(jobId, index, signal, totalCount = generatedResults.length) {
  while (true) {
    const response = await fetch(apiPath(`generate/jobs/${encodeURIComponent(jobId)}`), {
      method: "GET",
      signal
    });
    const { data, text } = await readResponsePayload(response);

    if (!response.ok) {
      throw new Error(
        data?.error
          || `第 ${index + 1} 张生成轮询失败：HTTP ${response.status} ${response.statusText} ${text?.slice(0, 300) || ""}`.trim()
      );
    }

    if (data?.status === "succeeded") {
      const result = data.result;
      if (!result?.imageDataUrl) {
        renderPipeline(result?.pipeline);
        throw new Error(`第 ${index + 1} 张生成失败：任务完成但没有返回生成图片。`);
      }
      return result;
    }

    if (data?.status === "failed") {
      updateGeneratedResultStatus(index, "failed", data?.error || "", totalCount);
      renderPipeline(data?.pipeline);
      throw new Error(data?.error || `第 ${index + 1} 张生成失败。`);
    }

    if (data?.status !== "queued" && data?.status !== "running") {
      throw new Error(`第 ${index + 1} 张生成状态异常：${data?.status || "空状态"}`);
    }

    updateGeneratedResultStatus(index, data.status, "", totalCount);
    setStatus(`正在等待第 ${index + 1} 张最终图生成...`, "working");
    await waitForGeneratePoll(signal);
  }
}

async function submitGenerateJob(formData, index, signal, totalCount = generatedResults.length) {
  updateGeneratedResultStatus(index, "queued", "", totalCount);

  const response = await fetch(apiPath("generate"), {
    method: "POST",
    body: formData,
    signal
  });
  const { data, text } = await readResponsePayload(response);

  if (!response.ok && response.status !== 202) {
    updateGeneratedResultStatus(index, "failed", data?.error || "", totalCount);
    renderPipeline(data?.pipeline);
    throw new Error(
      data?.error
        || `第 ${index + 1} 张生成提交失败：HTTP ${response.status} ${response.statusText} ${text?.slice(0, 300) || ""}`.trim()
    );
  }

  if (data?.jobId) {
    updateGeneratedResultStatus(index, data.status || "queued", "", totalCount);
    return pollGenerateJob(data.jobId, index, signal, totalCount);
  }

  if (data?.imageDataUrl) {
    return data;
  }

  renderPipeline(data?.pipeline);
  throw new Error(
    data?.error
      || `第 ${index + 1} 张生成提交失败：服务器没有返回任务编号 ${text?.slice(0, 300) || "空响应"}`.trim()
  );
}

function appendMaskPlacement(formData) {
  const placement = getMaskPlacement();
  formData.append("maskX", String(placement.x));
  formData.append("maskY", String(placement.y));
  formData.append("maskSize", String(placement.size));
  formData.append("maskShape", getMaskShape());
}

function appendQrTrimOptions(formData, { forceUseCurrentSelection: _forceUseCurrentSelection = false } = {}) {
  if (currentRect.width > 0 && currentRect.height > 0 && cropImage) {
    formData.append("qrTrimEnabled", "true");
    formData.append("qrTrimMode", "manual");
    const imageRect = canvasRectToImageRect(currentRect);
    
    formData.append("qrCropX", String(imageRect.x));
    formData.append("qrCropY", String(imageRect.y));
    formData.append("qrCropWidth", String(imageRect.width));
    formData.append("qrCropHeight", String(imageRect.height));
    
    console.log('发送裁剪坐标:', imageRect);
  } else {
    // 没有框选区域时不启用剪裁
    formData.append("qrTrimEnabled", "false");
  }
}

function scheduleMaskPreview() {
  clearTimeout(previewMaskTimer);
  previewMaskTimer = setTimeout(previewMaskNow, 150);
}

async function previewMaskNow({ silent = false } = {}) {
  const file = getActiveQrFile();
  const templateFile = getTemplateFile();
  const referenceFiles = Array.from(referenceInput.files ?? []).slice(0, 3);

  if (!file || !templateFile) {
    return;
  }

  if (!silent) {
    setStatus("正在生成遮罩预览（不调用 AI）...", "working");
  }

  const formData = new FormData();
  formData.append("qrImage", file);
  formData.append("templateImage", templateFile);
  appendMaskPlacement(formData);
  appendQrTrimOptions(formData);
  referenceFiles.forEach((referenceFile) => {
    formData.append("referenceImages", referenceFile);
  });

  try {
    const response = await fetch(apiPath("preview-mask"), {
      method: "POST",
      body: formData
    });
    const data = await response.json();
    if (!response.ok) {
      throw new Error(data.error || "遮罩预览失败");
    }

    const samePreview = data.debugOverlayDataUrl || data.collagePreviewDataUrl;
    if (samePreview) {
      if (debugOverlayPreview && debugOverlayEmpty) {
        debugOverlayPreview.src = samePreview;
        debugOverlayEmpty.hidden = true;
      }
      collagePreview.src = samePreview;
      collageEmpty.hidden = true;
      updateImageDownloadLink(collageDownloadLink, samePreview, "generation-context.png");
    }
    if (!silent) {
      setStatus("上下文图已生成（未调用 AI）。", "ready");
    }
    return data;
  } catch (error) {
    setStatus(error.message || "遮罩预览失败", "error");
  }
}

function buildClientPipeline({ file, templateFile, referenceFiles }) {
  return {
    phase: "client-before-submit",
    message: "客户端已准备提交。下面是本次请求在进入服务端前的状态。",
    files: {
      qrImage: file
        ? { name: file.name, type: file.type, size: file.size }
        : null,
      normalQrImage: normalQrInput.files?.[0]
        ? {
            name: normalQrInput.files[0].name,
            type: normalQrInput.files[0].type,
            size: normalQrInput.files[0].size
          }
        : null,
      templateImage: templateFile
        ? { name: templateFile.name, type: templateFile.type, size: templateFile.size }
        : null,
      referenceImages: referenceFiles.map((item) => ({
        name: item.name,
        type: item.type,
        size: item.size
      }))
    },
    fixedMask: { ...getMaskPlacement(), shape: getMaskShape(), note: "绝对坐标，不调用 AI，可在页面调节" },
    qrTrim: currentRect.width > 0 && currentRect.height > 0 && cropImage
      ? { enabled: true, mode: "manual", ...canvasRectToImageRect(currentRect) }
      : { enabled: false },
    positivePrompt: positivePromptInput.value,
    negativePrompt: negativePromptInput.value
  };
}

normalQrInput.addEventListener("change", () => {
  const file = normalQrInput.files?.[0];
  setGeneratedImageDataUrl(null);

  if (!file) {
    inputPreview.removeAttribute("src");
    updateImageDownloadLink(inputDownloadLink, null, "input-qrcode.png");
    inputEmpty.hidden = false;
    artisticPreview.removeAttribute("src");
    updateImageDownloadLink(artisticDownloadLink, null, "artistic-qrcode.png");
    artisticEmpty.hidden = false;
    cropCanvasContainer.style.display = 'none';
    cropEmpty.hidden = false;
    cropImage = null;
    workingQrFile = null;
    currentRect = { x: 0, y: 0, width: 0, height: 0 };
    updateCropCoords();
    return;
  }

  try {
    validateImageFile(file, "普通二维码图片");
  } catch (error) {
    normalQrInput.value = "";
    setStatus(error.message, "error");
    return;
  }

  const previewUrl = URL.createObjectURL(file);
  inputPreview.src = previewUrl;
  updateImageDownloadLink(inputDownloadLink, previewUrl, file.name || "input-qrcode.png");
  inputEmpty.hidden = true;
  artisticPreview.removeAttribute("src");
  updateImageDownloadLink(artisticDownloadLink, null, "artistic-qrcode.png");
  artisticEmpty.hidden = false;
  workingQrFile = null;
  setStatus('普通二维码已载入。可先框选，再点击"艺术化二维码"。', 'ready');
  
  // 初始化Canvas
  initCropCanvas(file, "normal");
});

// URL转二维码功能
convertUrlButton.addEventListener("click", async () => {
  const url = urlInput.value.trim();
  setGeneratedImageDataUrl(null);
  
  if (!url) {
    setStatus("请输入URL地址", "error");
    return;
  }
  
  // 简单验证URL格式
  if (!url.startsWith('http://') && !url.startsWith('https://')) {
    setStatus("URL必须以http://或https://开头", "error");
    return;
  }
  
  convertUrlButton.disabled = true;
  setStatus("正在将URL转换为二维码...", "working");
  
  try {
    const formData = new FormData();
    formData.append("url", url);
    
    const response = await fetch(apiPath("url-to-qr"), {
      method: "POST",
      body: formData
    });
    
    const data = await response.json();
    
    if (!response.ok) {
      throw new Error(data.error || "URL转二维码失败");
    }
    
    // 将生成的二维码显示在预览区
    inputPreview.src = data.imageDataUrl;
    updateImageDownloadLink(inputDownloadLink, data.imageDataUrl, data.filename || "input-qrcode.png");
    inputEmpty.hidden = true;
    
    // 将DataURL转换为File对象，以便后续处理
    const qrFile = await dataUrlToFile(
      data.imageDataUrl,
      data.filename || `qr-${Date.now()}.png`
    );
    
    // 模拟文件输入，使后续流程正常工作
    const dataTransfer = new DataTransfer();
    dataTransfer.items.add(qrFile);
    normalQrInput.files = dataTransfer.files;
    
    workingQrFile = null;
    setStatus('URL已转换为二维码。可先框选，再点击"艺术化二维码"。', 'success');
    
    // 初始化Canvas
    initCropCanvas(qrFile, "normal");
  } catch (error) {
    setStatus(error.message || "URL转二维码失败", "error");
  } finally {
    convertUrlButton.disabled = false;
  }
});

// 切换输入模式（已在HTML中处理）

stylizeButton.addEventListener("click", async () => {
  const file = normalQrInput.files?.[0];

  if (isStylizingQr) {
    setStatus("艺术化正在进行中，请等待当前任务完成。", "working");
    return;
  }

  if (!file) {
    setStatus("请先选择普通二维码图片。", "error");
    return;
  }

  try {
    validateImageFile(file, "普通二维码图片");
  } catch (error) {
    setStatus(error.message, "error");
    return;
  }

  isStylizingQr = true;
  stylizeButton.disabled = true;
  stylizeButton.setAttribute("aria-busy", "true");
  setStylizeProgress(12, "正在上传二维码");
  setStatus("正在通过 Sub2API 会话调用图像模型，期间不能重复点击艺术化。", "working");

  const formData = new FormData();
  formData.append("qrImage", file);
  appendQrTrimOptions(formData, { forceUseCurrentSelection: true });

  try {
    setStylizeProgress(38, "正在裁剪并提交");
    startStylizeWaitingProgress();
    const response = await fetch(apiPath("stylize-qr"), {
      method: "POST",
      body: formData
    });
    stopStylizeWaitingProgress();
    setStylizeProgress(76, "正在等待艺术化结果");
    const { data, text } = await readResponsePayload(response);

    if (!response.ok) {
      throw new Error(
        data?.error
          || `艺术化二维码失败：HTTP ${response.status} ${response.statusText} ${text?.slice(0, 300) || ""}`.trim()
      );
    }

    if (!data?.imageDataUrl) {
      throw new Error("艺术化接口没有返回图片，请稍后重试。");
    }

    setStylizeProgress(92, "正在写入预览");
    artisticPreview.src = data.imageDataUrl;
    updateImageDownloadLink(artisticDownloadLink, data.imageDataUrl, data.filename || "artistic-qrcode.png");
    artisticEmpty.hidden = true;
    workingQrFile = await dataUrlToFile(
      data.imageDataUrl,
      data.filename || `artistic-${Date.now()}.png`
    );
    initCropCanvas(workingQrFile, "artistic");
    setStatus(
      `艺术化二维码已生成（Sub2API 会话中继）。当前裁剪区已切到艺术化二维码。`,
      "success"
    );
    setStylizeProgress(100, "艺术化完成");
    scheduleMaskPreview();
  } catch (error) {
    stopStylizeWaitingProgress();
    setStatus(error.message || "艺术化二维码失败", "error");
    setStylizeProgress(100, "艺术化失败");
  } finally {
    isStylizingQr = false;
    stylizeButton.disabled = false;
    stylizeButton.removeAttribute("aria-busy");
    setTimeout(resetStylizeProgress, 1200);
  }
});

templateInput.addEventListener("change", () => {
  const file = templateInput.files?.[0];

  if (!file) {
    if (defaultTemplateFile) {
      templatePreview.src = "./blank.jpg";
      templateEmpty.hidden = true;
      templatePresetCard?.classList.remove("is-custom");
    } else {
      templatePreview.removeAttribute("src");
      templateEmpty.hidden = false;
    }
    return;
  }

  try {
    validateImageFile(file, "空白盘模板");
  } catch (error) {
    templateInput.value = "";
    templatePreview.removeAttribute("src");
    templateEmpty.hidden = false;
    setStatus(error.message, "error");
    return;
  }

  templatePreview.src = URL.createObjectURL(file);
  templateEmpty.hidden = true;
  templatePresetCard?.classList.add("is-custom");
  setStatus("空白盘模板已载入。", "ready");
  scheduleMaskPreview();
});

async function loadDefaultTemplate() {
  try {
    const response = await fetch("./blank.jpg");
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }

    const blob = await response.blob();
    defaultTemplateFile = new File([blob], "blank-porcelain-plate.jpg", {
      type: blob.type || "image/jpeg"
    });

    try {
      const transfer = new DataTransfer();
      transfer.items.add(defaultTemplateFile);
      templateInput.files = transfer.files;
    } catch {
      // getTemplateFile() keeps the preset usable when programmatic file assignment is unavailable.
    }

    templatePreview.src = "./blank.jpg";
    templateEmpty.hidden = true;
    templatePresetCard?.classList.add("is-selected");
    setStatus("默认空白青花瓷盘已就绪，请上传二维码开始制作。", "ready");
  } catch (error) {
    templatePresetCard?.classList.add("has-error");
    setStatus(`默认模板加载失败：${error.message}，请手动选择模板。`, "error");
  }
}

loadDefaultTemplate();

referenceInput.addEventListener("change", () => {
  const files = Array.from(referenceInput.files ?? []);
  referencePreview.innerHTML = "";

  if (files.length === 0) {
    referenceEmpty.hidden = false;
    return;
  }

  try {
    files.slice(0, 3).forEach((file) => validateImageFile(file, "参考图"));
  } catch (error) {
    referenceInput.value = "";
    referenceEmpty.hidden = false;
    setStatus(error.message, "error");
    return;
  }

  files.slice(0, 3).forEach((file) => {
    const img = document.createElement("img");
    img.src = URL.createObjectURL(file);
    img.alt = "参考图预览";
    img.className = "reference-thumb";
    referencePreview.appendChild(img);
  });

  referenceEmpty.hidden = true;
  scheduleMaskPreview();
});


[maskXInput, maskYInput, maskSizeInput].forEach((input) => {
  input?.addEventListener("input", scheduleMaskPreview);
});

maskShapeInputs.forEach((input) => {
  input.addEventListener("change", () => {
    document.querySelectorAll(".shape-option").forEach((option) => {
      const radio = option.querySelector('input[name="maskShape"]');
      option.classList.toggle("active", radio?.checked === true);
    });
    scheduleMaskPreview();
  });
});

function getCropDisplayBox(imageWidth, imageHeight, maxWidth, maxHeight) {
  const scale = Math.min(1, maxWidth / imageWidth, maxHeight / imageHeight);
  const width = imageWidth * scale;
  const height = imageHeight * scale;

  return {
    x: Math.max(0, (maxWidth - width) / 2),
    y: Math.max(0, (maxHeight - height) / 2),
    width,
    height
  };
}

function canvasRectToImageRect(rect) {
  if (!cropImage || cropDrawBox.width <= 0 || cropDrawBox.height <= 0) {
    return { x: 0, y: 0, width: 0, height: 0 };
  }

  const left = Math.max(cropDrawBox.x, Math.min(rect.x, cropDrawBox.x + cropDrawBox.width));
  const top = Math.max(cropDrawBox.y, Math.min(rect.y, cropDrawBox.y + cropDrawBox.height));
  const right = Math.max(left, Math.min(rect.x + rect.width, cropDrawBox.x + cropDrawBox.width));
  const bottom = Math.max(top, Math.min(rect.y + rect.height, cropDrawBox.y + cropDrawBox.height));
  const scaleX = cropImage.width / cropDrawBox.width;
  const scaleY = cropImage.height / cropDrawBox.height;

  return {
    x: Math.round((left - cropDrawBox.x) * scaleX),
    y: Math.round((top - cropDrawBox.y) * scaleY),
    width: Math.round((right - left) * scaleX),
    height: Math.round((bottom - top) * scaleY)
  };
}

// Canvas框选功能
function initCropCanvas(file, target = "normal") {
  cropTarget = target;
  cropCanvasContainer.style.display = 'grid';
  cropEmpty.hidden = true;
  
  const reader = new FileReader();
  reader.onload = (e) => {
    const img = new Image();
    img.onload = () => {
      cropImage = img;
      
      // 等待DOM更新后获取Canvas容器实际宽度
      requestAnimationFrame(() => {
        const containerWidth = cropCanvasContainer.clientWidth - 40; // 减去padding
        const maxWidth = Math.min(1000, containerWidth);
        const maxHeight = Math.min(720, Math.max(320, window.innerHeight * 0.72));
        cropDrawBox = getCropDisplayBox(img.width, img.height, maxWidth, maxHeight);
        const displayWidth = cropDrawBox.x * 2 + cropDrawBox.width;
        const displayHeight = cropDrawBox.y * 2 + cropDrawBox.height;
        
        // 提高Canvas分辨率（Retina屏支持）
        const dpr = window.devicePixelRatio || 1;
        
        cropCanvas.width = displayWidth * dpr;
        cropCanvas.height = displayHeight * dpr;
        cropCanvas.style.width = displayWidth + 'px';
        cropCanvas.style.height = displayHeight + 'px';
        
        cropContext = cropCanvas.getContext('2d');
        cropContext.scale(dpr, dpr); // 缩放上下文以匹配DPR
        
        drawCropCanvas();
        
        // 默认选择整个图片区域
        currentRect = { ...cropDrawBox };
        drawCropCanvas();
        updateCropCoords();
      });
    };
    img.src = e.target.result;
  };
  reader.readAsDataURL(file);
}

function drawCropCanvas() {
  if (!cropContext || !cropImage) return;
  
  // 清空画布
  cropContext.clearRect(0, 0, cropCanvas.width, cropCanvas.height);
  
  // 绘制图片（降低透明度）
  cropContext.globalAlpha = 0.5;
  cropContext.imageSmoothingEnabled = true;
  cropContext.imageSmoothingQuality = 'high';
  cropContext.drawImage(
    cropImage,
    0,
    0,
    cropImage.width,
    cropImage.height,
    cropDrawBox.x,
    cropDrawBox.y,
    cropDrawBox.width,
    cropDrawBox.height
  );
  cropContext.globalAlpha = 1.0;
  
  // 绘制选择框
  if (currentRect.width > 0 && currentRect.height > 0) {
    // 绘制半透明遮罩（选区外）
    cropContext.fillStyle = 'rgba(0, 0, 0, 0.6)';
    cropContext.fillRect(0, 0, parseFloat(cropCanvas.style.width), parseFloat(cropCanvas.style.height));
    
    // 清除选区内的遮罩，显示原图
    cropContext.save();
    cropContext.beginPath();
    cropContext.rect(currentRect.x, currentRect.y, currentRect.width, currentRect.height);
    cropContext.clip();
    cropContext.globalAlpha = 1.0;
    cropContext.drawImage(
      cropImage,
      0,
      0,
      cropImage.width,
      cropImage.height,
      cropDrawBox.x,
      cropDrawBox.y,
      cropDrawBox.width,
      cropDrawBox.height
    );
    cropContext.restore();
    
    // 绘制边框（更粗更清晰）
    cropContext.strokeStyle = '#ff6b35';
    cropContext.lineWidth = 4;
    cropContext.setLineDash([10, 5]);
    cropContext.lineCap = 'round';
    cropContext.lineJoin = 'round';
    cropContext.strokeRect(currentRect.x, currentRect.y, currentRect.width, currentRect.height);
    cropContext.setLineDash([]);
    
    // 绘制四个角的控制点（更大更明显）
    const cornerSize = 12;
    cropContext.fillStyle = '#ff6b35';
    cropContext.shadowColor = 'rgba(0, 0, 0, 0.5)';
    cropContext.shadowBlur = 4;
    cropContext.shadowOffsetX = 2;
    cropContext.shadowOffsetY = 2;
    
    // 左上角
    cropContext.fillRect(currentRect.x - cornerSize/2, currentRect.y - cornerSize/2, cornerSize, cornerSize);
    // 右上角
    cropContext.fillRect(currentRect.x + currentRect.width - cornerSize/2, currentRect.y - cornerSize/2, cornerSize, cornerSize);
    // 左下角
    cropContext.fillRect(currentRect.x - cornerSize/2, currentRect.y + currentRect.height - cornerSize/2, cornerSize, cornerSize);
    // 右下角
    cropContext.fillRect(currentRect.x + currentRect.width - cornerSize/2, currentRect.y + currentRect.height - cornerSize/2, cornerSize, cornerSize);
    
    cropContext.shadowColor = 'transparent';
    cropContext.shadowBlur = 0;
    cropContext.shadowOffsetX = 0;
    cropContext.shadowOffsetY = 0;
    
    // 绘制尺寸提示
    if (currentRect.width > 50 && currentRect.height > 30) {
      cropContext.fillStyle = 'rgba(255, 107, 53, 0.9)';
      cropContext.font = 'bold 14px Arial';
      cropContext.textAlign = 'center';
      cropContext.textBaseline = 'middle';
      
      const imageRect = canvasRectToImageRect(currentRect);
      const realWidth = imageRect.width;
      const realHeight = imageRect.height;
      
      const label = `${realWidth} × ${realHeight}`;
      const labelX = currentRect.x + currentRect.width / 2;
      const labelY = currentRect.y + currentRect.height / 2;
      
      // 文字背景
      const textMetrics = cropContext.measureText(label);
      const padding = 8;
      cropContext.fillStyle = 'rgba(0, 0, 0, 0.7)';
      cropContext.fillRect(
        labelX - textMetrics.width / 2 - padding,
        labelY - 10 - padding,
        textMetrics.width + padding * 2,
        20 + padding * 2
      );
      
      // 文字
      cropContext.fillStyle = '#fff';
      cropContext.fillText(label, labelX, labelY);
    }
  }
}

function updateCropCoords() {
  if (cropImage && currentRect.width > 0 && currentRect.height > 0) {
    const imageRect = canvasRectToImageRect(currentRect);
    
    cropXSpan.textContent = imageRect.x;
    cropYSpan.textContent = imageRect.y;
    cropWidthSpan.textContent = imageRect.width;
    cropHeightSpan.textContent = imageRect.height;
  } else {
    cropXSpan.textContent = '0';
    cropYSpan.textContent = '0';
    cropWidthSpan.textContent = '0';
    cropHeightSpan.textContent = '0';
  }
}

// Canvas鼠标事件处理
let isDragging = false;
let dragStartX = 0;
let dragStartY = 0;

cropCanvas.addEventListener('mousedown', (e) => {
  e.preventDefault();
  isDragging = true;
  const rect = cropCanvas.getBoundingClientRect();
  
  const pointerX = (e.clientX - rect.left) * (parseFloat(cropCanvas.style.width) / rect.width);
  const pointerY = (e.clientY - rect.top) * (parseFloat(cropCanvas.style.height) / rect.height);
  dragStartX = Math.max(cropDrawBox.x, Math.min(pointerX, cropDrawBox.x + cropDrawBox.width));
  dragStartY = Math.max(cropDrawBox.y, Math.min(pointerY, cropDrawBox.y + cropDrawBox.height));
  
  currentRect = { 
    x: dragStartX, 
    y: dragStartY, 
    width: 0, 
    height: 0 
  };
  
  drawCropCanvas();
});

cropCanvas.addEventListener('mousemove', (e) => {
  if (!isDragging) return;
  e.preventDefault();
  
  const rect = cropCanvas.getBoundingClientRect();
  const displayWidth = parseFloat(cropCanvas.style.width) || rect.width;
  const displayHeight = parseFloat(cropCanvas.style.height) || rect.height;
  
  const rawCurrentX = (e.clientX - rect.left) * (displayWidth / rect.width);
  const rawCurrentY = (e.clientY - rect.top) * (displayHeight / rect.height);
  const currentX = Math.max(cropDrawBox.x, Math.min(rawCurrentX, cropDrawBox.x + cropDrawBox.width));
  const currentY = Math.max(cropDrawBox.y, Math.min(rawCurrentY, cropDrawBox.y + cropDrawBox.height));
  
  // 计算矩形（支持任意方向拖动）
  const newX = Math.min(dragStartX, currentX);
  const newY = Math.min(dragStartY, currentY);
  const newWidth = Math.abs(currentX - dragStartX);
  const newHeight = Math.abs(currentY - dragStartY);
  
  // 确保不超出Canvas边界
  currentRect.x = Math.max(cropDrawBox.x, newX);
  currentRect.y = Math.max(cropDrawBox.y, newY);
  currentRect.width = Math.min(newWidth, cropDrawBox.x + cropDrawBox.width - currentRect.x);
  currentRect.height = Math.min(newHeight, cropDrawBox.y + cropDrawBox.height - currentRect.y);
  
  drawCropCanvas();
  updateCropCoords();
});

cropCanvas.addEventListener('mouseup', (e) => {
  e.preventDefault();
  isDragging = false;
  
  // 确保最小选择区域
  if (currentRect.width < 10 || currentRect.height < 10) {
    // 如果选择区域太小，重置为全选
    currentRect = { ...cropDrawBox };
    drawCropCanvas();
    updateCropCoords();
    setStatus("选择区域太小，已重置为全选", "working");
  } else if (currentRect.width > 0 && currentRect.height > 0) {
    // 输出调试信息
    const displayWidth = parseFloat(cropCanvas.style.width);
    const displayHeight = parseFloat(cropCanvas.style.height);
    console.log('框选完成:', {
      canvas: { width: cropCanvas.width, height: cropCanvas.height },
      display: { width: displayWidth, height: displayHeight },
      rect: { ...currentRect },
      image: { width: cropImage.width, height: cropImage.height }
    });
    
    scheduleMaskPreview();
    setStatus("框选完成，可以调整或点击生成", "success");
  }
});

cropCanvas.addEventListener('mouseleave', () => {
  isDragging = false;
});

// 触摸设备支持
cropCanvas.addEventListener('touchstart', (e) => {
  e.preventDefault();
  const touch = e.touches[0];
  const mouseEvent = new MouseEvent('mousedown', {
    clientX: touch.clientX,
    clientY: touch.clientY
  });
  cropCanvas.dispatchEvent(mouseEvent);
});

cropCanvas.addEventListener('touchmove', (e) => {
  e.preventDefault();
  const touch = e.touches[0];
  const mouseEvent = new MouseEvent('mousemove', {
    clientX: touch.clientX,
    clientY: touch.clientY
  });
  cropCanvas.dispatchEvent(mouseEvent);
});

cropCanvas.addEventListener('touchend', (e) => {
  e.preventDefault();
  const mouseEvent = new MouseEvent('mouseup', {});
  cropCanvas.dispatchEvent(mouseEvent);
});

document.querySelectorAll("[data-nudge]").forEach((button) => {
  button.addEventListener("click", () => {
    const direction = button.dataset.nudge;
    if (direction === "left") maskXInput.value = String(Number(maskXInput.value) - MASK_NUDGE_STEP);
    if (direction === "right") maskXInput.value = String(Number(maskXInput.value) + MASK_NUDGE_STEP);
    if (direction === "up") maskYInput.value = String(Number(maskYInput.value) - MASK_NUDGE_STEP);
    if (direction === "down") maskYInput.value = String(Number(maskYInput.value) + MASK_NUDGE_STEP);
    scheduleMaskPreview();
  });
});

document.querySelectorAll("[data-size]").forEach((button) => {
  button.addEventListener("click", () => {
    const delta = button.dataset.size === "larger" ? MASK_NUDGE_STEP : -MASK_NUDGE_STEP;
    maskSizeInput.value = String(Math.max(100, Number(maskSizeInput.value) + delta));
    scheduleMaskPreview();
  });
});

generateButton.addEventListener("click", async () => {
  const file = getActiveQrFile();
  const templateFile = getTemplateFile();
  const referenceFiles = Array.from(referenceInput.files ?? []).slice(0, 3);
  setGeneratedImageDataUrl(null);

  if (!file) {
    setStatus("请先选择普通二维码并点击“艺术化二维码”。", "error");
    return;
  }

  if (!templateFile) {
    templateEmpty.hidden = false;
    setStatus("请先选择空白盘模板，生成上下文图后再点击生成二维码。", "error");
    return;
  }

  generateButton.disabled = true;
  revisedPromptNode.textContent = FIXED_PROMPT_TEXT;
  pipelineThoughtsNode.textContent = FIXED_PIPELINE_TEXT;
  renderPipeline(buildClientPipeline({ file, templateFile, referenceFiles }));

  try {
    setStatus("先生成实际提交上下文图（不调用 AI）...", "working");
    const previewData = await previewMaskNow({ silent: true });
    if (!previewData?.debugOverlayDataUrl && !previewData?.collagePreviewDataUrl) {
      setStatus("空白盘模板上下文图未生成，不能继续生成二维码。", "error");
      generateButton.disabled = false;
      return;
    }
  } catch (error) {
    setStatus(error.message || "遮罩预览失败", "error");
    generateButton.disabled = false;
    return;
  }

  const generateCount = getGenerateCount();
  setStatus(`上下文图已就绪，正在按稳定队列生成 ${generateCount} 张最终图，完成一张会立即显示...`, "working");
  generatedResults = Array.from({ length: generateCount }, () => ({ status: "queued" }));
  selectedGeneratedImageIndex = -1;
  renderGeneratedResultCards(generateCount);

  const generateController = new AbortController();
  const generateTimeoutMs = GENERATE_TIMEOUT_MS * generateCount;
  const generateTimeout = setTimeout(() => generateController.abort(), generateTimeoutMs);
  let completedCount = 0;

  try {
    const settledResults = await Promise.allSettled(
      Array.from({ length: generateCount }, async (_, index) => {
        const formData = new FormData();
        formData.append("qrImage", file);
        appendMaskPlacement(formData);
        appendQrTrimOptions(formData);
        formData.append("templateImage", templateFile);
        referenceFiles.forEach((referenceFile) => {
          formData.append("referenceImages", referenceFile);
        });
        formData.append("positivePrompt", positivePromptInput.value);
        formData.append("negativePrompt", negativePromptInput.value);
        formData.append("async", "true");

        let data;
        try {
          data = await submitGenerateJob(formData, index, generateController.signal, generateCount);
        } catch (error) {
          updateGeneratedResultStatus(index, "failed", error?.message || "", generateCount);
          throw error;
        }

        const result = {
          ...data,
          filename: `generated-qrcode-${index + 1}.png`
        };

        appendGeneratedResult(result, index, generateCount);
        completedCount += 1;
        setStatus(`已完成 ${completedCount}/${generateCount} 张，后续图片仍在队列中生成...`, "working");
        return result;
      })
    );

    const results = settledResults
      .filter((result) => result.status === "fulfilled")
      .map((result) => result.value);
    const failures = settledResults
      .filter((result) => result.status === "rejected")
      .map((result) => result.reason);

    if (results.length === 0) {
      throw failures[0] || new Error("生成失败。");
    }

    revisedPromptNode.textContent = FIXED_PROMPT_TEXT;
    renderPipeline(results[0]?.pipeline);

    if (failures.length > 0) {
      setStatus(`已完成 ${results.length}/${generateCount} 张，${failures.length} 张失败。已完成图片可以先使用或上传。`, "error");
    } else {
      setStatus(`生成完成，共 ${results.length} 张。点击下方结果可切换选中图，再上传到微信素材库。`, "success");
    }
  } catch (error) {
    if (error?.name === "AbortError") {
      setStatus(`生成超时：单张最多等待 ${Math.round(GENERATE_TIMEOUT_MS / 60000)} 分钟，当前批次最多等待 ${Math.round(generateTimeoutMs / 60000)} 分钟。`, "error");
    } else {
      setStatus(error.message || "生成失败。", "error");
    }
  } finally {
    clearTimeout(generateTimeout);
    generateButton.disabled = false;
  }
});

uploadWechatMaterialButton?.addEventListener("click", async () => {
  const filename = wechatUploadNameInput.value.trim();
  const selectedResult = getSelectedGeneratedResult();

  if (!selectedResult?.imageDataUrl) {
    setWechatStatus("生成完成后才可以上传到素材库。", "error");
    return;
  }
  if (!filename) {
    setWechatStatus("请填写生成图片结果的名字。", "error");
    return;
  }
  if ((selectedResult.bytes ?? generatedImageBytes) > WECHAT_IMAGE_MAX_BYTES) {
    setWechatStatus("生成图片超过微信永久图片素材 10MB 限制，请先压缩为微信素材大小。", "error");
    setWechatResult({
      size: formatBytes(selectedResult.bytes ?? generatedImageBytes),
      limit: "10 MB",
      plan: [
        "优先使用 PNG 压缩，不改变二维码结构。",
        "如果压缩后仍超限，再按比例降低整张图尺寸，保留二维码整体几何关系。",
        "仍无法达标时，建议重新生成更低细节或更小尺寸图片。"
      ]
    });
    return;
  }

  setButtonBusy(uploadWechatMaterialButton, "上传中...");
  beginWechatOperation("正在上传图片到微信素材库...", {
    action: "upload_material",
    endpoint: apiPath("wechat/material/upload"),
    filename
  });

  try {
    const accessToken = await getValidWechatAccessToken();
    setWechatOperationProgress(58, "正在读取选中的生成图片...");
    const mediaFile = await getGeneratedMediaFile(filename, selectedResult);
    const uploadFilename = mediaFile.name;
    const formData = new FormData();
    formData.append("accessToken", accessToken);
    formData.append("filename", uploadFilename);
    formData.append("media", mediaFile, uploadFilename);

    setWechatOperationProgress(72, "正在上传图片到微信素材库...");
    const response = await fetch(apiPath("wechat/material/upload"), {
      method: "POST",
      body: formData
    });
    const { data, text } = await readResponsePayload(response);

    if (!response.ok) {
      throw new Error(
        data?.error
          || `上传素材失败：HTTP ${response.status} ${response.statusText} ${text?.slice(0, 300) || ""}`.trim()
      );
    }

    if (!data) {
      throw new Error(`服务器返回了非 JSON 内容：${text?.slice(0, 300) || "空响应"}`);
    }

    setWechatResult(data);
    setWechatStatus("素材上传完成。", "success");
    finishWechatOperation("素材上传完成");
    if (wechatUploadNameInput && wechatUploadNameInput.value.trim() !== uploadFilename) {
      wechatUploadNameInput.value = uploadFilename;
    }
  } catch (error) {
    setWechatStatus(error.message || "上传素材失败。", "error");
    finishWechatOperation("上传素材失败", "error");
  } finally {
    clearButtonBusy(uploadWechatMaterialButton);
    updateWechatUploadState();
  }
});

prepareWechatImageButton?.addEventListener("click", async () => {
  const selectedResult = getSelectedGeneratedResult();
  if (!selectedResult?.imageDataUrl) {
    setWechatStatus("生成完成后才可以压缩图片。", "error");
    return;
  }

  setButtonBusy(prepareWechatImageButton, "压缩中...");
  startWechatOperation("正在压缩为微信素材大小...");
  setWechatStatus("正在压缩为微信素材大小...", "working");

  try {
    const filename = wechatUploadNameInput.value.trim() || "generated-qrcode.png";
    setWechatOperationProgress(32, "正在读取待处理图片...");
    const mediaFile = await getGeneratedMediaFile(filename, selectedResult);
    const formData = new FormData();
    formData.append("media", mediaFile, mediaFile.name);

    setWechatOperationProgress(58, "正在提交压缩处理...");
    const response = await fetch(apiPath("wechat/material/prepare"), {
      method: "POST",
      body: formData
    });
    const { data, text } = await readResponsePayload(response);

    if (!response.ok || !data?.imageDataUrl) {
      throw new Error(
        data?.error
          || `压缩失败：HTTP ${response.status} ${response.statusText} ${text?.slice(0, 300) || ""}`.trim()
      );
    }

    const preparedFilename = normalizeFilenameForMime(filename, data.mimeType);
    if (wechatUploadNameInput && wechatUploadNameInput.value.trim() === filename) {
      wechatUploadNameInput.value = preparedFilename;
    }

    outputPreview.src = data.imageDataUrl;
    const preparedIndex = Math.max(selectedGeneratedImageIndex, 0);
    if (preparedIndex >= 0) {
      generatedResults[preparedIndex] = {
        ...selectedResult,
        imageDataUrl: data.imageDataUrl,
        filename: preparedFilename,
        bytes: getDataUrlByteSize(data.imageDataUrl)
      };
    }
    setGeneratedImageDataUrl(data.imageDataUrl, {
      filename: preparedFilename,
      index: preparedIndex,
      syncResults: generatedResults.length === 0
    });
    renderBatchGeneratedResults(generatedResults);
    selectGeneratedResult(preparedIndex);
    updateImageDownloadLink(outputDownloadLink, data.imageDataUrl, preparedFilename);
    setWechatResult({
      originalSize: formatBytes(data.originalBytes),
      preparedSize: formatBytes(data.preparedBytes),
      filename: preparedFilename,
      action: data.action,
      recommendations: data.recommendations
    });
    setWechatStatus("图片已处理为微信素材可上传大小。", "success");
    finishWechatOperation("图片压缩完成");
  } catch (error) {
    setWechatStatus(error.message || "压缩图片失败。", "error");
    finishWechatOperation("压缩图片失败", "error");
  } finally {
    clearButtonBusy(prepareWechatImageButton);
  }
});

searchWechatMaterialButton?.addEventListener("click", async () => {
  const searchName = wechatMaterialSearchNameInput.value.trim();

  if (!searchName) {
    setWechatStatus("请输入素材名称。", "error");
    return;
  }

  setButtonBusy(searchWechatMaterialButton, "查询中...");
  beginWechatOperation("正在获取素材列表并匹配名称...", {
    action: "list_and_match_materials",
    endpoint: apiPath("wechat/material/list"),
    offset: WECHAT_MATERIAL_LIST_OFFSET,
    count: WECHAT_MATERIAL_LIST_COUNT,
    name: searchName
  });
  setWechatPreviewLoading("正在查询素材列表，请稍候...");

  try {
    const accessToken = await getValidWechatAccessToken();
    setWechatOperationProgress(58, "正在拉取微信永久图片素材列表...");
    const { items } = await fetchWechatMaterialItems(accessToken);
    setWechatOperationProgress(78, "正在按名称筛选素材...");
    const matchedItems = filterWechatMaterialsByName(items, searchName);

    renderWechatMaterialRows(matchedItems);
    clearWechatPreviewLoading();
    if (wechatMaterialPreview) {
      wechatMaterialPreview.removeAttribute("src");
    }
    if (wechatMaterialPreviewEmpty) {
      wechatMaterialPreviewEmpty.hidden = false;
    }

    if (matchedItems.length === 0) {
      if (wechatMaterialPreviewEmpty) {
        wechatMaterialPreviewEmpty.textContent = "未找到匹配素材，请调整名称后重新查询。";
      }
      setWechatStatus("未在素材列表中找到包含这个名称的素材。", "error");
      finishWechatOperation("未找到匹配素材", "error");
      return;
    }

    if (wechatMaterialPreviewEmpty) {
      wechatMaterialPreviewEmpty.textContent = `已找到 ${matchedItems.length} 个匹配素材，点击表格中的查看图片后显示。`;
    }
    setWechatStatus(`已找到 ${matchedItems.length} 个匹配素材，请点击表格中的“查看图片”。`, "success");
    finishWechatOperation("素材查询完成");
  } catch (error) {
    setWechatStatus(error.message || "获取素材图片失败。", "error");
    clearWechatPreviewLoading();
    if (wechatMaterialPreview) {
      wechatMaterialPreview.removeAttribute("src");
    }
    if (wechatMaterialPreviewEmpty) {
      wechatMaterialPreviewEmpty.hidden = false;
      wechatMaterialPreviewEmpty.textContent = "查询素材失败，请检查凭据或网络后重试。";
    }
    finishWechatOperation("查询素材失败", "error");
  } finally {
    clearButtonBusy(searchWechatMaterialButton);
  }
});
