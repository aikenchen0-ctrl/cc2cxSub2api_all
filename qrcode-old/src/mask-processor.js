import sharp from "sharp";

const DEFAULT_MASK_X = 250;
const DEFAULT_MASK_Y = 270;
const DEFAULT_MASK_SIZE = 950;
const MASK_SHAPES = new Set(["square", "circle"]);

function normalizeMaskShape(shape) {
  return MASK_SHAPES.has(shape) ? shape : "square";
}

export function computeFixedMaskBox(templateSize, placement = {}) {
  const left = Number.isFinite(placement.x) ? Math.round(placement.x) : DEFAULT_MASK_X;
  const top = Number.isFinite(placement.y) ? Math.round(placement.y) : DEFAULT_MASK_Y;
  const size = Number.isFinite(placement.size) ? Math.round(placement.size) : DEFAULT_MASK_SIZE;
  const shape = normalizeMaskShape(placement.shape);

  if (templateSize.width < left + size || templateSize.height < top + size) {
    throw new Error(
      `空白盘模板尺寸不足：需要至少 ${left + size}x${top + size}，当前 ${templateSize.width}x${templateSize.height}`
    );
  }

  return {
    left,
    top,
    right: left + size,
    bottom: top + size,
    width: size,
    height: size,
    source: {
      mode: "fixed-absolute",
      x: left,
      y: top,
      size,
      shape
    }
  };
}


export async function trimQrWhiteBorder(qrBuffer, options = {}) {
  const enabled = options.enabled !== false;
  if (!enabled) {
    return { buffer: qrBuffer, info: { enabled: false, trimmed: false } };
  }

  // 手动剪裁模式
  if (options.mode === "manual") {
    const x = Number.isFinite(options.x) ? Math.round(options.x) : 0;
    const y = Number.isFinite(options.y) ? Math.round(options.y) : 0;
    const width = Number.isFinite(options.width) ? Math.round(options.width) : 0;
    const height = Number.isFinite(options.height) ? Math.round(options.height) : 0;
    
    if (width <= 0 || height <= 0) {
      return { buffer: qrBuffer, info: { enabled: true, trimmed: false, reason: "invalid-coords" } };
    }
    
    const source = sharp(qrBuffer).rotate();
    const meta = await source.metadata();
    
    // 确保坐标在图片范围内
    const validX = Math.max(0, Math.min(x, meta.width - 1));
    const validY = Math.max(0, Math.min(y, meta.height - 1));
    const validWidth = Math.min(width, meta.width - validX);
    const validHeight = Math.min(height, meta.height - validY);
    
    if (validWidth <= 0 || validHeight <= 0) {
      return { buffer: qrBuffer, info: { enabled: true, trimmed: false, reason: "out-of-bounds" } };
    }
    
    const buffer = await source
      .extract({ left: validX, top: validY, width: validWidth, height: validHeight })
      .png()
      .toBuffer();
    
    return {
      buffer,
      info: {
        enabled: true,
        trimmed: true,
        mode: "manual",
        originalWidth: meta.width,
        originalHeight: meta.height,
        left: validX,
        top: validY,
        width: validWidth,
        height: validHeight
      }
    };
  }

  // 自动白边检测模式（原有逻辑）
  const threshold = Number.isFinite(options.threshold) ? Math.max(0, Math.min(255, Math.round(options.threshold))) : 245;
  const source = sharp(qrBuffer).rotate().ensureAlpha();
  const meta = await source.metadata();
  const { data } = await source.raw().toBuffer({ resolveWithObject: true });
  const channels = 4;
  let minX = meta.width;
  let minY = meta.height;
  let maxX = -1;
  let maxY = -1;

  for (let y = 0; y < meta.height; y += 1) {
    for (let x = 0; x < meta.width; x += 1) {
      const idx = (y * meta.width + x) * channels;
      const r = data[idx];
      const g = data[idx + 1];
      const b = data[idx + 2];
      const a = data[idx + 3];
      const isWhite = a === 0 || (r >= threshold && g >= threshold && b >= threshold);
      if (!isWhite) {
        if (x < minX) minX = x;
        if (y < minY) minY = y;
        if (x > maxX) maxX = x;
        if (y > maxY) maxY = y;
      }
    }
  }

  if (maxX < minX || maxY < minY) {
    return { buffer: qrBuffer, info: { enabled: true, trimmed: false, reason: "all-white", threshold } };
  }

  const padding = Number.isFinite(options.padding) ? Math.max(0, Math.round(options.padding)) : 0;
  const left = Math.max(0, minX - padding);
  const top = Math.max(0, minY - padding);
  const right = Math.min(meta.width - 1, maxX + padding);
  const bottom = Math.min(meta.height - 1, maxY + padding);
  const width = right - left + 1;
  const height = bottom - top + 1;

  if (left === 0 && top === 0 && width === meta.width && height === meta.height) {
    return {
      buffer: qrBuffer,
      info: { enabled: true, trimmed: false, threshold, padding, originalWidth: meta.width, originalHeight: meta.height }
    };
  }

  const buffer = await sharp(qrBuffer)
    .rotate()
    .extract({ left, top, width, height })
    .png()
    .toBuffer();

  return {
    buffer,
    info: {
      enabled: true,
      trimmed: true,
      threshold,
      padding,
      originalWidth: meta.width,
      originalHeight: meta.height,
      left,
      top,
      width,
      height
    }
  };
}

