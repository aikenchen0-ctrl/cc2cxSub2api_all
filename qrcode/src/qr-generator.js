import { APP_CONFIG } from "./config.js";
import { buildSquareQrPlatePreview, trimQrWhiteBorder } from "./mask-processor.js";
import { qrcodeRequestHeaders } from "./sso.js";
import sharp from "sharp";

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function buildTaskUrl(taskId) {
  return `${APP_CONFIG.apiBaseUrl}${APP_CONFIG.taskEndpointTemplate.replace("{taskId}", taskId)}`;
}

class GenerationSubmitError extends Error {
  constructor(message, pipeline) {
    super(message);
    this.name = "GenerationSubmitError";
    this.pipeline = pipeline;
  }
}

async function fetchTaskResult({ taskId, fetchImpl, apiBaseUrl, userId, apiKey }) {
  for (let attempt = 1; attempt <= APP_CONFIG.taskPollMaxAttempts; attempt += 1) {
    let response;

    try {
      response = await fetchImpl(`${apiBaseUrl}${APP_CONFIG.taskEndpointTemplate.replace("{taskId}", taskId)}`, {
        method: "GET",
        headers: qrcodeRequestHeaders(userId, apiKey)
      });
    } catch (error) {
      const causeCode = error?.cause?.code ? ` ${error.cause.code}` : "";
      const causeMessage = error?.cause?.message || error?.message || "未知错误";

      throw new Error(
        `任务查询失败: ${buildTaskUrl(taskId)}${causeCode} ${causeMessage}`.trim()
      );
    }

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`任务查询失败: ${response.status} ${errorText}`);
    }

    const data = await response.json();
    const state = data?.state;

    if (state === "succeeded") {
      return data;
    }

    if (["failed", "error", "cancelled"].includes(state)) {
      throw new Error(`生成失败: 任务状态为 ${state} ${JSON.stringify(data)}`);
    }

    await sleep(APP_CONFIG.taskPollIntervalMs);
  }

  throw new Error("生成失败: 任务轮询超时");
}

export async function normalizeInputImage(imageBuffer) {
  return sharp(imageBuffer)
    .rotate()
    .resize(APP_CONFIG.maxInputImageSize, APP_CONFIG.maxInputImageSize, {
      fit: "inside",
      withoutEnlargement: true
    })
    .png()
    .toBuffer();
}

async function buildArtisticQrCanvas(imageBuffer, size = 1024, quietZoneRatio = 0) {
  // Do not send the small 128px preview used by the collage path. A matching
  // square canvas gives the image model the whole QR and keeps its placement
  // stable instead of allowing it to become a tiny QR embedded in artwork.
  const quietZone = Math.max(0, Math.min(0.25, Number(quietZoneRatio) || 0));
  const inset = Math.round(size * quietZone);
  const innerSize = Math.max(1, size - inset * 2);
  const fitted = await sharp(imageBuffer)
    .rotate()
    .resize(innerSize, innerSize, {
      fit: "contain",
      background: { r: 255, g: 255, b: 255, alpha: 1 }
    })
    .png()
    .toBuffer();

  if (inset === 0) {
    return fitted;
  }

  return sharp({
    create: {
      width: size,
      height: size,
      channels: 4,
      background: { r: 255, g: 255, b: 255, alpha: 1 }
    }
  })
    .composite([{ input: fitted, left: inset, top: inset }])
    .png()
    .toBuffer();
}

async function buildTransparentMaskLike(imageBuffer) {
  const metadata = await sharp(imageBuffer).metadata();
  return sharp({
    create: {
      width: metadata.width || 1024,
      height: metadata.height || 1024,
      channels: 4,
      background: { r: 0, g: 0, b: 0, alpha: 0 }
    }
  }).png().toBuffer();
}

