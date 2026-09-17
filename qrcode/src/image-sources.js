import fs from "node:fs/promises";
import path from "node:path";

const mimeTypes = {
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".gif": "image/gif",
  ".webp": "image/webp",
  ".bmp": "image/bmp"
};

function inferMimeType(source) {
  const extension = path.extname(source).toLowerCase();
  return mimeTypes[extension] ?? "application/octet-stream";
}

async function readLocalImage(filePath) {
  const buffer = await fs.readFile(filePath);
  return {
    source: filePath,
    dataUrl: `data:${inferMimeType(filePath)};base64,${buffer.toString("base64")}`
  };
}

async function readRemoteImage(url) {
  const response = await fetch(url);

  if (!response.ok) {
    throw new Error(`无法下载远程图片: ${url}`);
  }

  const arrayBuffer = await response.arrayBuffer();
  const buffer = Buffer.from(arrayBuffer);
  const mimeType = response.headers.get("content-type") || inferMimeType(url);

  return {
    source: url,
    dataUrl: `data:${mimeType};base64,${buffer.toString("base64")}`
  };
}

export async function collectImageDataUrls({
  localPaths,
  remoteUrls,
  maxImages = 6
}) {
  const images = [];
  const skipped = [];
  const seen = new Set();

  async function tryLoad(source, loader) {
    if (images.length >= maxImages) {
      skipped.push({ source, reason: "超过最大图片数量限制" });
      return;
    }

    if (seen.has(source)) {
      return;
    }

    seen.add(source);

    try {
      images.push(await loader(source));
    } catch (error) {
      skipped.push({ source, reason: error.message });
    }
  }

  for (const filePath of localPaths) {
    await tryLoad(filePath, readLocalImage);
  }

  for (const url of remoteUrls) {
    await tryLoad(url, readRemoteImage);
  }

  return { images, skipped };
}
