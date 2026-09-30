import { modelCapabilityConfigFor, normalizeImageValue, normalizeVideoValue } from "@/lib/model-capabilities";
import { resolveModelVideoBooleanOptions } from "@/lib/model-selection";
import type { AiConfig } from "@/stores/use-config-store";
import type { CreationMode, CreationSettings } from "./creation-types";

export function creationGenerationConfig(config: AiConfig, model: string, mode: CreationMode, settings: CreationSettings): AiConfig {
    const profile = modelCapabilityConfigFor(config, model);
    const result = { ...config, model, imageModel: model, videoModel: model, textModel: model };
    if (mode === "image") {
        const normalized = normalizeImageValue(profile.image!, { size: settings.ratio, quality: settings.quality, count: settings.count });
        return { ...result, size: normalized.size || settings.ratio, quality: normalized.quality || settings.quality, count: normalized.count || settings.count };
    }
    if (mode === "video") {
        const normalized = normalizeVideoValue(profile.video!, { seconds: settings.seconds, ratio: settings.ratio, resolution: settings.videoQuality });
        return {
            ...result,
            size: normalized.ratio,
            videoSeconds: normalized.seconds,
            vquality: normalized.resolution.replace(/p$/i, ""),
            // 首页未提供音频/水印开关，按所选模型能力解析这些隐式默认值。
            ...resolveModelVideoBooleanOptions(config, model, {}, config),
        };
    }
    return result;
}
