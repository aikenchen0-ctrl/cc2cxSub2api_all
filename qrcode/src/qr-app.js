import express from "express";
import multer from "multer";
import sharp from "sharp";
import { randomUUID } from "crypto";

import { generateQrArtwork } from "./qr-generator.js";
import { buildSquareQrPlatePreview } from "./mask-processor.js";
import { urlToQrImage, isValidUrl } from "./url-to-qr.js";
import { APP_CONFIG } from "./config.js";
import {
  createSessionCookie,
  requireQrcodeUser,
  verifyQrcodeSSOTicket
} from "./sso.js";

const DEFAULT_PRESET_MASK_PLACEMENT = {
  x: 575,
  y: 670,
  size: 950
};



function parseQrTrimOptions(body = {}) {
  const parse = (value) => {
    const number = Number(value);
    return Number.isFinite(number) ? number : undefined;
  };

  const enabled = body.qrTrimEnabled === "true";
  
  if (!enabled) {
    return { enabled: false };
  }
  
  // 检查是否是手动剪裁模式
  if (body.qrTrimMode === "manual") {
    return {
      enabled: true,
      mode: "manual",
      x: parse(body.qrCropX),
      y: parse(body.qrCropY),
      width: parse(body.qrCropWidth),
      height: parse(body.qrCropHeight)
    };
  }
  
  // 自动剪裁模式（保留原有逻辑，但默认不启用）
  return {
    enabled: false,
    threshold: parse(body.qrTrimThreshold),
    padding: parse(body.qrTrimPadding)
  };
}

function parseMaskPlacement(body = {}) {
  const parse = (value) => {
    const number = Number(value);
    return Number.isFinite(number) ? number : undefined;
  };
  const shape = body.maskShape === "circle" ? "circle" : "square";
  const isBlankPreset = body.templatePreset === "blank";

  return {
    x: parse(body.maskX) ?? (isBlankPreset ? DEFAULT_PRESET_MASK_PLACEMENT.x : undefined),
    y: parse(body.maskY) ?? (isBlankPreset ? DEFAULT_PRESET_MASK_PLACEMENT.y : undefined),
    size: parse(body.maskSize) ?? (isBlankPreset ? DEFAULT_PRESET_MASK_PLACEMENT.size : undefined),
    shape
  };
}

function withGenerateTimeout(promise) {
  let timer;
  const timeout = new Promise((_, reject) => {
    timer = setTimeout(() => {
      reject(new Error(`生成超时：单次生图最多等待 ${Math.round(APP_CONFIG.generateTimeoutMs / 1000)} 秒`));
    }, APP_CONFIG.generateTimeoutMs);
  });

  return Promise.race([promise, timeout]).finally(() => clearTimeout(timer));
}

const GENERATE_JOB_TTL_MS = 30 * 60 * 1000;

function shouldRunGenerateAsync(body = {}) {
  return body.async === "true" || body.async === "1";
}

function slimSubmitAttempt(attempt) {
  if (!attempt || typeof attempt !== "object") {
    return undefined;
  }

  return {
    httpStatus: attempt.httpStatus,
    ok: attempt.ok,
    error: attempt.error,
    endpoint: attempt.endpoint
  };
}

function slimGeneratePipeline(pipeline) {
  if (!pipeline || typeof pipeline !== "object") {
    return pipeline;
  }

  return {
    phase: "production-slimmed",
    debugPayloadOmitted: true,
    templateUsed: pipeline.templateUsed,
    referenceImageCount: pipeline.referenceImageCount,
    requestImageCount: pipeline.requestImageCount,
    uploadContextMode: pipeline.uploadContextMode,
    editMode: pipeline.editMode,
    maskShape: pipeline.maskShape,
    selectedSubmitAttempt: slimSubmitAttempt(pipeline.selectedSubmitAttempt),
    submitAttempts: Array.isArray(pipeline.submitAttempts)
      ? pipeline.submitAttempts.map(slimSubmitAttempt).filter(Boolean)
      : undefined,
    error: pipeline.error,
    message: pipeline.message
  };
}

