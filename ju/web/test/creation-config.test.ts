import { expect, test } from "bun:test";
import { creationGenerationConfig } from "../src/pages/create/creation-config";
import { defaultModelCapabilityConfig } from "../src/lib/model-capabilities";
import { modelCompatibilityError, modelRequestOptions } from "../src/lib/model-selection";
import { prepareBackendGenerationTask } from "../src/services/api/generation-task";
import { defaultConfig, type AiConfig } from "../src/stores/use-config-store";

const settings = { ratio: "16:9", seconds: "6", videoQuality: "720", quality: "auto", count: "1" };
const models = ["grok-imagine-video-1.5", "seedance-2.0", "kling-v3"];

function directorConfig(): AiConfig {
    return {
        ...defaultConfig,
        videoGenerateAudio: "true",
        videoWatermark: "true",
        channels: [{
            id: "theater", name: "智能剧场", baseUrl: "/api", apiKey: "system", apiFormat: "openai", scope: "system", models,
            modelCosts: models.map((model) => ({
                model, capability: "video", billingMode: "fixed_request", unitPriceMicrocredits: 0,
                logicalModelId: model,
                logicalCapabilitySpec: { version: 1, capability: "video", operations: ["text_to_video"], options: { videoGenerateAudio: { values: [false] }, videoWatermark: { values: [false] } } },
                capabilityConfig: defaultModelCapabilityConfig("sub2api-video-v1", model),
            })),
        }],
        videoModels: models.map((model) => `theater::${model}`),
    };
}

for (const model of models) {
    test(`导演台 ${model} 默认参数和重试请求均关闭不支持的同步音频`, async () => {
        const config = directorConfig();
        const selected = `theater::${model}`;
        expect(modelCompatibilityError(config, selected, { capability: "video", options: modelRequestOptions(config, "video") })).toContain("同步音频");
        const resolved = creationGenerationConfig(config, selected, "video", settings);
        expect(modelCompatibilityError(resolved, selected, { capability: "video", options: modelRequestOptions(resolved, "video") })).toBe("");
        expect(resolved).toMatchObject({ size: "16:9", videoSeconds: "6", vquality: "720", videoGenerateAudio: "false", videoWatermark: "false" });
        for (const retryOf of [undefined, "failed-video-task"]) {
            const request = await prepareBackendGenerationTask({ mode: "video", prompt: "雨夜天台，镜头缓缓推进", config: resolved, retryOf });
            expect(request.logicalModelId).toBe(model);
            expect(request.input.config).toMatchObject({ videoGenerateAudio: "false", videoWatermark: "false", size: "16:9", videoSeconds: "6", vquality: "720" });
            expect(request.input.capabilityOptions).toEqual({ videoGenerateAudio: false, videoWatermark: false });
        }
        expect(config.videoGenerateAudio).toBe("true");
    });
}

test("切换到支持同步音频的平台模型时使用该模型默认值", () => {
    const config = directorConfig();
    const cost = config.channels[0].modelCosts![0];
    cost.capabilityConfig!.video!.generateAudio = { supported: true, default: true };
    config.videoGenerateAudio = "false";
    expect(creationGenerationConfig(config, "theater::grok-imagine-video-1.5", "video", settings).videoGenerateAudio).toBe("true");
    expect(creationGenerationConfig(config, "theater::kling-v3", "video", settings).videoGenerateAudio).toBe("false");
});

test("文本与图片入口保持各自参数", () => {
    const config = directorConfig();
    expect(creationGenerationConfig(config, "theater::kling-v3", "text", settings)).toMatchObject({ videoGenerateAudio: "true", videoSeconds: config.videoSeconds });
    expect(creationGenerationConfig(config, "theater::kling-v3", "image", settings)).toMatchObject({ size: "16:9", count: "1", videoSeconds: config.videoSeconds });
});
