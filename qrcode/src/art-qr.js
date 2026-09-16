import sharp from "sharp";
import crypto from "node:crypto";

import { APP_CONFIG } from "./config.js";
import { trimQrWhiteBorder } from "./mask-processor.js";

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function buildArtQrUrl(pathname) {
  return `${APP_CONFIG.artQr.apiBaseUrl}${pathname}`;
}

function isTransientSubmitTimeout(text) {
  const normalized = String(text || "").toLowerCase();
  return normalized.includes("timed out") || normalized.includes("timeout");
}

async function buildQrImageBase64(imageBuffer) {
  const qualities = [95, 88, 80, 72];

  for (const quality of qualities) {
    const jpegBuffer = await sharp(imageBuffer)
      .rotate()
      .jpeg({ quality, mozjpeg: true })
      .toBuffer();

    if (jpegBuffer.length <= APP_CONFIG.artQr.maxImageBytes) {
      return {
        jpegBuffer,
        base64: jpegBuffer.toString("base64"),
        quality
      };
    }
  }

  throw new Error(`艺术化二维码失败：处理后的 jpg 超过 ${Math.round(APP_CONFIG.artQr.maxImageBytes / 1024 / 1024)}MB 限制`);
}

async function submitArtQrTask({
  qrImageBase64,
  fetchImpl,
  prompt,
  negativePrompt,
  model,
  callbackUrl,
  submitMaxAttempts = APP_CONFIG.artQr.submitMaxAttempts,
  submitRetryDelayMs = APP_CONFIG.artQr.submitRetryDelayMs
}) {
  const submitUrl = buildArtQrUrl(APP_CONFIG.artQr.generateEndpoint);
  const body = {
    prompt,
    negative_prompt: negativePrompt,
    model,
    callback_url: callbackUrl,
    qr_image: qrImageBase64
  };

  for (let attempt = 1; attempt <= submitMaxAttempts; attempt += 1) {
    const response = await fetchImpl(submitUrl, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${APP_CONFIG.artQr.apiKey}`,
        "Content-Type": "application/json"
      },
      body: JSON.stringify(body)
    });

    const text = await response.text();
    let data;
    try {
      data = JSON.parse(text);
    } catch {
      if (isTransientSubmitTimeout(text) && attempt < submitMaxAttempts) {
        await sleep(submitRetryDelayMs);
        continue;
      }

      const retryNote = attempt > 1 ? `，已重试 ${attempt - 1} 次` : "";
      throw new Error(`艺术化二维码失败：生成接口临时超时${retryNote}，未返回任务编号。原始响应：${text.slice(0, 300)}`);
    }

    if (!response.ok || data?.code !== 0 || !data?.data?.img_uuid) {
      throw new Error(`艺术化二维码失败：生成接口异常 ${response.status} ${text.slice(0, 300)}`);
    }

    return {
      imgUuid: data.data.img_uuid,
      raw: data
    };
  }

  throw new Error("艺术化二维码失败：生成接口提交失败，未返回任务编号");
}

async function pollArtQrResult({ imgUuid, fetchImpl }) {
  const detailUrl = buildArtQrUrl(`${APP_CONFIG.artQr.detailEndpoint}?img_uuid=${encodeURIComponent(imgUuid)}`);

  for (let attempt = 1; attempt <= APP_CONFIG.artQr.pollMaxAttempts; attempt += 1) {
    const response = await fetchImpl(detailUrl, {
      headers: {
        Authorization: `Bearer ${APP_CONFIG.artQr.apiKey}`
      }
    });

    const text = await response.text();
    let data;
    try {
      data = JSON.parse(text);
    } catch {
      throw new Error(`艺术化二维码失败：详情接口返回非 JSON：${text.slice(0, 300)}`);
    }

    if (!response.ok || data?.code !== 0) {
      throw new Error(`艺术化二维码失败：详情接口异常 ${response.status} ${text.slice(0, 300)}`);
    }

    if (data?.data?.status === 1 && data?.data?.urls?.[0]) {
      return data.data;
    }

    if (data?.data?.status === -1) {
      throw new Error(`艺术化二维码失败：任务执行失败 ${text.slice(0, 300)}`);
    }

    await sleep(APP_CONFIG.artQr.pollIntervalMs);
  }

  throw new Error("艺术化二维码失败：任务轮询超时");
}

async function downloadArtQrImage({ imageUrl, fetchImpl }) {
  const response = await fetchImpl(imageUrl);
  if (!response.ok) {
    throw new Error(`艺术化二维码失败：结果图下载失败 ${response.status}`);
  }

  const contentType = response.headers.get("content-type") || "image/png";
  return {
    buffer: Buffer.from(await response.arrayBuffer()),
    mimeType: contentType
  };
}

export async function stylizeQrImage({
  imageBuffer,
  qrTrim = {},
  prompt = APP_CONFIG.artQr.defaultPrompt,
  negativePrompt = APP_CONFIG.artQr.negativePrompt,
  model = APP_CONFIG.artQr.model,
  callbackUrl = APP_CONFIG.artQr.callbackUrl,
  submitMaxAttempts = APP_CONFIG.artQr.submitMaxAttempts,
  submitRetryDelayMs = APP_CONFIG.artQr.submitRetryDelayMs,
  fetchImpl = fetch
}) {
  const trimmed = await trimQrWhiteBorder(imageBuffer, qrTrim);
  const prepared = await buildQrImageBase64(trimmed.buffer);
  const qrImageSha256 = crypto
    .createHash("sha256")
    .update(prepared.jpegBuffer)
    .digest("hex");
  const submitResult = await submitArtQrTask({
    qrImageBase64: prepared.base64,
    fetchImpl,
    prompt,
    negativePrompt,
    model,
    callbackUrl,
    submitMaxAttempts,
    submitRetryDelayMs
  });
  const detail = await pollArtQrResult({
    imgUuid: submitResult.imgUuid,
    fetchImpl
  });
  const image = await downloadArtQrImage({
    imageUrl: detail.urls[0],
    fetchImpl
  });
  const trimmedOutput = await trimQrWhiteBorder(image.buffer, {
    enabled: true,
    threshold: 245,
    padding: 0
  });

  return {
    imageBuffer: trimmedOutput.buffer,
    mimeType: "image/png",
    task: {
      imgUuid: submitResult.imgUuid,
      status: detail.status,
      duration: detail.duration,
      cost: detail.cost,
      url: detail.urls[0]
    },
    input: {
      qrTrim: trimmed.info,
      outputTrim: trimmedOutput.info,
      jpegQuality: prepared.quality,
      jpegBytes: prepared.jpegBuffer.length,
      model,
      prompt,
      negativePrompt,
      qrImageBytes: prepared.jpegBuffer.length,
      qrImageBase64Length: prepared.base64.length,
      qrImageSha256,
      callbackUrl
    }
  };
}
