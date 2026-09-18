const legacyModels = ["seedance2.5-9图", "seedance2.0-900-720p", "minimax-h3-933-图文"] as const;

export function legacyGatewayVideoOptions(model: string): { durations: number[]; defaultDuration: number; resolution?: string } {
    switch (model.trim().toLowerCase()) {
        case "seedance2.5-9图": return { durations: [30], defaultDuration: 30, resolution: "720" };
        case "seedance2.0-900-720p": return { durations: [10, 15], defaultDuration: 10 };
        case "minimax-h3-933-图文": return { durations: [5, 10, 15], defaultDuration: 10, resolution: "768" };
        default: throw new Error("Unsupported legacy video model");
    }
}

export function isLegacyGatewayVideoModel(model: string, protocol: string) {
    return protocol === "openai" && legacyModels.some((name) => name === model.trim().toLowerCase());
}

// Preserve the three custom model contracts from canvas-old, not generic provider models.
export function legacyGatewayVideoBody(input: {
    model: string;
    prompt: string;
    seconds: string;
    size: string;
    resolution: string;
    images: string[];
    baseUrl: string;
}) {
    const model = input.model.trim().toLowerCase();
    if (!isLegacyGatewayVideoModel(model, "openai")) throw new Error("Unsupported legacy video model");
    const seedance25 = model === "seedance2.5-9图";
    const minimax = model === "minimax-h3-933-图文";
    const options = legacyGatewayVideoOptions(model);
    const seconds = options.durations.includes(Number(input.seconds)) ? Number(input.seconds) : options.defaultDuration;
    const dimensions = /^(\d+)x(\d+)$/.exec(input.size);
    const portrait = ["9:16", "2:3", "3:4"].includes(input.size) || Boolean(dimensions && Number(dimensions[2]) > Number(dimensions[1]));
    const ratio = portrait ? "9:16" : "16:9";
    const size = minimax ? portrait ? "768x1365" : "1344x768" : seedance25 ? portrait ? "720x1280" : "1280x720" : input.size === "auto" ? undefined : dimensions ? input.size : portrait ? "720x1280" : "1280x720";
    const quality = input.resolution.trim();
    const resolution = options.resolution ? options.resolution + "p" : quality === "low" ? "480p" : ["", "auto", "high", "medium"].includes(quality) ? "720p" : quality.replace(/p$/i, "") + "p";
    const images = input.images.slice(0, 8);
    let vivid = false;
    try { vivid = new URL(input.baseUrl).hostname.toLowerCase() === "aigc.easysu.cn"; } catch { /* Remote channels do not expose an upstream URL. */ }
    const body: Record<string, unknown> = { model: input.model, prompt: input.prompt, seconds: String(seconds), ...(size ? { size } : {}), resolution };
    if (vivid) {
        body.ratio = ratio;
        if (images.length) body[seedance25 ? "reference_images" : "input_reference"] = seedance25 ? images.map((image) => image.replace(/^data:[^,]+,/i, "")) : images;
    } else {
        body.duration = seconds;
        body.aspect_ratio = ratio;
        if (images.length) body.images = images;
    }
    return body;
}
