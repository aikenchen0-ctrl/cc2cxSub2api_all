import axios from "axios";
import { nanoid } from "nanoid";

import i18n from "@/i18n";
import { uploadMediaFile, type UploadedFile } from "@/services/file-storage";
import { imageToDataUrl } from "@/services/image-storage";
import { boolConfig, buildApiUrl, modelApiName, modelOptionName, resolveModelRequestConfig, resolveModelScript, type AiConfig } from "@/stores/use-config-store";
import { runModelPlugin } from "./model-plugin";
import type { ReferenceImage } from "@/types/image";

const VIDEO_POLL_INTERVAL_MS = 2500;
const VIDEO_TIMEOUT_MS = 30 * 60 * 1000;
const VIDEO_MAX_ATTEMPTS = Math.ceil(VIDEO_TIMEOUT_MS / VIDEO_POLL_INTERVAL_MS);
const VIDEO_STATUS_REQUEST_TIMEOUT_MS = 30 * 1000;
const VIDEO_CONTENT_REQUEST_TIMEOUT_MS = 2 * 60 * 1000;

type VideoResponse = { id?: string; task_id?: string; request_id?: string; status?: string; state?: string; error?: { message?: string }; url?: string; result_url?: string; video_url?: string; result_urls?: unknown; urls?: unknown; results?: unknown; content?: { video_url?: string; url?: string } | null; data?: VideoResponse | null };
type ApiVideoResponse = VideoResponse | { code?: number | string; data?: VideoResponse | null; msg?: string; message?: string; error?: { message?: string } };
type ApiEnvelope<T> = T | { code?: number | string; data?: T | null; msg?: string; message?: string; error?: { message?: string } };
type RequestOptions = { signal?: AbortSignal };
const apiText = (key: string, options?: Record<string, unknown>) => i18n.t(`apiErrors.${key}`, options);

export type VideoGenerationResult = { blob?: Blob; url?: string; mimeType?: string };
export type VideoGenerationTask = { id: string; provider: "openai" | "plugin"; model: string };
export type VideoGenerationTaskState = { status: "pending" } | { status: "completed"; result: VideoGenerationResult } | { status: "failed"; error: string };

/** Results for scripted (plugin) video models, which run their own create+poll in one shot at task creation. */
const pluginVideoResults = new Map<string, VideoGenerationResult>();

function aiApiUrl(config: AiConfig, path: string) {
    return buildApiUrl(config.baseUrl, path);
}

function aiHeaders(config: AiConfig, contentType?: string) {
    return {
        Authorization: `Bearer ${config.apiKey}`,
        ...(contentType ? { "Content-Type": contentType } : {}),
    };
}

export async function requestVideoGeneration(config: AiConfig, prompt: string, references: ReferenceImage[] = [], options?: RequestOptions): Promise<VideoGenerationResult> {
    const task = await createVideoGenerationTask(config, prompt, references, options);
    for (let attempt = 0; attempt < VIDEO_MAX_ATTEMPTS; attempt += 1) {
        if (options?.signal?.aborted) throw new DOMException("Aborted", "AbortError");
        const state = await pollVideoGenerationTask(config, task, options);
        if (state.status === "completed") return state.result;
        if (state.status === "failed") throw new Error(state.error);
        if (attempt === VIDEO_MAX_ATTEMPTS - 1) throw new Error(apiText("videoTimeout", { provider: "" }));
        await delay(VIDEO_POLL_INTERVAL_MS, options?.signal);
    }
    throw new Error(apiText("videoTimeout", { provider: "" }));
}

export async function createVideoGenerationTask(config: AiConfig, prompt: string, references: ReferenceImage[] = [], options?: RequestOptions): Promise<VideoGenerationTask> {
    const selectedModel = (config.videoModel || config.model).trim();
    const requestConfig = resolveModelRequestConfig(config, selectedModel);
    const script = resolveModelScript(config, selectedModel);
    if (script) return createPluginVideoTask(requestConfig, selectedModel, script, prompt, references, options);
    assertVideoConfig(requestConfig, requestConfig.model);
    return createOpenAIVideoTask(requestConfig, selectedModel, prompt, references, options);
}

