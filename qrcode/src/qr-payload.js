import { APP_CONFIG } from "./config.js";

export function buildQrEditPayload({ images, positivePrompt, negativePrompt }) {
  const payload = {
    model: APP_CONFIG.model,
    size: APP_CONFIG.qrOutputSize,
    prompt: positivePrompt,
    image: images
  };

  if (negativePrompt) {
    payload.negative_prompt = negativePrompt;
  }

  return payload;
}