async function rebuildArtisticQrFromModules(sourceQrBuffer, generatedBuffer) {
  const size = 1024;
  // sourceQrBuffer is already the exact canvas submitted to the edit API;
  // do not add a second quiet zone while rebuilding the module mask.
  const sourceCanvas = await buildArtisticQrCanvas(sourceQrBuffer, size);
  const generatedCanvas = await buildArtisticQrCanvas(generatedBuffer, size);
  const { data, info } = await sharp(sourceCanvas)
    .greyscale()
    .raw()
    .toBuffer({ resolveWithObject: true });
  const alpha = Buffer.alloc(info.width * info.height);

  // Keep the exact source QR module geometry. Anti-aliased dark pixels are
  // included so the finder patterns and module edges remain scan-friendly.
  for (let index = 0; index < alpha.length; index += 1) {
    alpha[index] = data[index] < 245 ? 255 : 0;
  }

  const moduleMask = await sharp({
    create: {
      width: info.width,
      height: info.height,
      channels: 3,
      background: { r: 255, g: 255, b: 255 }
    }
  })
    .joinChannel(alpha, {
      raw: { width: info.width, height: info.height, channels: 1 }
    })
    .png()
    .toBuffer();

  // Give every source module a dark porcelain-blue base. The generated
  // texture is multiplied into it, so a pale/white model response cannot
  // erase the QR contrast and make the final code unsensible.
  const texturedCanvas = await sharp({
    create: {
      width: size,
      height: size,
      channels: 4,
      background: { r: 28, g: 96, b: 126, alpha: 1 }
    }
  })
    .composite([{ input: generatedCanvas, blend: "multiply" }])
    .png()
    .toBuffer();

  const artisticModules = await sharp(texturedCanvas)
    .ensureAlpha()
    .composite([{ input: moduleMask, blend: "dest-in" }])
    .png()
    .toBuffer();

  return sharp({
    create: {
      width: size,
      height: size,
      channels: 4,
      background: { r: 255, g: 255, b: 255, alpha: 1 }
    }
  })
    .composite([{ input: artisticModules }])
    .png()
    .toBuffer();
}

export async function buildSingleContextImage(primaryImageBuffer, referenceBuffers) {
  if (referenceBuffers.length === 0) {
    return {
      uploadBuffer: primaryImageBuffer,
      collagePreviewDataUrl: null
    };
  }

  const primaryTile = await sharp(primaryImageBuffer)
    .resize(96, 96, {
      fit: "contain",
      background: { r: 255, g: 255, b: 255, alpha: 1 }
    })
    .png()
    .toBuffer();

  const refTiles = await Promise.all(
    referenceBuffers.slice(0, 2).map((buffer) =>
      sharp(buffer)
        .resize(48, 48, {
          fit: "cover"
        })
        .png()
        .toBuffer()
    )
  );

  const composite = [{ input: primaryTile, left: 0, top: 16 }];
  if (refTiles[0]) composite.push({ input: refTiles[0], left: 80, top: 0 });
  if (refTiles[1]) composite.push({ input: refTiles[1], left: 80, top: 80 });

  const collageBuffer = await sharp({
    create: {
      width: 128,
      height: 128,
      channels: 4,
      background: { r: 255, g: 255, b: 255, alpha: 1 }
    }
  })
    .composite(composite)
    .png()
    .toBuffer();

  return {
    uploadBuffer: collageBuffer,
    collagePreviewDataUrl: `data:image/png;base64,${collageBuffer.toString("base64")}`
  };
}

async function resizeUploadCandidate(buffer, size) {
  return sharp(buffer)
    .resize(size, size, {
      fit: "inside",
      withoutEnlargement: true,
      background: { r: 255, g: 255, b: 255, alpha: 1 }
    })
    .png()
    .toBuffer();
}

function appendPngFile(formData, name, buffer, filename) {
  const blob = new Blob([buffer], { type: "image/png" });
  formData.append(name, blob, filename);
}

function buildImageEditUrl(apiBaseUrl) {
  const base = String(apiBaseUrl || "").trim().replace(/\/+$/, "");
  const endpoint = String(APP_CONFIG.imageEditEndpoint || "").trim();

  // Callers may provide either the public host (http://sub2api:8080) or the
  // OpenAI-compatible base (http://sub2api:8080/v1). Keep the endpoint
  // exactly once in both cases.
  if (base.endsWith("/v1") && endpoint.startsWith("/v1/")) {
    return `${base}${endpoint.slice(3)}`;
  }
  return `${base}${endpoint}`;
}