export async function pollVideoGenerationTask(config: AiConfig, task: VideoGenerationTask, options?: RequestOptions): Promise<VideoGenerationTaskState> {
    if (task.provider === "plugin") {
        const result = pluginVideoResults.get(task.id);
        return result ? { status: "completed", result } : { status: "failed", error: apiText("pluginVideoExpired") };
    }
    const requestConfig = resolveModelRequestConfig(config, task.model);
    assertVideoConfig(requestConfig, requestConfig.model);
    try {
        return await pollOpenAIVideoTask(requestConfig, task, options);
    } catch (error) {
        // A missing/unauthorized task is terminal. Treating it as transient
        // causes stale tasks to generate an endless 404 polling loop.
        if (isTerminalVideoPollError(error)) {
            return { status: "failed", error: readAxiosError(error, apiText("videoTaskQueryFailed")) };
        }
        throw error;
    }
}

async function createPluginVideoTask(config: AiConfig, model: string, script: string, prompt: string, references: ReferenceImage[], options?: RequestOptions): Promise<VideoGenerationTask> {
    if (!config.baseUrl.trim()) throw new Error(apiText("baseUrlRequired"));
    if (!config.apiKey.trim()) throw new Error(apiText("apiKeyRequired"));
    const refs = await Promise.all(references.map((image) => imageToDataUrl(image)));
    const result = videoPluginResult(
        await runModelPlugin({
            capability: "video",
            script,
            config,
            prompt,
            images: refs,
            params: {
                seconds: normalizeVideoSeconds(config.videoSeconds),
                size: normalizeVideoSize(config.size),
                resolution: normalizeVideoResolution(config.vquality),
                ratio: config.size,
                generateAudio: boolConfig(config.videoGenerateAudio, true),
                watermark: boolConfig(config.videoWatermark, false),
            },
            signal: options?.signal,
        }),
    );
    const id = nanoid();
    pluginVideoResults.set(id, result);
    return { id, provider: "plugin", model };
}

function videoPluginResult(result: unknown): VideoGenerationResult {
    if (result instanceof Blob) return { blob: result };
    if (typeof result === "string") return { url: result, mimeType: "video/mp4" };
    if (result && typeof result === "object") {
        const record = result as Record<string, unknown>;
        if (record.blob instanceof Blob) return { blob: record.blob };
        const url = [record.url, record.video_url, record.result_url].find((value) => typeof value === "string" && value) as string | undefined;
        if (url) return { url, mimeType: "video/mp4" };
    }
    throw new Error(apiText("scriptNoVideo"));
}

export async function storeGeneratedVideo(result: VideoGenerationResult): Promise<UploadedFile> {
    if (result.blob) return uploadMediaFile(result.blob, "video");
    if (result.url) {
        try {
            return await uploadMediaFile(result.url, "video");
        } catch {
            return { url: result.url, storageKey: "", bytes: 0, mimeType: result.mimeType || "video/mp4" };
        }
    }
    throw new Error(apiText("noPlayableVideo"));
}

