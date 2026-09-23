import type { AutoDLWorkflow } from "@/services/api/autodl";
import { channelIdForActiveModel, channelProtocolForConfig, localChannelForActiveModel, type AiConfig } from "@/stores/use-config-store";
import { getAutoDLCapabilities } from "./autodl-capabilities";

export { getAutoDLCapabilities } from "./autodl-capabilities";

export function isAutoDLConfig(config: AiConfig, model = config.model) {
    return channelProtocolForConfig({ ...config, model }) === "autodl";
}

export function autoDLBaseUrl(config: AiConfig, model = config.model) {
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