function buildGenerateResultPayload(result, { includeDebugPipeline = true } = {}) {
  if (!result || includeDebugPipeline) {
    return result;
  }

  return {
    ...result,
    pipeline: slimGeneratePipeline(result.pipeline)
  };
}

function buildGenerateErrorPayload(error, { includeDebugPipeline = true } = {}) {
  const pipeline = error?.pipeline || {
    phase: "server-error-before-pipeline",
    message: "服务端在生成完整 pipeline 前失败。请看 error 字段和服务端日志。",
    error: error instanceof Error ? error.message : String(error)
  };

  return {
    error: error instanceof Error ? error.message : "生成失败",
    pipeline: includeDebugPipeline ? pipeline : slimGeneratePipeline(pipeline)
  };
}

function upstreamStatusFromGenerationError(error) {
  const attempts = error?.pipeline?.submitAttempts;
  const status = Number(
    error?.pipeline?.selectedSubmitAttempt?.httpStatus
      || (Array.isArray(attempts) ? attempts[attempts.length - 1]?.httpStatus : 0)
  );
  return Number.isInteger(status) && status >= 400 && status <= 599 ? status : 0;
}

function cleanupGenerateJobs(generateJobs, now = Date.now()) {
  for (const [jobId, job] of generateJobs.entries()) {
    const age = now - Number(job.createdAt || 0);
    if (age > GENERATE_JOB_TTL_MS) {
      generateJobs.delete(jobId);
    }
  }
}

function toPublicGenerateJob(job) {
  const payload = {
    jobId: job.id,
    status: job.status,
    createdAt: job.createdAt,
    updatedAt: job.updatedAt
  };

  if (job.status === "succeeded") {
    payload.result = job.result;
  }

  if (job.status === "failed") {
    payload.error = job.error;
    payload.pipeline = job.pipeline;
  }

  return payload;
}

function normalizeGenerateConcurrency(value) {
  const number = Number(value);
  return Number.isInteger(number) && number > 0 ? number : 1;
}

function resolveSessionAiConfig(userId) {
  const baseUrl = String(APP_CONFIG.apiBaseUrl || "").trim().replace(/\/+$/, "");
  return { userId, apiBaseUrl: baseUrl.endsWith("/v1") ? baseUrl : `${baseUrl}/v1` };
}

function createGenerateJobScheduler(generateJobs, generateConcurrency, { includeDebugPipeline = true } = {}) {
  const queue = [];
  const concurrency = normalizeGenerateConcurrency(generateConcurrency);
  let activeCount = 0;

  function runNextGenerateJob() {
    while (activeCount < concurrency && queue.length > 0) {
      const { job, runGenerate } = queue.shift();
      if (!generateJobs.has(job.id)) {
        continue;
      }

      activeCount += 1;
      Promise.resolve()
        .then(async () => {
          job.status = "running";
          job.updatedAt = Date.now();
          job.result = buildGenerateResultPayload(await runGenerate(), { includeDebugPipeline });
          job.status = "succeeded";
          job.updatedAt = Date.now();
        })
        .catch((error) => {
          const payload = buildGenerateErrorPayload(error, { includeDebugPipeline });
          job.status = "failed";
          job.error = payload.error;
          job.pipeline = payload.pipeline;
          job.updatedAt = Date.now();
        })
        .finally(() => {
          activeCount -= 1;
          runNextGenerateJob();
        });
    }
  }

  return function createGenerateJob(runGenerate, ownerUserId = "") {
    cleanupGenerateJobs(generateJobs);

    const now = Date.now();
    const job = {
      id: `gen_${randomUUID()}`,
      ownerUserId: String(ownerUserId || ""),
      status: "queued",
      createdAt: now,
      updatedAt: now,
      result: null,
      error: null,
      pipeline: null
    };

    generateJobs.set(job.id, job);
    queue.push({ job, runGenerate });
    runNextGenerateJob();

    return job;
  };
}

const WECHAT_IMAGE_MAX_BYTES = 10 * 1024 * 1024;
const WECHAT_PREPARE_MAX_BYTES = 30 * 1024 * 1024;