async function createOpenAIVideoTask(config: AiConfig, model: string, prompt: string, references: ReferenceImage[], options?: RequestOptions): Promise<VideoGenerationTask> {
    const images = await Promise.all(references.slice(0, 8).map((image) => imageToDataUrl(image)));
    const upstreamModel = modelApiName(model);
    const requestedSeconds = /seedance2\.5-9图/i.test(upstreamModel)
        ? 30
        : /seedance2\.0-900-720p/i.test(upstreamModel)
            ? Number(config.videoSeconds) === 15
                ? 15
                : 10
        : /minimax-h3-933-图文/i.test(upstreamModel)
          ? [5, 10, 15].includes(Number(config.videoSeconds))
              ? Number(config.videoSeconds)
              : 10
        : /veo/i.test(upstreamModel)
          ? 8
          : Number(normalizeVideoSeconds(config.videoSeconds)) || 8;
    const requestedSize = normalizeVideoSizeForModel(config.size, upstreamModel);
    const requestedResolution = normalizeVideoResolutionForModel(config.vquality, upstreamModel);
    const aspectRatio = config.size === "9:16" || config.size === "2:3" || config.size === "3:4" ? "9:16" : "16:9";
    const vivid = /aigc\.easysu\.cn/i.test(requestConfigBaseUrl(config));
    // Vivid's documented schema uses seconds, ratio and pure-base64
    // reference_images. Other OpenAI-compatible providers keep the broader
    // compatibility aliases used by the existing gateway.
    const vividReferences = images.map(stripImageDataUrlPrefix);
    const vividReferenceField = /seedance2\.5-9图/i.test(upstreamModel) ? "reference_images" : "input_reference";
    const body = vivid ? {
        model: upstreamModel,
        prompt,
        seconds: String(requestedSeconds),
        ...(requestedSize ? { size: requestedSize } : {}),
        resolution: requestedResolution,
        ratio: aspectRatio,
        ...(images.length ? { [vividReferenceField]: /seedance2\.5-9图/i.test(upstreamModel) ? vividReferences : images } : {}),
    } : {
        model: upstreamModel,
        prompt,
        seconds: String(requestedSeconds),
        duration: requestedSeconds,
        ...(requestedSize ? { size: requestedSize } : {}),
        resolution: requestedResolution,
        aspect_ratio: aspectRatio,
        ...(images.length ? { images } : {}),
    };
    try {
        const created = unwrapVideoResponse((await axios.post<ApiVideoResponse>(aiApiUrl(config, "/videos"), body, { headers: aiHeaders(config, "application/json"), signal: options?.signal, timeout: VIDEO_STATUS_REQUEST_TIMEOUT_MS })).data);
        const taskId = created.id || created.task_id || created.request_id;
        if (!taskId) throw new Error(apiText("noVideoTaskId"));
        return { id: taskId, provider: "openai", model };
    } catch (error) {
        throw new Error(readAxiosError(error, apiText("videoTaskCreateFailed")));
    }
}

function requestConfigBaseUrl(config: AiConfig) {
    return config.baseUrl.trim();
}

function stripImageDataUrlPrefix(value: string) {
    const comma = value.indexOf(",");
    return value.toLowerCase().startsWith("data:") && comma >= 0 ? value.slice(comma + 1) : value;
}

async function pollOpenAIVideoTask(config: AiConfig, task: VideoGenerationTask, options?: RequestOptions): Promise<VideoGenerationTaskState> {
    try {
        const video = unwrapVideoResponse((await axios.get<ApiVideoResponse>(aiApiUrl(config, `/videos/${task.id}`), { headers: aiHeaders(config), signal: options?.signal, timeout: VIDEO_STATUS_REQUEST_TIMEOUT_MS })).data);
        const url = videoResultUrl(video);
        if (url) return { status: "completed", result: await videoResultFromUrl(config, url, options) };
        if (isCompletedVideoStatus(video.status || video.state)) {
            const content = await axios.get<Blob>(aiApiUrl(config, `/videos/${task.id}/content`), { headers: aiHeaders(config), responseType: "blob", signal: options?.signal, timeout: VIDEO_CONTENT_REQUEST_TIMEOUT_MS });
            await assertVideoBlob(content.data);
            return { status: "completed", result: { blob: content.data } };
        }
        if (isFailedVideoStatus(video.status || video.state)) return { status: "failed", error: readApiErrorMessage(video.error?.message) || apiText("videoGenerationFailed") };
        return { status: "pending" };
    } catch (error) {
        throw new Error(readAxiosError(error, apiText("videoTaskQueryFailed")));
    }
}

async function videoResultFromUrl(config: AiConfig, url: string, options?: RequestOptions): Promise<VideoGenerationResult> {
    const resolvedUrl = resolveVideoUrl(config, url);
    try {
        const response = await axios.get<Blob>(resolvedUrl, {
            ...(isPublicMediaUrl(url) ? {} : { headers: aiHeaders(config) }),
            responseType: "blob",
            signal: options?.signal,
            timeout: VIDEO_CONTENT_REQUEST_TIMEOUT_MS,
        });
        await assertVideoBlob(response.data);
        return { blob: response.data };
    } catch (error) {
        if (axios.isCancel(error) || options?.signal?.aborted) throw error;
        return { url: resolvedUrl, mimeType: "video/mp4" };
    }
}