export async function buildSquareQrPlatePreview({
  templateBuffer,
  qrBuffer,
  placement = {},
  qrTrim = {}
}) {
  const originalTemplateMeta = await sharp(templateBuffer).metadata();
  const requestedPlacement = {
    x: Number.isFinite(placement.x) ? Math.round(placement.x) : DEFAULT_MASK_X,
    y: Number.isFinite(placement.y) ? Math.round(placement.y) : DEFAULT_MASK_Y,
    size: Number.isFinite(placement.size) ? Math.round(placement.size) : DEFAULT_MASK_SIZE,
    shape: normalizeMaskShape(placement.shape)
  };
  const minTemplateWidth = requestedPlacement.x + requestedPlacement.size;
  const minTemplateHeight = requestedPlacement.y + requestedPlacement.size;
  let workingTemplateBuffer = templateBuffer;
  let templatePreprocess = { mode: "none", scale: 1 };

  if (originalTemplateMeta.width < minTemplateWidth || originalTemplateMeta.height < minTemplateHeight) {
    const scale = Math.max(
      minTemplateWidth / originalTemplateMeta.width,
      minTemplateHeight / originalTemplateMeta.height
    );
    const resizedWidth = Math.ceil(originalTemplateMeta.width * scale);
    const resizedHeight = Math.ceil(originalTemplateMeta.height * scale);
    workingTemplateBuffer = await sharp(templateBuffer)
      .rotate()
      .resize(resizedWidth, resizedHeight, { fit: "fill" })
      .png()
      .toBuffer();
    templatePreprocess = {
      mode: "upscale-template-to-fit-fixed-mask",
      scale,
      originalWidth: originalTemplateMeta.width,
      originalHeight: originalTemplateMeta.height,
      resizedWidth,
      resizedHeight
    };
  }

  const templateMeta = await sharp(workingTemplateBuffer).metadata();
  const templateSize = {
    width: templateMeta.width,
    height: templateMeta.height
  };
  const squareBox = computeFixedMaskBox(templateSize, requestedPlacement);

  const trimmedQr = await trimQrWhiteBorder(qrBuffer, qrTrim);

  const fittedQrBase = await sharp(trimmedQr.buffer)
    .rotate()
    .resize(squareBox.width, squareBox.height, {
      fit: "contain",
      background: { r: 255, g: 255, b: 255, alpha: 1 }
    })
    .png()
    .toBuffer();
  const fittedQr = requestedPlacement.shape === "circle"
    ? await sharp(fittedQrBase)
        .ensureAlpha()
        .composite([
          {
            input: Buffer.from(`
              <svg width="${squareBox.width}" height="${squareBox.height}" xmlns="http://www.w3.org/2000/svg">
                <circle cx="${squareBox.width / 2}" cy="${squareBox.height / 2}" r="${squareBox.width / 2}" fill="white" />
              </svg>
            `),
            blend: "dest-in"
          }
        ])
        .png()
        .toBuffer()
    : fittedQrBase;

  const composedBuffer = await sharp(workingTemplateBuffer)
    .composite([
      {
        input: fittedQr,
        left: squareBox.left,
        top: squareBox.top
      }
    ])
    .png()
    .toBuffer();

  // 用户要求：遮罩识别调试图就是实际提交给接口的上下文图。
  // 所以这里不再叠加红框/额外标记，避免预览图和提交图不一致。
  const debugOverlayBuffer = composedBuffer;

  const maskShapeSvg = requestedPlacement.shape === "circle"
    ? `<circle cx="${squareBox.left + squareBox.width / 2}" cy="${squareBox.top + squareBox.height / 2}" r="${squareBox.width / 2}" fill="rgba(255,255,255,1)" />`
    : `<rect x="${squareBox.left}" y="${squareBox.top}" width="${squareBox.width}" height="${squareBox.height}" fill="rgba(255,255,255,1)" />`;

  const editMaskSvg = `
    <svg width="${templateMeta.width}" height="${templateMeta.height}" xmlns="http://www.w3.org/2000/svg">
      <rect width="100%" height="100%" fill="rgba(0,0,0,0)" />
      ${maskShapeSvg}
    </svg>
  `;

  const editMaskBuffer = await sharp(Buffer.from(editMaskSvg))
    .png()
    .toBuffer();

  return {
    composedBuffer,
    debugOverlayBuffer,
    editMaskBuffer,
    templateSize,
    plateBox: null,
    squareBox,
    templatePreprocess,
    qrTrim: trimmedQr.info
  };
}
