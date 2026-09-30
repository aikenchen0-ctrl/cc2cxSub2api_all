import type { AutoDLWorkflow } from "@/services/api/autodl";
import { channelIdForActiveModel, channelProtocolForConfig, localChannelForActiveModel, type AiConfig } from "@/stores/use-config-store";
import { getAutoDLCapabilities } from "./autodl-capabilities";
import { autoDLVideoWorkflow } from "./autodl-video-catalog";

export { getAutoDLCapabilities } from "./autodl-capabilities";

export function isAutoDLConfig(config: AiConfig, model = config.model) {
    return channelProtocolForConfig({ ...config, model }) === "autodl" || isManagedAutoDLConfig(config, model);
}

export function isManagedAutoDLConfig(config: AiConfig, model = config.model) {
    const channelId = channelIdForActiveModel({ ...config, model, videoModel: model });
    return config.channelMode === "remote" && Boolean(autoDLVideoWorkflow(model)) &&
        (channelId === "sub2api-relay" || !channelId && config.publicChannels[0]?.id === "sub2api-relay");
}

export function autoDLBaseUrl(config: AiConfig, model = config.model) {
    if (isManagedAutoDLConfig(config, model)) return "";
    const active = { ...config, model };
    const channel = active.channelMode === "remote"
        ? active.publicChannels.find((item) => item.id === channelIdForActiveModel(active)) || active.publicChannels[0]
        : localChannelForActiveModel(active);
    return (channel?.baseUrl || "https://autodl.art").trim().replace(/\/+$/, "");
}

export function normalizeAutoDLDuration(value: string, workflow?: AutoDLWorkflow) {
    const rule = getAutoDLCapabilities(workflow)?.duration;
    if (!rule) return value;
    const parsed = Number(value.trim() || rule.default);
    if (!Number.isFinite(parsed)) return String(rule.default ?? "");
    const seconds = rule.type === "integer" ? Math.floor(parsed) : parsed;
    return String(Math.min(rule.max ?? Infinity, Math.max(rule.min ?? 0, seconds)));
}