function assertVideoConfig(config: AiConfig, model: string) {
    if (!model) throw new Error(apiText("videoModelRequired"));
    if (!config.baseUrl.trim()) throw new Error(apiText("baseUrlRequired"));
    if (!config.apiKey.trim()) throw new Error(apiText("apiKeyRequired"));
    if (config.apiFormat === "gemini") throw new Error(apiText("geminiVideoUnsupported"));
}

function normalizeVideoSeconds(value: string) {
    const seconds = Math.floor(Number(value) || 6);
    return String(Math.max(1, Math.min(30, seconds)));
}

function normalizeVideoSize(value: string) {
    if (value === "auto") return null;
    const size = value || "1280x720";
    if (/^\d+x\d+$/.test(size)) return size;
    return ["9:16", "2:3", "3:4"].includes(size) ? "720x1280" : "1280x720";
}

function normalizeVideoSizeForModel(value: string, model: string) {
    const normalized = model.toLowerCase();
    const portrait = value === "9:16" || value === "2:3" || value === "3:4" || /^\d+x\d+$/.test(value) && Number(value.split("x")[1]) > Number(value.split("x")[0]);
    if (normalized === "minimax-h3-933-图文") return portrait ? "768x1365" : "1344x768";
    if (normalized === "seedance2.5-9图") return portrait ? "720x1280" : "1280x720";
    return normalizeVideoSize(value);
}

function normalizeVideoResolution(value: string) {
    if (value === "low") return "480p";
    if (value === "auto" || value === "high" || value === "medium") return "720p";
    const resolution = value.replace(/p$/i, "") || "720";
    return `${resolution}p`;
}

function normalizeVideoResolutionForModel(value: string, model: string) {
    const normalized = model.toLowerCase();
    if (normalized === "minimax-h3-933-图文") return "768p";
    if (normalized === "seedance2.5-9图") return "720p";
    return normalizeVideoResolution(value);
}

function unwrapVideoResponse(payload: ApiVideoResponse) {
    const value = unwrapEnvelope(payload, apiText("noVideoTask"));
    // Some gateways return {data:{data:{...}}}; normalize all envelope
    // variants before polling so task ids and URLs are never lost.
    let current = value as VideoResponse;
    for (let depth = 0; depth < 3 && current?.data; depth += 1) current = current.data;
    return current;
}

function unwrapEnvelope<T>(payload: ApiEnvelope<T>, emptyMessage: string): T {
    if (!payload) throw new Error(emptyMessage);
    if (typeof payload === "object" && "code" in payload && payload.code !== undefined) {
        if (!isSuccessfulEnvelopeCode(payload.code)) throw new Error(readApiErrorMessage(payload) || apiText("requestFailed"));
        if (!payload.data) throw new Error(emptyMessage);
        return payload.data;
    }
    return payload as T;
}