async function submitImageEditOnce({
  fetchImpl,
  submitUrl,
  contextImageBuffer,
  editMaskBuffer,
  positivePrompt,
  negativePrompt,
  inputFidelity,
  pipeline,
  userId,
  apiKey
}) {
  const formData = new FormData();
  formData.append("model", APP_CONFIG.model);
  formData.append("size", APP_CONFIG.qrOutputSize);
  formData.append("prompt", positivePrompt || "");
  if (negativePrompt) {
    formData.append("negative_prompt", negativePrompt);
  }
  if (inputFidelity) {
    formData.append("input_fidelity", inputFidelity);
  }
  appendPngFile(formData, "image", contextImageBuffer, "context.png");
  appendPngFile(formData, "mask", editMaskBuffer, "mask.png");

  const attempt = {
    label: "single-image2-edit-submit",
    endpoint: submitUrl,
    imageBytes: contextImageBuffer.length,
    maskBytes: editMaskBuffer.length,
    negativePromptUsed: Boolean(negativePrompt)
  };

  let response;
  try {
    response = await fetchImpl(submitUrl, {
      method: "POST",
      headers: qrcodeRequestHeaders(userId, apiKey),
      body: formData
    });
  } catch (error) {
    attempt.ok = false;
    attempt.error = `${error?.cause?.code ? `${error.cause.code} ` : ""}${error?.cause?.message || error?.message || "未知错误"}`;
    throw new GenerationSubmitError(`扩图失败: edits 接口提交失败 ${JSON.stringify(attempt)}`, {
      ...pipeline,
      submitAttempts: [attempt]
    });
  }

  const responseText = await response.text();
  attempt.httpStatus = response.status;
  attempt.responsePreview = responseText.slice(0, 500);
  attempt.ok = response.ok;

  if (!response.ok) {
    throw new GenerationSubmitError(`扩图失败: edits 接口提交失败 ${JSON.stringify(attempt)}`, {
      ...pipeline,
      submitAttempts: [attempt]
    });
  }

  let submitData;
  try {
    submitData = JSON.parse(responseText);
  } catch {
    throw new GenerationSubmitError(`扩图失败: edits 接口返回不是 JSON ${responseText.slice(0, 300)}`, {
      ...pipeline,
      submitAttempts: [attempt]
    });
  }

  const first = submitData?.data?.[0] || submitData?.data?.images?.[0] || submitData?.images?.[0];
  const b64Json = first?.b64_json || submitData?.b64_json;
  const imageUrl = first?.url || submitData?.url;

  if (b64Json) {
    return {
      imageBuffer: Buffer.from(b64Json, "base64"),
      revisedPrompt: first?.revised_prompt || submitData?.revised_prompt || positivePrompt,
      selectedAttempt: attempt,
      attempts: [attempt]
    };
  }

  if (imageUrl) {
    const imageResponse = await fetchImpl(imageUrl);
    if (!imageResponse.ok) {
      const errorText = await imageResponse.text();
      throw new Error(`图片下载失败: ${imageResponse.status} ${errorText}`);
    }
    return {
      imageBuffer: Buffer.from(await imageResponse.arrayBuffer()),
      revisedPrompt: first?.revised_prompt || submitData?.revised_prompt || positivePrompt,
      selectedAttempt: attempt,
      attempts: [attempt]
    };
  }

  throw new GenerationSubmitError(`扩图失败: edits 接口没有返回图片 ${responseText.slice(0, 300)}`, {
    ...pipeline,
    submitAttempts: [attempt]
  });
}