function buildWechatImageRecommendations() {
  return [
    "先使用 PNG 压缩，不改变二维码结构。",
    "如果压缩后仍超限，再按比例降低整张图尺寸，保持二维码整体几何关系。",
    "如果仍无法达标，建议重新生成低细节或更小尺寸图片。"
  ];
}

async function prepareWechatMaterialImage(buffer, mimeType = "image/png") {
  if (buffer.length <= WECHAT_IMAGE_MAX_BYTES) {
    return {
      buffer,
      mimeType,
      action: "none",
      withinLimit: true,
      recommendations: []
    };
  }

  let metadata;
  try {
    metadata = await sharp(buffer).metadata();
  } catch {
    throw new Error("图片格式无法处理，请使用 bmp/png/jpeg/jpg/gif 格式。");
  }

  let candidate = await sharp(buffer)
    .rotate()
    .png({
      compressionLevel: 9,
      adaptiveFiltering: true,
      effort: 10,
      palette: true,
      quality: 100
    })
    .toBuffer();

  if (candidate.length <= WECHAT_IMAGE_MAX_BYTES) {
    return {
      buffer: candidate,
      mimeType: "image/png",
      action: "png-compress",
      withinLimit: true,
      recommendations: buildWechatImageRecommendations()
    };
  }

  let width = metadata.width || 0;
  let height = metadata.height || 0;
  while (candidate.length > WECHAT_IMAGE_MAX_BYTES && width > 1024 && height > 1024) {
    width = Math.max(1024, Math.floor(width * 0.9));
    height = Math.max(1024, Math.floor(height * 0.9));
    candidate = await sharp(buffer)
      .rotate()
      .resize(width, height, { fit: "inside", withoutEnlargement: true })
      .png({
        compressionLevel: 9,
        adaptiveFiltering: true,
        effort: 10,
        palette: true,
        quality: 100
      })
      .toBuffer();
  }

  return {
    buffer: candidate,
    mimeType: "image/png",
    action: "png-compress-and-resize",
    withinLimit: candidate.length <= WECHAT_IMAGE_MAX_BYTES,
    recommendations: buildWechatImageRecommendations()
  };
}

const upload = multer({
  storage: multer.memoryStorage(),
  limits: {
    fileSize: 8 * 1024 * 1024
  }
});
const wechatMaterialUpload = multer({
  storage: multer.memoryStorage(),
  limits: {
    fileSize: 10 * 1024 * 1024,
    fieldSize: 64 * 1024
  }
});
const wechatMaterialPrepareUpload = multer({
  storage: multer.memoryStorage(),
  limits: {
    fileSize: WECHAT_PREPARE_MAX_BYTES,
    fieldSize: 64 * 1024
  }
});
const textForm = multer().none();