function videoResultUrl(payload: VideoResponse) {
    const value = payload as VideoResponse & { result_urls?: unknown; results?: unknown; urls?: unknown };
    const candidates = [
        payload.video_url,
        payload.result_url,
        payload.url,
        payload.content?.video_url,
        payload.content?.url,
        payload.data?.video_url,
        payload.data?.result_url,
        payload.data?.url,
        ...(Array.isArray(value.result_urls) ? value.result_urls : []),
        ...(Array.isArray(value.urls) ? value.urls : []),
        ...(Array.isArray(value.results) ? value.results.flatMap((item) => (typeof item === "string" ? [item] : item && typeof item === "object" ? [(item as { url?: unknown }).url] : [])) : []),
    ];
    return candidates.find((url): url is string => typeof url === "string" && (isPublicMediaUrl(url) || url.startsWith("/") || /\.mp4(\?|#|$)/i.test(url)));
}

function isSuccessfulEnvelopeCode(code: number | string | undefined) {
    // AutoDL wraps successful workflow responses with {code:"Success"}.
    // Keep the numeric conventions used by the other OpenAI-compatible
    // gateways as well.
    return code === undefined || code === 0 || code === "0" || code === 200 || code === "200" || code === 201 || code === "201" || code === "Success" || code === "success";
}

function isCompletedVideoStatus(status: string | undefined) {
    return ["completed", "complete", "done", "succeeded", "success", "finished"].includes(String(status || "").toLowerCase());
}

function isFailedVideoStatus(status: string | undefined) {
    return ["failed", "failure", "error", "cancelled", "canceled", "rejected"].includes(String(status || "").toLowerCase());
}

function resolveVideoUrl(config: AiConfig, url: string) {
    if (isPublicMediaUrl(url)) return url;
    try {
        return new URL(url, `${config.baseUrl.trim().replace(/\/+$/, "")}/`).toString();
    } catch {
        return url;
    }
}

function readApiErrorMessage(value: unknown): string {
    if (!value) return "";
    if (typeof value === "string") {
        try {
            const parsed = JSON.parse(value);
            const inner = readApiErrorMessage(parsed) || value;
            if (inner === value && typeof parsed === "object" && Object.keys(parsed).length === 0) return "";
            return inner;
        } catch {
            if (/<[a-z][\s\S]*>/i.test(value)) return apiText("htmlError", { preview: `${value.slice(0, 80)}...` });
            return value;
        }
    }
    if (typeof value !== "object") return "";
    const payload = value as { msg?: unknown; message?: unknown; error?: unknown; detail?: unknown };
    // error may be a string or an object containing a message.
    const errorMsg =
        typeof payload.error === "string"
            ? payload.error
            : (payload.error as { message?: unknown })?.message;
    return (
        readApiErrorMessage(payload.msg) ||
        readApiErrorMessage(payload.message) ||
        readApiErrorMessage(errorMsg) ||
        readApiErrorMessage(payload.detail) ||
        ""
    );
}

function readAxiosError(error: unknown, fallback: string) {
    if (axios.isCancel(error)) return apiText("requestCanceled");
    if (axios.isAxiosError<{ error?: { message?: string }; msg?: string; message?: string; code?: number | string }>(error)) {
        if (!error.response && error.code === "ERR_NETWORK") return apiText("requestFailed");
        const responseData = error.response?.data;
        return readApiErrorMessage(responseData) || statusMessage(error.response?.status, fallback);
    }
    if (error instanceof DOMException && error.name === "AbortError") return apiText("requestCanceled");
    return error instanceof Error ? readApiErrorMessage(error.message) || error.message : fallback;
}

function isTerminalVideoPollError(error: unknown) {
    if (!axios.isAxiosError(error)) return false;
    const status = error.response?.status;
    return status === 400 || status === 401 || status === 403 || status === 404;
}

function statusMessage(status: number | undefined, fallback: string) {
    if (status === 401 || status === 403) return apiText("authenticationFailed");
    if (status === 429) return apiText("rateLimited");
    return status ? `${fallback}（${status}）` : fallback;
}

async function assertVideoBlob(blob: Blob) {
    if (!blob.type.includes("json")) return;
    let payload: { code?: number; msg?: string; error?: { message?: string } };
    try {
        payload = JSON.parse(await blob.text()) as { code?: number; msg?: string; error?: { message?: string } };
    } catch {
        return;
    }
    if (typeof payload.code === "number" && payload.code !== 0) throw new Error(readApiErrorMessage(payload) || apiText("videoDownloadFailed"));
    if (payload.error?.message) throw new Error(readApiErrorMessage(payload.error.message) || payload.error.message);
}

function isPublicMediaUrl(value: string) {
    return /^https?:\/\//i.test(value || "");
}

function delay(ms: number, signal?: AbortSignal) {
    return new Promise<void>((resolve, reject) => {
        if (signal?.aborted) {
            reject(new DOMException("Aborted", "AbortError"));
            return;
        }
        const timer = setTimeout(resolve, ms);
        signal?.addEventListener(
            "abort",
            () => {
                clearTimeout(timer);
                reject(new DOMException("Aborted", "AbortError"));
            },
            { once: true },
        );
    });
}
