import { collectImageDataUrls } from "./image-sources.js";
import { extractConversationText, extractImageCandidates } from "./parser.js";
import { buildResponsesPayload } from "./payload.js";
import { APP_CONFIG } from "./config.js";
import { callResponsesApi } from "./openai-client.js";

export async function analyzeChatLog({
  chatLog,
  callModel = async (payload) =>
    callResponsesApi({
      endpoint: APP_CONFIG.apiBaseUrl,
      apiKey: APP_CONFIG.apiKey,
      payload
    })
}) {
  const imageCandidates = extractImageCandidates(chatLog);
  const cleanedText = extractConversationText(chatLog);
  const collected = await collectImageDataUrls({
    localPaths: imageCandidates.localPaths,
    remoteUrls: imageCandidates.remoteUrls,
    maxImages: APP_CONFIG.maxImages
  });
  const requestPayload = buildResponsesPayload({
    chatText: cleanedText,
    imageDataUrls: collected.images.map((item) => item.dataUrl)
  });
  requestPayload.model = APP_CONFIG.model;

  const modelResult = await callModel(requestPayload);

  return {
    cleanedText,
    imageCount: collected.images.length,
    imageSources: collected.images.map((item) => item.source),
    skippedImages: collected.skipped,
    placeholders: imageCandidates.placeholders,
    requestPayload,
    aiResult: modelResult.outputText
  };
}
