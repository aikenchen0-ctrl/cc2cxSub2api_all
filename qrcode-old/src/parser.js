const windowsLocalImagePathPattern =
  /[A-Za-z]:\\[^\r\n<>"]+\.(?:png|jpg|jpeg|gif|webp|bmp)/gi;
const posixLocalImagePathPattern =
  /(?:^|\s)(\/[^\s<>"']+\.(?:png|jpg|jpeg|gif|webp|bmp))/gim;
const remoteImageUrlPattern =
  /https?:\/\/[^\s<>"']+\.(?:png|jpg|jpeg|gif|webp|bmp)(?:\?[^\s<>"']*)?/gi;
const placeholderPattern = /<image name=\[([^\]]+)\]>/gi;

function unique(values) {
  return [...new Set(values)];
}

export function extractImageCandidates(chatLog) {
  const posixLocalPaths = [...chatLog.matchAll(posixLocalImagePathPattern)].map(
    (match) => match[1]
  );

  return {
    localPaths: unique([
      ...(chatLog.match(windowsLocalImagePathPattern) ?? []),
      ...posixLocalPaths
    ]),
    remoteUrls: unique(chatLog.match(remoteImageUrlPattern) ?? []),
    placeholders: unique(
      [...chatLog.matchAll(placeholderPattern)].map((match) => match[1])
    )
  };
}

export function extractConversationText(chatLog) {
  return chatLog
    .replace(/<image name=\[[^\]]+\]>\s*<\/image>/gi, "")
    .replace(/\r/g, "")
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean)
    .join("\n");
}