function resolveAllowedCorsOrigin(configuredOrigin, requestOrigin) {
  const origins = String(configuredOrigin || "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);

  if (!origins.length) {
    return "";
  }

  if (origins.includes("*")) {
    return "*";
  }

  return requestOrigin && origins.includes(requestOrigin) ? requestOrigin : "";
}

function applyCors(app, configuredOrigin) {
  app.use((request, response, next) => {
    const allowedOrigin = resolveAllowedCorsOrigin(configuredOrigin, request.get("Origin"));

    if (allowedOrigin) {
      response.setHeader("Access-Control-Allow-Origin", allowedOrigin);
      response.setHeader("Access-Control-Allow-Methods", "GET,POST,OPTIONS");
      response.setHeader("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Requested-With,X-Sub2API-API-Key,X-Sub2API-Base-URL");
      response.setHeader("Access-Control-Max-Age", "86400");
      if (allowedOrigin !== "*") {
        response.setHeader("Vary", "Origin");
      }
    }

    if (request.method === "OPTIONS" && request.path.startsWith("/api/") && allowedOrigin) {
      response.status(204).end();
      return;
    }

    next();
  });
}

class WechatApiError extends Error {
  constructor(payload) {
    super(payload.error);
    this.name = "WechatApiError";
    this.payload = payload;
  }
}

const WECHAT_DEPENDENCY_FAILURE_STATUS = 424;

function extractWechatIp(message) {
  return String(message || "").match(/\b(?:\d{1,3}\.){3}\d{1,3}\b/)?.[0] || "";
}

function buildWechatFailurePayload(fallbackMessage, {
  status = WECHAT_DEPENDENCY_FAILURE_STATUS,
  data = null,
  text = "",
  error = null
} = {}) {
  const statusCode = status >= 400 && status < 500 ? status : WECHAT_DEPENDENCY_FAILURE_STATUS;
  const errcode = data?.errcode;
  const errmsg = data?.errmsg || error?.message || text || "未知错误";
  const rawMessage = typeof data === "object" && data
    ? JSON.stringify(data)
    : String(errmsg || "");
  const ip = extractWechatIp(`${errmsg} ${rawMessage}`);
  const normalizedMessage = `${errmsg} ${rawMessage}`.toLowerCase();

  if (errcode === 40164 || normalizedMessage.includes("invalid ip") || normalizedMessage.includes("not in whitelist")) {
    return {
      statusCode,
      error: `${fallbackMessage}: 微信拒绝当前部署服务器 IP，未加入公众号 IP 白名单。`,
      errcode,
      errmsg,
      diagnostic: ip
        ? `微信返回的服务器出口 IP 是 ${ip}，需要在微信公众号后台把这个 IP 加入白名单。`
        : "微信返回当前服务器出口 IP 不在公众号 IP 白名单中，需要在微信公众号后台补充部署服务器出口 IP。",
      actions: [
        "确认线上访问的是 Node 服务，不是只部署了 public 静态目录。",
        "在部署服务器上查看服务器出口 IP，并加入微信公众号后台“基本配置”的 IP 白名单。",
        "如果使用云平台、容器、Serverless 或代理，白名单要填写真实出站 IP，不是浏览器用户 IP。",
        "修改白名单后重新点击获取 access_token。"
      ],
      wechat: data || null
    };
  }

  if (errcode === 40013) {
    return {
      statusCode,
      error: `${fallbackMessage}: AppID 无效，请检查线上页面填写的 appid。`,
      errcode,
      errmsg,
      diagnostic: "微信返回 appid 无效，通常是 appid 填错或账号类型不匹配。",
      actions: ["复制公众号后台开发者 ID，不要复制小程序或其他账号的 AppID。"],
      wechat: data || null
    };
  }

  if (errcode === 40125 || errcode === 40001) {
    return {
      statusCode,
      error: `${fallbackMessage}: AppSecret 或 access_token 无效。`,
      errcode,
      errmsg,
      diagnostic: "微信返回凭据错误，通常是 secret 填错、重置后未同步，或 token 已失效。",
      actions: ["重新复制公众号后台 AppSecret。", "如果刚重置 secret，请刷新页面后重新获取 access_token。"],
      wechat: data || null
    };
  }

  return {
    statusCode,
    error: `${fallbackMessage}: HTTP ${status} ${rawMessage.slice(0, 300)}`.trim(),
    errcode,
    errmsg,
    diagnostic: "微信接口返回错误，详情见 wechat 字段。",
    actions: ["检查 appid、secret、access_token、公众号权限和微信后台 IP 白名单。"],
    wechat: data || null
  };
}

function buildWechatNetworkPayload(fallbackMessage, error) {
  const message = error?.message || "未知网络错误";
  return {
    statusCode: WECHAT_DEPENDENCY_FAILURE_STATUS,
    error: `${fallbackMessage}: 服务器无法连接微信接口。`,
    diagnostic: `部署服务器访问 api.weixin.qq.com 失败：${message}。请检查服务器出站网络、DNS、防火墙、代理和 HTTPS 证书环境。`,
    actions: [
      "确认线上部署运行的是 Node 服务，并且服务端可以访问 https://api.weixin.qq.com。",
      "检查云服务器安全组、防火墙、代理、DNS 和容器出站网络配置。",
      "如果前端和后端分开部署，请配置正确的 API 地址，不要只部署静态页面。"
    ]
  };
}

function sendWechatError(response, error, fallbackMessage) {
  const payload = error instanceof WechatApiError
    ? error.payload
    : buildWechatNetworkPayload(fallbackMessage, error);
  const { statusCode = WECHAT_DEPENDENCY_FAILURE_STATUS, ...body } = payload;
  response.status(statusCode).json(body);
}

async function readWechatJson(response, fallbackMessage) {
  const text = await response.text();
  let data;

  try {
    data = JSON.parse(text);
  } catch {
    throw new WechatApiError({
      statusCode: WECHAT_DEPENDENCY_FAILURE_STATUS,
      error: `${fallbackMessage}: 微信返回了非 JSON 内容。`,
      diagnostic: text.slice(0, 300),
      actions: ["确认请求的微信接口地址正确，并检查微信服务端返回内容。"]
    });
  }

  if (!response.ok || data?.errcode) {
    throw new WechatApiError(buildWechatFailurePayload(fallbackMessage, {
      status: response.status || WECHAT_DEPENDENCY_FAILURE_STATUS,
      data,
      text
    }));
  }

  return data;
}

export function createQrApp({
  generate = generateQrArtwork,
  // Keep an injection point for tests and local extensions. The production
  // path intentionally uses the same Sub2API image relay as /api/generate;
  // it must not call the retired third-party art-QR API.
  stylize = null,
  wechatFetch = fetch,
  corsOrigin = APP_CONFIG.corsOrigin,
  generateConcurrency = APP_CONFIG.generateConcurrency,
  includeDebugPipeline = APP_CONFIG.includeDebugPipeline
} = {}) {
  const app = express();
  const generateJobs = new Map();
  const createGenerateJob = createGenerateJobScheduler(generateJobs, generateConcurrency, { includeDebugPipeline });

  applyCors(app, corsOrigin);
  app.use(express.static("public"));
  app.use(express.json()); // 添加JSON解析中间件

  app.get("/api/health", (_request, response) => {
    response.json({ ok: true });
  });

  app.get("/api/auth/sso/callback", (request, response) => {
    try {
      const verified = verifyQrcodeSSOTicket(String(request.query.ticket || ""));
      response.setHeader("Cache-Control", "no-store");
      response.setHeader("Set-Cookie", createSessionCookie(verified.userId));
      response.redirect(verified.next || "/");
    } catch (error) {
      // Keep the browser response generic, but retain the actual reason in
      // server logs so missing/mismatched production secrets are diagnosable.
      console.error("QRCode SSO callback failed:", error instanceof Error ? error.message : error);
      response.status(401).send("SSO login failed");
    }
  });

  app.get("/api/generate/jobs/:jobId", (request, response) => {
    const userId = requireQrcodeUser(request, response);
    if (!userId) return;
    cleanupGenerateJobs(generateJobs);

    const job = generateJobs.get(String(request.params.jobId || ""));
    if (!job || (job.ownerUserId && job.ownerUserId !== userId)) {
      response.status(404).json({ error: "生成任务不存在或已过期。" });
      return;
    }

    response.json(toPublicGenerateJob(job));
  });

  app.post("/api/wechat/token", async (request, response) => {
    try {
      const appid = String(request.body?.appid || "").trim();
      const secret = String(request.body?.secret || "").trim();

      if (!appid || !secret) {
        response.status(400).json({ error: "请填写 appid 和 secret。" });
        return;
      }

      const params = new URLSearchParams({
        appid,
        secret,
        grant_type: "client_credential"
      });
      const tokenResponse = await wechatFetch(`https://api.weixin.qq.com/cgi-bin/token?${params.toString()}`);
      response.json(await readWechatJson(tokenResponse, "获取 access_token 失败"));
    } catch (error) {
      sendWechatError(response, error, "获取 access_token 失败");
    }
  });

  app.post("/api/wechat/material/upload", (request, response, next) => {
    wechatMaterialUpload.single("media")(request, response, (error) => {
      if (error) {
        if (error instanceof multer.MulterError && error.code === "LIMIT_FILE_SIZE") {
          response.status(400).json({
            error: "上传素材文件失败：图片超过微信永久图片素材 10MB 限制，请先点击“压缩为微信素材大小”。"
          });
          return;
        }
        response.status(400).json({
          error: error instanceof multer.MulterError
            ? `上传素材文件失败：${error.message}`
            : "上传素材文件失败。"
        });
        return;
      }
      next();
    });
  }, async (request, response) => {
    try {
      const accessToken = String(request.body?.accessToken || "").trim();
      const filename = String(request.body?.filename || "").trim();
      const mediaFile = request.file;

      if (!accessToken) {
        response.status(400).json({ error: "请先获取 access_token。" });
        return;
      }
      if (!filename) {
        response.status(400).json({ error: "请填写生成图片结果的名字。" });
        return;
      }
      if (!mediaFile) {
        response.status(400).json({ error: "没有收到 media 图片文件，请重新生成后再上传。" });
        return;
      }

      const formData = new FormData();
      formData.append("access_token", accessToken);
      formData.append("type", "image");
      formData.append(
        "media",
        new Blob([mediaFile.buffer], { type: mediaFile.mimetype || "image/png" }),
        filename
      );

      const uploadResponse = await wechatFetch(
        "https://api.weixin.qq.com/cgi-bin/material/add_material",
        {
          method: "POST",
          body: formData
        }
      );
      response.json(await readWechatJson(uploadResponse, "上传素材失败"));
    } catch (error) {
      sendWechatError(response, error, "上传素材失败");
    }
  });

  app.post("/api/wechat/material/prepare", (request, response, next) => {
    wechatMaterialPrepareUpload.single("media")(request, response, (error) => {
      if (error) {
        response.status(400).json({
          error: error instanceof multer.MulterError
            ? `准备素材文件失败：${error.message}`
            : "准备素材文件失败。"
        });
        return;
      }
      next();
    });
  }, async (request, response) => {
    try {
      const mediaFile = request.file;
      if (!mediaFile) {
        response.status(400).json({ error: "生成完成后才可以处理图片。" });
        return;
      }

      const prepared = await prepareWechatMaterialImage(mediaFile.buffer, mediaFile.mimetype || "image/png");
      if (!prepared.withinLimit) {
        response.status(422).json({
          error: "图片压缩后仍超过微信永久图片素材 10MB 限制。",
          originalBytes: mediaFile.size,
          preparedBytes: prepared.buffer.length,
          action: prepared.action,
          recommendations: prepared.recommendations
        });
        return;
      }

      response.json({
        mimeType: prepared.mimeType,
        originalBytes: mediaFile.size,
        preparedBytes: prepared.buffer.length,
        withinLimit: true,
        action: prepared.action,
        recommendations: prepared.recommendations,
        imageDataUrl: `data:${prepared.mimeType};base64,${prepared.buffer.toString("base64")}`
      });
    } catch (error) {
      response.status(500).json({
        error: error instanceof Error ? error.message : "准备素材图片失败"
      });
    }
  });

  app.post("/api/wechat/material/list", async (request, response) => {
    try {
      const accessToken = String(request.body?.accessToken || "").trim();
      const offset = Number.isFinite(Number(request.body?.offset)) ? Number(request.body.offset) : 0;
      const count = Number.isFinite(Number(request.body?.count)) ? Number(request.body.count) : 1000;

      if (!accessToken) {
        response.status(400).json({ error: "请先获取 access_token。" });
        return;
      }

      const listResponse = await wechatFetch(
        `https://api.weixin.qq.com/cgi-bin/material/batchget_material?access_token=${encodeURIComponent(accessToken)}`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            type: "image",
            offset,
            count
          })
        }
      );
      response.json(await readWechatJson(listResponse, "获取素材列表失败"));
    } catch (error) {
      sendWechatError(response, error, "获取素材列表失败");
    }
  });

  app.post("/api/wechat/material/get", async (request, response) => {
    try {
      const accessToken = String(request.body?.accessToken || "").trim();
      const mediaId = String(request.body?.mediaId || "").trim();

      if (!accessToken) {
        response.status(400).json({ error: "请先获取 access_token。" });
        return;
      }
      if (!mediaId) {
        response.status(400).json({ error: "请先输入名称查询素材。" });
        return;
      }

      const materialResponse = await wechatFetch(
        `https://api.weixin.qq.com/cgi-bin/material/get_material?access_token=${encodeURIComponent(accessToken)}`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ media_id: mediaId })
        }
      );
      const contentType = materialResponse.headers?.get?.("content-type") || "";

      if (contentType.includes("application/json")) {
        response.json(await readWechatJson(materialResponse, "获取素材图片失败"));
        return;
      }

      if (!materialResponse.ok) {
        response.status(500).json({ error: `获取素材图片失败: HTTP ${materialResponse.status}` });
        return;
      }

      const buffer = Buffer.from(await materialResponse.arrayBuffer());
      const mimeType = contentType || "image/png";
      response.json({
        mimeType,
        imageDataUrl: `data:${mimeType};base64,${buffer.toString("base64")}`
      });
    } catch (error) {
      sendWechatError(response, error, "获取素材图片失败");
    }
  });

  app.post("/api/preview-mask", upload.fields([
    { name: "qrImage", maxCount: 1 },
    { name: "templateImage", maxCount: 1 },
    { name: "referenceImages", maxCount: 3 }
  ]), async (request, response) => {
    try {
      const qrFile = request.files?.qrImage?.[0];
      const templateFile = request.files?.templateImage?.[0] || null;
      if (!qrFile || !templateFile) {
        response.status(400).json({ error: "遮罩预览需要同时上传二维码和空白盘模板。" });
        return;
      }

      const preprocessed = await buildSquareQrPlatePreview({
        templateBuffer: templateFile.buffer,
        qrBuffer: qrFile.buffer,
        placement: parseMaskPlacement(request.body),
        qrTrim: parseQrTrimOptions(request.body)
      });
      const submittedContextDataUrl = `data:image/png;base64,${preprocessed.debugOverlayBuffer.toString("base64")}`;

      response.json({
        debugOverlayDataUrl: submittedContextDataUrl,
        collagePreviewDataUrl: submittedContextDataUrl,
        squareBox: preprocessed.squareBox,
        templateSize: preprocessed.templateSize,
        templatePreprocess: preprocessed.templatePreprocess,
        qrTrim: preprocessed.qrTrim
      });
    } catch (error) {
      response.status(500).json({
        error: error instanceof Error ? error.message : "遮罩预览失败"
      });
    }
  });

  app.post("/api/stylize-qr", upload.fields([
    { name: "qrImage", maxCount: 1 }
  ]), async (request, response) => {
    const userId = requireQrcodeUser(request, response);
    if (!userId) return;

    try {
      const qrFile = request.files?.qrImage?.[0];
      if (!qrFile) {
        response.status(400).json({ error: "请先上传普通二维码图片。" });
        return;
      }

      const qrTrim = parseQrTrimOptions(request.body);
      const result = stylize
        ? await stylize({
            imageBuffer: qrFile.buffer,
            mimeType: qrFile.mimetype || "image/png",
            qrTrim,
            userId,
            ...resolveSessionAiConfig(userId)
          })
        : await withGenerateTimeout(generate({
            imageBuffer: qrFile.buffer,
            mimeType: qrFile.mimetype || "image/png",
            qrTrim,
            artisticQr: true,
            positivePrompt: APP_CONFIG.artQr.sub2apiPrompt,
            negativePrompt: APP_CONFIG.artQr.sub2apiNegativePrompt,
            // The artistic-QR button is the no-template variant of the
            // public qrcode capability. The generator submits gpt-image-2
            // to Sub2API /v1/images/edits with the session user identity.
            userId,
            ...resolveSessionAiConfig(userId)
          }));

      const imageDataUrl = result?.imageDataUrl || (
        result?.imageBuffer
          ? `data:${result.mimeType || "image/png"};base64,${result.imageBuffer.toString("base64")}`
          : ""
      );
      if (!imageDataUrl) {
        throw new Error("艺术化二维码接口服务未返回图片");
      }

      response.json({
        imageDataUrl,
        mimeType: result.mimeType || "image/png",
        filename: `artistic-${Date.now()}.png`,
        task: result.task,
        input: result.input,
        revisedPrompt: result.revisedPrompt
      });
    } catch (error) {
      response.status(upstreamStatusFromGenerationError(error) || 500).json({
        error: error instanceof Error ? error.message : "艺术化二维码失败"
      });
    }
  });

  // 解析 multipart/form-data 中的文本字段（无文件上传）
  const urlUpload = multer().none();
  
  app.post("/api/url-to-qr", urlUpload, async (request, response) => {
    try {
      const url = request.body?.url;
      
      if (!url) {
        response.status(400).json({ error: "请提供URL地址" });
        return;
      }
      
      if (!isValidUrl(url)) {
        response.status(400).json({ error: "无效的URL格式，请确保URL以http://或https://开头" });
        return;
      }
      
      const result = await urlToQrImage(url);
      
      response.json({
        imageDataUrl: `data:${result.mimeType};base64,${result.buffer.toString("base64")}`,
        mimeType: result.mimeType,
        filename: result.filename
      });
    } catch (error) {
      response.status(500).json({
        error: error instanceof Error ? error.message : "URL转二维码失败"
      });
    }
  });

  app.post("/api/generate", upload.fields([
    { name: "qrImage", maxCount: 1 },
    { name: "templateImage", maxCount: 1 },
    { name: "referenceImages", maxCount: 3 }
  ]), async (request, response) => {
    const userId = requireQrcodeUser(request, response);
    if (!userId) return;
    try {
      const qrFile = request.files?.qrImage?.[0];
      const templateFile = request.files?.templateImage?.[0] || null;
      const referenceFiles = request.files?.referenceImages || [];
      const basePipeline = {
        phase: "server-received-request",
        files: {
          qrImage: qrFile
            ? { originalname: qrFile.originalname, mimetype: qrFile.mimetype, size: qrFile.size }
            : null,
          templateImage: templateFile
            ? { originalname: templateFile.originalname, mimetype: templateFile.mimetype, size: templateFile.size }
            : null,
          referenceImages: referenceFiles.map((file) => ({
            originalname: file.originalname,
            mimetype: file.mimetype,
            size: file.size
          }))
        },
        maskPlacement: parseMaskPlacement(request.body),
        qrTrim: parseQrTrimOptions(request.body),
        positivePrompt: request.body?.positivePrompt || "",
        negativePrompt: request.body?.negativePrompt || ""
      };

      if (!qrFile) {
        response.status(400).json({ error: "请先上传二维码图片。", pipeline: basePipeline });
        return;
      }

      const runGenerate = () => withGenerateTimeout(generate({
          imageBuffer: qrFile.buffer,
          mimeType: qrFile.mimetype || "image/png",
          templateImage: templateFile
            ? {
                buffer: templateFile.buffer,
                mimeType: templateFile.mimetype || "image/png"
              }
            : null,
          maskPlacement: parseMaskPlacement(request.body),
          qrTrim: parseQrTrimOptions(request.body),
          referenceImages: referenceFiles.map((file) => ({
            buffer: file.buffer,
            mimeType: file.mimetype || "image/png"
          })),
          positivePrompt: request.body?.positivePrompt,
          negativePrompt: request.body?.negativePrompt,
          ...resolveSessionAiConfig(userId)
        }));

      if (shouldRunGenerateAsync(request.body)) {
        const job = createGenerateJob(runGenerate, userId);
        response.status(202).json(toPublicGenerateJob(job));
        return;
      }

      const result = await runGenerate();

      response.json(buildGenerateResultPayload(result, { includeDebugPipeline }));
    } catch (error) {
      response.status(500).json(buildGenerateErrorPayload(error, { includeDebugPipeline }));
    }
  });

  return app;
}
