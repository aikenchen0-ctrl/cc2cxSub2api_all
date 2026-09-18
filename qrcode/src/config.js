function readPositiveInteger(value, fallback) {
  const number = Number(value);
  return Number.isInteger(number) && number > 0 ? number : fallback;
}

function readText(value, fallback = "") {
  const text = String(value || "").trim();
  return text || fallback;
}

function readBoolean(value, fallback) {
  if (value === undefined || value === null || value === "") {
    return fallback;
  }

  return ["1", "true", "yes", "on"].includes(String(value).trim().toLowerCase());
}

export const APP_CONFIG = {
  port: readPositiveInteger(process.env.PORT, 5221),
  corsOrigin: readText(process.env.QR_CORS_ORIGIN),
  includeDebugPipeline: readBoolean(process.env.QR_INCLUDE_DEBUG_PIPELINE, process.env.NODE_ENV !== "production"),
  maxImages: 6,
  apiBaseUrl: readText(process.env.QR_API_BASE_URL || process.env.LINK, "http://localhost:18080"),
  apiKey: readText(process.env.QR_API_KEY),
  model: "image2",
  imageEditEndpoint: "/v1/images/edits",
  taskEndpointTemplate: "/v1/tasks/{taskId}",
  taskPollIntervalMs: 2000,
  taskPollMaxAttempts: 90,
  generateTimeoutMs: readPositiveInteger(process.env.QR_GENERATE_TIMEOUT_MS, 180000),
  generateConcurrency: readPositiveInteger(process.env.QR_GENERATE_CONCURRENCY, 1),
  maxInputImageSize: 128,
  maxReferenceImages: 3,
  qrOutputSize: "1024x1024",
  artQr: {
    apiBaseUrl: "https://open-qr.mewx.art",
    apiKey: "mx-JL9kdjnm6fM64ScdrQlEmqe5KtuVSBXy5k2SQHGiFluxxDIb",
    generateEndpoint: "/api/v1/images/generate",
    detailEndpoint: "/api/v1/images/detail",
    model: "67",
    defaultPrompt: "Pure white background.blue-and-white chinese porcelain style, elegant cobalt blue blue delicate peony and  blue camellia blossoms，Scattered Deep blue ink petals ,Ink painting floral motifs, Scattered Deep blue ink petals,blue delicate peony and  blue camellia blossoms, blue botanical leaves, refined ceramic ornament, Scattered Deep blue ink petals, glossy   glazed surface, clean soft background, centered composition, realistic porcelain   texture, crisp focus, clean edges, balanced floral decoration integrated into the central square pattern, minimal,High contrast pure white seamless background,\n--v 3 --iw 0.95 --shape tiny-plus --ar 1:1",
    negativePrompt: "text, letters, words, logo, watermark, signature, extra frames, border ornaments covering the code, busy background,\n  clutter, people, hands, food, table setting, perspective view, tilted plate, broken geometry, warped circle, low\n  contrast mush, blurry details, muddy pattern, heavy shadows, overexposed highlights, checkerboard artifacts, dense\n  geometric tessellation",
    callbackUrl: "https://shorts-locally-larry-refers.trycloudflare.com/api/v1/callback",
    maxImageBytes: 5 * 1024 * 1024,
    submitMaxAttempts: readPositiveInteger(process.env.QR_ART_SUBMIT_MAX_ATTEMPTS, 3),
    submitRetryDelayMs: readPositiveInteger(process.env.QR_ART_SUBMIT_RETRY_DELAY_MS, 1500),
    pollIntervalMs: 3000,
    pollMaxAttempts: 60
  },
  defaultPositivePrompt:
    "延展盘子空白处周围荷花纹理，严格锁定中心方形内全部像素与四角定位图形，不移动、不重绘、不改对比度；仅延展盘子空白处周围荷花纹理、色调与光影，让非定位区域边缘自然融合、无缝衔接不要有白边。把正方形的效果明显的边边角角也要融合到延展中融为一体。",
  defaultNegativePrompt:
    "不要改变中心方形二维码结构，不要重画二维码，不要移动二维码，不要裁剪二维码，不要破坏二维码可扫描性，不要改变原图光影关系，不要改变盘子透视，不要改变玻璃柜和阴影关系，不要在二维码区域新增明显图案，只允许在盘子四周空白区域填充与参考图相似的花纹。"
};
