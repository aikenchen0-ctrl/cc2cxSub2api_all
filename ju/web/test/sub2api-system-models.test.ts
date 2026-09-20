import { expect, test } from "bun:test";

import { defaultModelCapabilityConfig, normalizeVideoValue } from "../src/lib/model-capabilities";
import { modelCompatibilityError } from "../src/lib/model-selection";
import { systemChannelModelChannels } from "../src/lib/user-session";
import { backendProviderConfig, prepareBackendGenerationTask } from "../src/services/api/generation-task";
import type { PublicChannelCatalog, PublicChannelModel } from "../src/services/api/logical-models";
import { defaultConfig, normalizeConfigSnapshot, resolveModelRequestConfig, selectableModelsByCapability, useConfigStore } from "../src/stores/use-config-store";

function relayCatalog(): PublicChannelCatalog[] {
    const models = ([
        ["text", "gpt-5.5", "chat-completion"],
        ["image", "gpt-image-1", "openai-image"],
        ["video", "minimax_h3_lightx2v_no_pic", "sub2api-video-v1"],
    ] as const).map(([capability, modelKey, protocol]): PublicChannelModel => {
        const capabilityConfig = defaultModelCapabilityConfig(protocol, modelKey);
        if (capability === "video") {
            capabilityConfig.video = {
                ...capabilityConfig.video!,
                references: { ...capabilityConfig.video!.references, minImages: 0, maxImages: 0 },
                operations: ["text_to_video"],
                duration: { selection: "enum", values: [5], default: 5 },
                ratios: [],
                defaultRatio: "",
                resolutions: ["480p横", "480p竖"],
                defaultResolution: "480p横",
            };
        }
        return {
            id: `relay-${capability}`,
            modelKey,
            displayName: modelKey,
            icon: "",
            capability,
            protocol,
            capabilityConfig,
            available: true,
            pricingMode: "fixed",
            displayPrice: 0,
            priceLabel: "0",
            priceTiers: [{ id: `price-${capability}`, selector: {}, resolution: "*", videoSeconds: 0, billingMode: "fixed_request", unitPriceMicrocredits: 0, inputTokenPriceMicrocredits: 0, outputTokenPriceMicrocredits: 0, cachedTokenPriceMicrocredits: 0 }],
        };
    });
    return [{ id: "sub2api-relay", name: "Sub2API", displayName: "智能剧场", sortOrder: -100, models }];
}

function firstLoginConfig() {
    return normalizeConfigSnapshot({ config: { ...defaultConfig, channels: systemChannelModelChannels(relayCatalog()) } }).config;
}

test("first-login catalog selects usable free system models without a user key or channel configuration", () => {
    const config = firstLoginConfig();
    for (const capability of ["text", "image", "video"] as const) {
        const selected = config[`${capability}Model`];
        expect(selectableModelsByCapability(config, capability)).toEqual([selected]);
        expect(selected.startsWith("sub2api-relay::")).toBe(true);
        expect(useConfigStore.getState().isAiConfigReady(config, selected)).toBe(true);
    }
    expect(config.apiKey).toBe("");
    expect(config.channels[0]!.apiKey).toBe("system");
    expect(config.channels[0]!.baseUrl).toBe("/api/sub2api-relay");
    expect(resolveModelRequestConfig(config, config.videoModel).interfaceType).toBe("sub2api-video-v1");
});

test("system task submission contains model identity and creation options but no connection credentials", async () => {
    const initial = firstLoginConfig();
    for (const capability of ["text", "image", "video"] as const) {
        const config = { ...initial, model: initial[`${capability}Model`] };
        const task = await prepareBackendGenerationTask({ mode: capability, prompt: "Test creation", config });
        expect(task.type).toBe(`canvas_${capability}`);
        expect(task.input?.config).toMatchObject({ channelId: "sub2api-relay", model: config.model.split("::")[1] });
        for (const field of ["apiKey", "secretKey", "baseUrl", "headers", "allowLocalChannel", "interfaceType", "capabilityConfig"]) {
            expect(task.input?.config).not.toHaveProperty(field);
        }
    }
    const config = { ...initial, model: initial.videoModel };
    config.channels[0]!.apiKey = "must-remain-server-side";
    expect(JSON.stringify(backendProviderConfig(config, "video"))).not.toContain("must-remain-server-side");
});

test("server video capability defaults work without protocol-specific browser mappings", () => {
    const config = firstLoginConfig();
    const profile = config.channels[0]!.modelCosts!.find((model) => model.capability === "video")!.capabilityConfig!.video!;
    expect(normalizeVideoValue(profile, { seconds: "10", ratio: "16:9", resolution: "720p" })).toMatchObject({ seconds: "5", ratio: "", resolution: "480p横" });
    expect(modelCompatibilityError(config, config.videoModel, { capability: "video", input: { textCount: 1, imageCount: 0, videoCount: 0, audioCount: 0, characterCount: 0 }, videoSeconds: "5" })).toBe("");
});

test("disabled relay models do not become available through existing user preferences", () => {
    const catalog = relayCatalog();
    catalog[0]!.models.find((model) => model.capability === "video")!.available = false;
    const initial = firstLoginConfig();
    const config = normalizeConfigSnapshot({ config: { ...initial, channels: systemChannelModelChannels(catalog) } }).config;
    expect(config.videoModels).toEqual([]);
    expect(config.videoModel).toBe("");
});