export async function generateQrArtwork({
  imageBuffer,
  mimeType: _mimeType,
  templateImage = null,
  maskPlacement = {},
  qrTrim = {},
  referenceImages = [],
  positivePrompt,
  negativePrompt,
  artisticQr = false,
  fetchImpl = fetch,
  apiBaseUrl = APP_CONFIG.apiBaseUrl,
  userId = "",
  apiKey = ""
}) {
  const isArtisticQr = Boolean(artisticQr && !templateImage?.buffer);
  const trimmedInput = templateImage?.buffer
    ? null
    : await trimQrWhiteBorder(imageBuffer, qrTrim);
  const effectivePositivePrompt = positivePrompt
    || (isArtisticQr ? APP_CONFIG.artQr.sub2apiPrompt : APP_CONFIG.defaultPositivePrompt);
  const effectiveNegativePrompt = negativePrompt
    || (isArtisticQr ? APP_CONFIG.artQr.sub2apiNegativePrompt : APP_CONFIG.defaultNegativePrompt);
  const normalizedImageBuffer = isArtisticQr
    ? await buildArtisticQrCanvas(trimmedInput?.buffer || imageBuffer, 1024, 0.08)
    : await normalizeInputImage(trimmedInput?.buffer || imageBuffer);
  const normalizedReferenceImages = await Promise.all(
    referenceImages
      .slice(0, APP_CONFIG.maxReferenceImages)
      .map(async (item) => normalizeInputImage(item.buffer))
  );

  let primaryImageBuffer = normalizedImageBuffer;
  let pipeline = {
    thoughts: [
      "步骤 1：读取上传的艺术二维码图。",
      "步骤 2：如果提供空白盘模板，服务端把原始二维码按页面参数放入盘中。",
      "步骤 3：服务端按页面选择生成方形或圆形 image2 edits mask：中心二维码区域不透明锁定，外侧透明允许扩图。",
      "步骤 4：提交 image + mask + 单句 prompt 到 /v1/images/edits。",
      "步骤 5：只调用一次 image2 edits，最多等待 3 分钟。"
    ],
    templateUsed: false,
    artisticQr: isArtisticQr,
    referenceImageCount: normalizedReferenceImages.length,
    ...(trimmedInput ? { qrTrim: trimmedInput.info } : {})
  };

  if (templateImage?.buffer) {
    const templateBuffer = await sharp(templateImage.buffer).png().toBuffer();
    const preprocessed = await buildSquareQrPlatePreview({
      templateBuffer,
      qrBuffer: imageBuffer,
      placement: maskPlacement,
      qrTrim
    });
    primaryImageBuffer = preprocessed.composedBuffer;
    pipeline = {
      ...pipeline,
      templateUsed: true,
      templateSize: preprocessed.templateSize,
      plateBox: preprocessed.plateBox,
      squareBox: preprocessed.squareBox,
      maskShape: preprocessed.squareBox.source.shape,
      qrTrim: preprocessed.qrTrim,
      squarePreviewDataUrl: `data:image/png;base64,${preprocessed.composedBuffer.toString("base64")}`,
      squarePreviewUploadDataUrl: `data:image/png;base64,${primaryImageBuffer.toString("base64")}`,
      debugOverlayDataUrl: `data:image/png;base64,${preprocessed.debugOverlayBuffer.toString("base64")}`,
      editMaskDataUrl: `data:image/png;base64,${preprocessed.editMaskBuffer.toString("base64")}`,
      editMaskBuffer: preprocessed.editMaskBuffer
    };
  }

  const contextImage = isArtisticQr
    ? {
        uploadBuffer: primaryImageBuffer,
        collagePreviewDataUrl: `data:image/png;base64,${primaryImageBuffer.toString("base64")}`
      }
    : templateImage?.buffer
    ? {
        uploadBuffer: primaryImageBuffer,
        collagePreviewDataUrl: `data:image/png;base64,${primaryImageBuffer.toString("base64")}`
      }
    : await buildSingleContextImage(primaryImageBuffer, normalizedReferenceImages);

  const editMaskBuffer = pipeline.editMaskBuffer
    || await buildTransparentMaskLike(contextImage.uploadBuffer);

  const submitUrl = buildImageEditUrl(apiBaseUrl);
  const basePipeline = {
    ...pipeline,
    editMaskBuffer: undefined,
    requestImageCount: 1,
    uploadContextMode: isArtisticQr
      ? "artistic-qr-canvas"
      : (templateImage?.buffer ? "single-mask-preview" : (normalizedReferenceImages.length > 0 ? "single-collage" : "single-primary")),
    collagePreviewDataUrl: contextImage.collagePreviewDataUrl,
    positivePrompt: effectivePositivePrompt,
    negativePrompt: effectiveNegativePrompt
  };

  const editResult = await submitImageEditOnce({
    fetchImpl,
    submitUrl,
    contextImageBuffer: contextImage.uploadBuffer,
    editMaskBuffer,
    positivePrompt: effectivePositivePrompt,
    negativePrompt: effectiveNegativePrompt,
    inputFidelity: isArtisticQr ? "high" : undefined,
    pipeline: basePipeline,
    userId,
    apiKey
  });

  const outputBuffer = isArtisticQr
    ? await rebuildArtisticQrFromModules(primaryImageBuffer, editResult.imageBuffer)
    : editResult.imageBuffer;

  return {
    imageDataUrl: `data:image/png;base64,${outputBuffer.toString("base64")}`,
    revisedPrompt: editResult.revisedPrompt,
    pipeline: {
      ...basePipeline,
      selectedSubmitAttempt: editResult.selectedAttempt,
      submitAttempts: editResult.attempts,
      editMode: isArtisticQr ? "artistic-qr-edits" : "image2-edits-mask"
    }
  };
}
