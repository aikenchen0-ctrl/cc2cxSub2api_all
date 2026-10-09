import { useEffect, useRef, useState } from "react";
import { useProjectStore } from "../../store/project-store";
import { useAppStore } from "../../store/app-store";
import { AppState } from "../../types";
import {
  AgentEvent,
  AgentEventType,
} from "../commits/types";
import {
  BsChatDots,
  BsChevronDown,
  BsChevronRight,
  BsLightbulb,
  BsFileEarmarkPlus,
  BsPencilSquare,
  BsImage,
  BsScissors,
  BsFiles,
  BsBookmarkCheck,
  BsBoundingBox,
  BsCamera,
} from "react-icons/bs";
import ReactMarkdown from "react-markdown";
import { Light as SyntaxHighlighterBase } from "react-syntax-highlighter";
import html from "react-syntax-highlighter/dist/esm/languages/hljs/xml";
import { vs2015 } from "react-syntax-highlighter/dist/esm/styles/hljs";
import WorkingPulse from "../core/WorkingPulse";
import { groupCompletedAgentEvents } from "./activity-order";
import {
  formatDurationBetween,
  isTerminalVariantStatus,
} from "./generation-time";

SyntaxHighlighterBase.registerLanguage("html", html);
const SyntaxHighlighter = SyntaxHighlighterBase as any;

function ExpandablePrompt({ prompt }: { prompt: string }) {
  const [isExpanded, setIsExpanded] = useState(false);
  const [isClamped, setIsClamped] = useState(false);
  const promptRef = useRef<HTMLParagraphElement>(null);

  useEffect(() => {
    setIsExpanded(false);
  }, [prompt]);

  useEffect(() => {
    const element = promptRef.current;
    if (!element) return;

    const updateClampedState = () => {
      setIsClamped(element.scrollHeight > element.clientHeight + 1);
    };

    updateClampedState();
    if (typeof ResizeObserver === "undefined") return;

    const resizeObserver = new ResizeObserver(updateClampedState);
    resizeObserver.observe(element);
    return () => resizeObserver.disconnect();
  }, [prompt, isExpanded]);

  return (
    <div>
      <p
        ref={promptRef}
        className={`whitespace-pre-wrap break-words text-xs text-gray-600 dark:text-gray-300 ${
          !isExpanded ? "line-clamp-4" : ""
        }`}
      >
        {prompt}
      </p>
      {(isClamped || isExpanded) && (
        <div className="mt-1 flex justify-end">
          <button
            type="button"
            onClick={() => setIsExpanded((previous) => !previous)}
            aria-expanded={isExpanded}
            className="rounded-full bg-gray-100 px-2 py-0.5 text-[11px] font-medium text-gray-600 transition-colors hover:bg-gray-200 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-gray-700"
          >
            {isExpanded ? "收起" : "展开"}
          </button>
        </div>
      )}
    </div>
  );
}

function CodePreviewBlock({ code, isGenerating }: { code: string; isGenerating: boolean }) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (isGenerating && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [code, isGenerating]);

  return (
    <div ref={containerRef} className="max-h-60 overflow-auto rounded-md">
      <SyntaxHighlighter
        language="html"
        style={vs2015}
        customStyle={{ margin: 0, padding: "0.5rem", fontSize: "0.75rem", borderRadius: "0.375rem" }}
        wrapLongLines
      >
        {code}
      </SyntaxHighlighter>
    </div>
  );
}

function formatDuration(startedAt?: number, endedAt?: number): string {
  return formatDurationBetween(startedAt, endedAt);
}

function formatElapsedSince(timestampMs: number | undefined, nowMs: number): string {
  return formatDurationBetween(timestampMs, nowMs);
}

function getArrayField(value: unknown, field: string): unknown[] | null {
  if (!value || typeof value !== "object") return null;
  const fieldValue = (value as Record<string, unknown>)[field];
  return Array.isArray(fieldValue) ? fieldValue : null;
}

function getRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
}

function getExtractedAssetPreviewUrl(asset: unknown): string | null {
  const assetRecord = getRecord(asset);
  if (!assetRecord) return null;
  if (typeof assetRecord.public_url === "string" && assetRecord.public_url) {
    return assetRecord.public_url;
  }
  if (typeof assetRecord.data_url === "string" && assetRecord.data_url) {
    return assetRecord.data_url;
  }
  return null;
}

function isSuccessfulExtractedAsset(asset: unknown): boolean {
  const assetRecord = getRecord(asset);
  if (!assetRecord) return false;
  const status =
    typeof assetRecord.status === "string" ? assetRecord.status : null;
  return Boolean(
    getExtractedAssetPreviewUrl(asset) &&
      status !== "missing" &&
      status !== "error"
  );
}

function getEventIcon(type: AgentEventType, toolName?: string) {
  if (type === "thinking") {
    return <BsLightbulb className="text-yellow-500" />;
  }
  if (type === "assistant") {
    return <BsChatDots className="text-blue-500" />;
  }
  if (toolName === "create_file") {
    return <BsFileEarmarkPlus className="text-indigo-500" />;
  }
  if (toolName === "edit_file") {
    return <BsPencilSquare className="text-purple-500" />;
  }
  if (toolName === "generate_images") {
    return <BsImage className="text-pink-500" />;
  }
  if (toolName === "remove_backgrounds") {
    return <BsScissors className="text-teal-500" />;
  }
  if (toolName === "edit_images") {
    return <BsImage className="text-violet-500" />;
  }
  if (toolName === "retrieve_option") {
    return <BsFiles className="text-slate-500" />;
  }
  if (toolName === "save_assets") {
    return <BsBookmarkCheck className="text-emerald-500" />;
  }
  if (toolName === "extract_assets") {
    return <BsBoundingBox className="text-orange-500" />;
  }
  if (toolName === "screenshot_preview") {
    return <BsCamera className="text-cyan-500" />;
  }
  return <BsFileEarmarkPlus className="text-gray-500" />;
}

function getEventTitle(event: AgentEvent): string {
  if (event.type === "thinking") {
    if (event.status === "running") return "正在思考";
    const duration = formatDuration(event.startedAt, event.endedAt);
    return duration ? `思考用时${duration}` : "已思考";
  }
  if (event.type === "assistant") {
    return "AI 回复";
  }
  if (event.type === "tool") {
    if (event.toolName === "create_file") {
      return event.status === "running" ? "正在创建文件" : "已创建文件";
    }
    if (event.toolName === "edit_file") {
      return event.status === "running" ? "正在编辑文件" : "已编辑文件";
    }
    if (event.toolName === "generate_images") {
      const input = event.input as any;
      const output = event.output as any;
      const count = output?.images?.length || input?.count || 0;
      if (event.status === "running") {
        return count ? `正在生成${count} image${count !== 1 ? "s" : ""}` : "正在生成图片";
      }
      return count ? `已生成${count} image${count !== 1 ? "s" : ""}` : "已生成图片";
    }
    if (event.toolName === "remove_backgrounds") {
      const rbInput = event.input as any;
      const rbOutput = event.output as any;
      const rbCount = rbOutput?.images?.length || rbInput?.image_urls?.length || 0;
      if (event.status === "running") {
        return rbCount > 1 ? `正在移除${rbCount} backgrounds` : "正在移除背景";
      }
      return rbCount > 1 ? `已移除${rbCount} backgrounds` : "已移除背景";
    }
    if (event.toolName === "edit_images") {
      const editInput = event.input as { edits?: unknown[] } | null;
      const editOutput = event.output as { images?: unknown[] } | null;
      const editCount =
        editOutput?.images?.length || editInput?.edits?.length || 0;
      if (event.status === "running") {
        return editCount
          ? `正在编辑${editCount} image${editCount !== 1 ? "s" : ""}`
          : "正在编辑图片";
      }
      return editCount
        ? `已编辑${editCount} image${editCount !== 1 ? "s" : ""}`
        : "已编辑图片";
    }
    if (event.toolName === "retrieve_option") {
      return event.status === "running"
        ? "正在读取方案"
        : "已读取方案";
    }
    if (event.toolName === "save_assets") {
      const saveInput = event.input as any;
      const saveOutput = event.output as any;
      const saveCount = saveOutput?.images?.length || saveInput?.asset_ids?.length || 0;
      if (event.status === "running") {
        return saveCount > 1 ? `正在保存${saveCount} assets` : "正在保存素材";
      }
      return saveCount > 1 ? `已保存${saveCount} assets` : "已保存素材";
    }
    if (event.toolName === "extract_assets") {
      const extractInputDescriptions = getArrayField(event.input, "asset_descriptions");
      const extractOutputAssets = getArrayField(event.output, "assets");
      const requestedCount = extractInputDescriptions?.length || 0;
      const successfulCount =
        extractOutputAssets?.filter(isSuccessfulExtractedAsset).length || 0;
      const extractCount =
        extractOutputAssets?.length || extractInputDescriptions?.length || 0;
      if (event.status === "running") {
        return extractCount > 1
          ? `正在提取${extractCount} assets`
          : "正在提取素材";
      }
      if (
        extractOutputAssets &&
        requestedCount > 0 &&
        successfulCount < requestedCount
      ) {
        return successfulCount > 0
          ? `已提取${successfulCount}/${requestedCount} assets`
          : "素材提取失败";
      }
      return extractCount > 1
        ? `已提取${extractCount} assets`
        : "已提取素材";
    }
    if (event.toolName === "screenshot_preview") {
      return event.status === "running"
        ? "正在截取预览画面"
        : "已截取预览画面";
    }
    return event.status === "running" ? "正在调用工具" : "工具执行完成";
  }
  return "执行过程";
}


function renderToolDetails(event: AgentEvent, variantCode?: string) {
  if (!event.input && !event.output) return null;

  const renderJson = (data: unknown) => {
    if (!data) return null;
    let json = "";
    try {
      json = JSON.stringify(data, null, 2);
    } catch {
      json = String(data);
    }
    if (json.length > 900) {
      json = json.slice(0, 900) + "...";
    }
    return (
      <pre className="mt-2 rounded-md bg-gray-50 dark:bg-gray-800 p-2 text-xs text-gray-700 dark:text-gray-200 overflow-x-auto">
        {json}
      </pre>
    );
  };

  const output = event.output as any;
  const input = event.input as any;
  const hasError = Boolean(output?.error);
  const images =
    output && Array.isArray(output.images) ? (output.images as Array<any>) : null;
  const edits =
    output && Array.isArray(output.edits) ? (output.edits as Array<any>) : null;
  const extractedAssets =
    output && Array.isArray(output.assets) ? (output.assets as Array<unknown>) : null;
  const successfulExtractedAssets =
    extractedAssets?.filter(isSuccessfulExtractedAsset) || null;
  const visibleExtractedAssets =
    hasError && successfulExtractedAssets
      ? successfulExtractedAssets
      : extractedAssets;
  const screenshotPreviews =
    output && Array.isArray(output.screenshots)
      ? (output.screenshots as Array<unknown>)
      : [];

  return (
    <div className="text-sm text-gray-700 dark:text-gray-200">
      {hasError && (
        <div className="rounded-md border border-red-200 dark:border-red-700 bg-red-50 dark:bg-red-900/30 p-3">
          <div className="text-xs uppercase tracking-wide text-red-500">错误</div>
          <div className="mt-1 text-sm text-red-700 dark:text-red-200">
            {output?.error}
          </div>
          {event.input && (
            <div className="mt-2">
              <div className="text-xs uppercase tracking-wide text-red-400">

                输入
              </div>
              {renderJson(event.input)}
            </div>
          )}
        </div>
      )}
      {event.toolName === "create_file" && !hasError && variantCode && (
        <CodePreviewBlock code={variantCode} isGenerating={event.status === "running"} />
      )}

      {event.toolName === "edit_file" && edits && !hasError && (
        <div className="space-y-2">
          {edits.map((edit, index) => (
            <div
              key={`${edit.old_text}-${index}`}
              className="rounded-md border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/60 p-3"
            >
              <div className="text-xs uppercase tracking-wide text-gray-400">

                编辑 {index + 1}
              </div>
              <div className="mt-2 grid gap-2">
                <div>
                  <div className="text-xs text-gray-500">原内容</div>
                  <div className="mt-1 rounded bg-red-50 dark:bg-red-900/30 p-2 text-xs font-mono text-red-700 dark:text-red-200 break-all">
                    {edit.old_text}
                  </div>
                </div>
                <div>
                  <div className="text-xs text-gray-500">新内容</div>
                  <div className="mt-1 rounded bg-emerald-50 dark:bg-emerald-900/30 p-2 text-xs font-mono text-emerald-700 dark:text-emerald-200 break-all">
                    {edit.new_text}
                  </div>
                </div>
              </div>
              {edit.replaced !== undefined && (
                <div className="mt-2 text-xs text-gray-500">

                  已替换 {edit.replaced}  次{edit.replaced === 1 ? "" : "s"}
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {event.toolName === "generate_images" && !hasError && (
        <div>
          {/* While running: show prompts with dividers */}
          {event.status === "running" && input?.prompts && Array.isArray(input.prompts) && (
            <div className="divide-y divide-gray-100 dark:divide-gray-800">
              {input.prompts.map((prompt: string, index: number) => (
                <div key={index} className="text-xs text-gray-600 dark:text-gray-400 py-1.5">
                  {prompt}
                </div>
              ))}
            </div>
          )}
          {/* After complete: 50/50 image left, prompt right */}
          {event.status !== "running" && images && (
            <div className="divide-y divide-gray-100 dark:divide-gray-800">
              {images.map((item, index) => (
                <div key={`${item.prompt}-${index}`} className="flex gap-3 py-2">
                  <div className="w-1/2 shrink-0">
                    {item.url ? (
                      <img
                        src={item.url}
                        alt={item.prompt || `生成的图片${index + 1}`}
                        className="w-full rounded object-cover"
                        loading="lazy"
                      />
                    ) : (
                      <div className="aspect-square rounded bg-gray-100 dark:bg-gray-800 flex items-center justify-center text-xs text-gray-400">

                        失败
                      </div>
                    )}
                  </div>
                  <div className="w-1/2 text-xs text-gray-600 dark:text-gray-400 self-center">
                    {item.prompt}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {event.toolName === "remove_backgrounds" && !hasError && (
        <div>
          {/* While running: show the source images */}
          {event.status === "running" && input?.image_urls && Array.isArray(input.image_urls) && (
            <div className="divide-y divide-gray-100 dark:divide-gray-800">
              {input.image_urls.map((url: string, index: number) => (
                <div key={index} className="py-2">
                  <img
                    src={url}
                    alt={`原始图片${index + 1}`}
                    className="w-full rounded object-cover"
                    loading="lazy"
                  />
                </div>
              ))}
            </div>
          )}
          {/* After complete: before/after side by side for each image */}
          {event.status !== "running" && output?.images && Array.isArray(output.images) && (
            <div className="divide-y divide-gray-100 dark:divide-gray-800">
              {output.images.map((item: any, index: number) => (
                <div key={`${item.image_url}-${index}`} className="flex gap-2 py-2">
                  <div className="w-1/2">
                    <div className="text-xs text-gray-500 dark:text-gray-400 mb-1">修改前</div>
                    <img
                      src={item.image_url}
                      alt={`原始图片${index + 1}`}
                      className="w-full rounded object-cover"
                      loading="lazy"
                    />
                  </div>
                  <div className="w-1/2">
                    <div className="text-xs text-gray-500 dark:text-gray-400 mb-1">修改后</div>
                    {item.result_url ? (
                      <div className="relative">
                        <div
                          className="absolute inset-0 rounded"
                          style={{
                            backgroundImage:
                              "linear-gradient(45deg, #e5e7eb 25%, transparent 25%), linear-gradient(-45deg, #e5e7eb 25%, transparent 25%), linear-gradient(45deg, transparent 75%, #e5e7eb 75%), linear-gradient(-45deg, transparent 75%, #e5e7eb 75%)",
                            backgroundSize: "10px 10px",
                            backgroundPosition: "0 0, 0 5px, 5px -5px, -5px 0px",
                          }}
                        />
                        <img
                          src={item.result_url}
                          alt="已移除背景"
                          className="relative w-full rounded"
                          loading="lazy"
                        />
                      </div>
                    ) : (
                      <div className="aspect-square rounded bg-gray-100 dark:bg-gray-800 flex items-center justify-center text-xs text-gray-400">

                        失败
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {event.toolName === "edit_images" && !hasError && (
        <div>
          {event.status === "running" &&
            input?.edits &&
            Array.isArray(input.edits) && (
              <div className="divide-y divide-gray-100 dark:divide-gray-800">
                {input.edits.map((rawItem: unknown, index: number) => {
                  const item = getRecord(rawItem);
                  const imageUrls = (getArrayField(item, "image_urls") || []).filter(
                    (url): url is string => typeof url === "string"
                  );
                  const prompt =
                    typeof item?.prompt === "string" ? item.prompt : null;
                  const aspectRatio =
                    typeof item?.aspect_ratio === "string"
                      ? item.aspect_ratio
                      : "与原图一致";
                  return (
                    <div key={`${prompt || "edit"}-${index}`} className="py-3">
                      <div className="flex items-center justify-between gap-2">
                        <div className="text-xs font-medium text-gray-700 dark:text-gray-200">

                          编辑 {index + 1}
                        </div>
                        <div className="rounded-full bg-violet-50 px-2 py-0.5 text-[10px] text-violet-600 dark:bg-violet-900/30 dark:text-violet-300">
                          {aspectRatio}
                        </div>
                      </div>
                      {prompt && (
                        <div className="mt-2">
                          <ExpandablePrompt prompt={prompt} />
                        </div>
                      )}
                      <div className="mt-3 grid grid-cols-3 gap-2">
                        {imageUrls.map((url: string, imageIndex: number) => (
                          <div key={`${url}-${imageIndex}`}>
                            <div className="mb-1 text-[10px] text-gray-500 dark:text-gray-400">
                              {imageIndex === 0
                                ? "主图"
                                : `参考图${imageIndex}`}
                            </div>
                            <img
                              src={url}
                              alt={
                                imageIndex === 0
                                  ? `用于编辑的主图${index + 1}`
                                  : `参考图${imageIndex}用于编辑${index + 1}`
                              }
                              className="aspect-square w-full rounded object-cover bg-gray-50 dark:bg-gray-800"
                              loading="lazy"
                            />
                          </div>
                        ))}
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          {event.status !== "running" && images && (
            <div className="divide-y divide-gray-100 dark:divide-gray-800">
              {images.map((item, index) => {
                const imageUrls = Array.isArray(item?.image_urls)
                  ? item.image_urls
                  : [];
                const prompt =
                  typeof item?.prompt === "string" ? item.prompt : null;
                const succeeded =
                  item?.status === "ok" && typeof item?.result_url === "string";
                return (
                  <div key={`${prompt || "edit"}-${index}`} className="space-y-3 py-3">
                    <div className="flex items-center justify-between gap-2">
                      <div className="text-xs font-medium text-gray-700 dark:text-gray-200">

                        编辑 {index + 1}
                      </div>
                      <div className="flex items-center gap-1.5">
                        <span className="rounded-full bg-gray-100 px-2 py-0.5 text-[10px] text-gray-500 dark:bg-gray-800 dark:text-gray-300">
                          {item?.aspect_ratio || "与原图一致"}
                        </span>
                        <span
                          className={`rounded-full px-2 py-0.5 text-[10px] ${
                            succeeded
                              ? "bg-emerald-50 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-300"
                              : "bg-red-50 text-red-600 dark:bg-red-900/30 dark:text-red-300"
                          }`}
                        >
                          {succeeded ? "完成" : "失败"}
                        </span>
                      </div>
                    </div>
                    {prompt && <ExpandablePrompt prompt={prompt} />}
                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <div className="mb-1 text-xs text-gray-500 dark:text-gray-400">

                          修改前
                        </div>
                        {imageUrls[0] ? (
                          <img
                            src={imageUrls[0]}
                            alt={`用于编辑的主图${index + 1}`}
                            className="aspect-square w-full rounded object-contain bg-gray-50 dark:bg-gray-800"
                            loading="lazy"
                          />
                        ) : (
                          <div className="aspect-square rounded bg-gray-100 dark:bg-gray-800 flex items-center justify-center text-xs text-gray-400">

                            缺失
                          </div>
                        )}
                      </div>
                      <div>
                        <div className="mb-1 text-xs text-gray-500 dark:text-gray-400">

                          修改后
                        </div>
                        {succeeded ? (
                          <img
                            src={item.result_url}
                            alt={`编辑后的图片${index + 1}`}
                            className="aspect-square w-full rounded object-contain bg-gray-50 dark:bg-gray-800"
                            loading="lazy"
                          />
                        ) : (
                          <div className="aspect-square rounded bg-gray-100 dark:bg-gray-800 flex items-center justify-center text-xs text-gray-400">

                            失败
                          </div>
                        )}
                      </div>
                    </div>
                    {imageUrls.length > 1 && (
                      <div>
                        <div className="mb-1 text-xs text-gray-500 dark:text-gray-400">

                          参考图片
                        </div>
                        <div className="grid grid-cols-3 gap-2">
                          {imageUrls
                            .slice(1)
                            .map((url: string, referenceIndex: number) => (
                              <img
                                key={`${url}-${referenceIndex}`}
                                src={url}
                                alt={`参考图${referenceIndex + 1}用于编辑${index + 1}`}
                                className="aspect-square w-full rounded object-cover bg-gray-50 dark:bg-gray-800"
                                loading="lazy"
                              />
                            ))}
                        </div>
                      </div>
                    )}
                    {item?.error && (
                      <div className="rounded bg-red-50 p-2 text-xs text-red-600 dark:bg-red-900/30 dark:text-red-200">
                        {item.error}
                      </div>
                    )}
                    {succeeded && (
                      <div>
                        <div className="text-xs text-gray-500 dark:text-gray-400">

                          结果地址
                        </div>
                        <div className="mt-1 break-all rounded bg-gray-50 p-2 font-mono text-xs text-gray-600 dark:bg-gray-800 dark:text-gray-300">
                          {item.result_url}
                        </div>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}

      {event.toolName === "save_assets" && !hasError && (
        <div className="space-y-3">
          {event.status === "running" && input?.asset_ids && Array.isArray(input.asset_ids) && (
            <div className="divide-y divide-gray-100 dark:divide-gray-800">
              {input.asset_ids.map((assetId: string, index: number) => (
                <div key={`${assetId}-${index}`} className="py-2">
                  <div className="text-xs text-gray-500 dark:text-gray-400 mb-1">

                    素材编号
                  </div>
                  <div className="break-all rounded bg-gray-50 p-2 font-mono text-xs text-gray-600 dark:bg-gray-800 dark:text-gray-300">
                    {assetId}
                  </div>
                </div>
              ))}
            </div>
          )}
          {event.status !== "running" && output?.images && Array.isArray(output.images) && (
            <div className="divide-y divide-gray-100 dark:divide-gray-800">
              {output.images.map((item: any, index: number) => (
                <div key={`${item.asset_id}-${index}`} className="flex gap-3 py-2">
                  <div className="w-1/2">
                    <div className="text-xs text-gray-500 dark:text-gray-400 mb-1">

                      已保存素材
                    </div>
                    {item.public_url ? (
                      <img
                        src={item.public_url}
                        alt={`已保存上传素材${index + 1}`}
                        className="w-full rounded object-cover"
                        loading="lazy"
                      />
                    ) : (
                      <div className="aspect-square rounded bg-gray-100 dark:bg-gray-800 flex items-center justify-center text-xs text-gray-400">

                        失败
                      </div>
                    )}
                  </div>
                  <div className="w-1/2 self-center">
                    <div className="text-xs text-gray-500 dark:text-gray-400">

                      永久地址
                    </div>
                    <div className="mt-1 break-all rounded bg-gray-50 p-2 font-mono text-xs text-gray-600 dark:bg-gray-800 dark:text-gray-300">
                      {item.public_url}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {event.toolName === "extract_assets" &&
        (!hasError || Boolean(visibleExtractedAssets?.length)) && (
          <div>
            {!hasError &&
              event.status === "running" &&
              input?.asset_descriptions &&
              Array.isArray(input.asset_descriptions) && (
                <div className="divide-y divide-gray-100 dark:divide-gray-800">
                  {input.asset_descriptions.map((description: string, index: number) => (
                    <div key={`${description}-${index}`} className="py-2">
                      <div className="text-xs text-gray-500 dark:text-gray-400 mb-1">

                        素材 {index + 1}
                      </div>
                      <div className="text-xs text-gray-600 dark:text-gray-300">
                        {description}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            {event.status !== "running" && visibleExtractedAssets && (
              <div className="divide-y divide-gray-100 dark:divide-gray-800">
                {visibleExtractedAssets.map((asset, index) => {
                  const assetRecord =
                    asset && typeof asset === "object"
                      ? (asset as Record<string, unknown>)
                      : {};
                  const description =
                    typeof assetRecord.description === "string"
                      ? assetRecord.description
                      : `素材${index + 1}`;
                  const publicUrl =
                    typeof assetRecord.public_url === "string"
                      ? assetRecord.public_url
                      : null;
                  const previewUrl =
                    publicUrl ||
                    (typeof assetRecord.data_url === "string"
                      ? assetRecord.data_url
                      : null);
                  const boxText = Array.isArray(assetRecord.box_2d)
                    ? assetRecord.box_2d.join(", ")
                    : "无选区";
                  const statusLabel =
                    typeof assetRecord.status === "string"
                      ? assetRecord.status
                      : previewUrl
                        ? "ok"
                        : "missing";
                  return (
                    <div key={`${description}-${index}`} className="flex gap-3 py-2">
                      <div className="w-1/2 shrink-0">
                        <div className="text-xs text-gray-500 dark:text-gray-400 mb-1">

                          提取的图片
                        </div>
                        {previewUrl ? (
                          <img
                            src={previewUrl}
                            alt={description}
                            className="max-h-48 w-full rounded object-contain bg-gray-50 dark:bg-gray-800"
                            loading="lazy"
                          />
                        ) : (
                          <div className="aspect-square rounded bg-gray-100 dark:bg-gray-800 flex items-center justify-center text-xs text-gray-400">

                            缺失
                          </div>
                        )}
                      </div>
                      <div className="w-1/2 self-center space-y-2">
                        <div>
                          <div className="text-xs text-gray-500 dark:text-gray-400">

                            请求的素材
                          </div>
                          <div className="mt-1 text-xs text-gray-600 dark:text-gray-300">
                            {description}
                          </div>
                        </div>
                        {publicUrl && (
                          <div>
                            <div className="text-xs text-gray-500 dark:text-gray-400">

                              公开地址
                            </div>
                            <div className="mt-1 break-all rounded bg-gray-50 p-2 font-mono text-xs text-gray-600 dark:bg-gray-800 dark:text-gray-300">
                              {publicUrl}
                            </div>
                          </div>
                        )}
                        <div className="grid grid-cols-2 gap-2 text-xs">
                          <div>
                            <div className="text-gray-500 dark:text-gray-400">状态</div>
                            <div className="mt-1 font-mono text-gray-600 dark:text-gray-300">
                              {statusLabel}
                            </div>
                          </div>
                          <div>
                            <div className="text-gray-500 dark:text-gray-400">

                              原始图片
                            </div>
                            <div className="mt-1 font-mono text-gray-600 dark:text-gray-300">
                              {String(assetRecord.image_index ?? "-")}
                            </div>
                          </div>
                        </div>
                        <div>
                          <div className="text-xs text-gray-500 dark:text-gray-400">

                            选区范围
                          </div>
                          <div className="mt-1 break-all rounded bg-gray-50 p-2 font-mono text-xs text-gray-600 dark:bg-gray-800 dark:text-gray-300">
                            [{boxText}]
                          </div>
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}

      {event.toolName === "screenshot_preview" && !hasError && (
        <div>
          {event.status === "running" && (
            <div className="text-xs text-gray-600 dark:text-gray-400 py-1.5">

              正在渲染桌面和移动端预览…
            </div>
          )}
          {event.status !== "running" && (
            <div className="grid gap-3 py-2 sm:grid-cols-2">
              {(["desktop", "mobile"] as const).map((viewport) => {
                const screenshot = screenshotPreviews.find((item) => {
                  const itemRecord = getRecord(item);
                  return itemRecord?.viewport === viewport;
                });
                const screenshotRecord = getRecord(screenshot);
                const imageUrl =
                  typeof screenshotRecord?.image_url === "string"
                    ? screenshotRecord.image_url
                    : null;
                return (
                  <div key={viewport}>
                    <div className="text-xs text-gray-500 dark:text-gray-400 mb-1 capitalize">
                      {viewport === "desktop" ? "桌面端" : "移动端"}
                    </div>
                    {imageUrl ? (
                      <div className="max-h-96 overflow-y-auto rounded border border-gray-200 dark:border-gray-700">
                        <img
                          src={imageUrl}
                          alt={`生成页面截图：${viewport === "desktop" ? "桌面端" : "移动端"}`}
                          className="w-full"
                          loading="lazy"
                        />
                      </div>
                    ) : (
                      <div className="aspect-square rounded bg-gray-100 dark:bg-gray-800 flex items-center justify-center text-xs text-gray-400">

                        缺失
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}

      {!event.toolName && !hasError && (
        <>
          {event.input && (
            <div>
              <div className="text-xs uppercase tracking-wide text-gray-400">

                输入
              </div>
              {renderJson(event.input)}
            </div>
          )}
          {event.output && (
            <div className="mt-3">
              <div className="text-xs uppercase tracking-wide text-gray-400">

                输出
              </div>
              {renderJson(event.output)}
            </div>
          )}
        </>
      )}
    </div>
  );
}

function AgentEventCard({
  event,
  autoExpand,
  variantCode,
}: {
  event: AgentEvent;
  autoExpand?: boolean;
  variantCode?: string;
}) {
  const [expanded, setExpanded] = useState(Boolean(autoExpand));

  useEffect(() => {
    if (autoExpand) {
      setExpanded(true);
    }
  }, [autoExpand]);

  const isExpanded =
    (event.type !== "thinking" && event.status === "running") || expanded;

  if (event.type === "assistant") {
    if (!event.content) return null;
    return (
      <div className="py-1 text-sm text-gray-700 dark:text-gray-300 prose prose-sm dark:prose-invert max-w-none">
        <ReactMarkdown
          components={{
            img: ({ ...props }) => (
              <div className="my-2 flex justify-start max-w-full">
                <img
                  {...props}
                  className="max-h-60 max-w-full object-contain rounded-lg border border-gray-200 dark:border-gray-700"
                  loading="lazy"
                />
              </div>
            ),
          }}
        >
          {event.content}
        </ReactMarkdown>
      </div>
    );
  }

  return (
    <div>
      <button
        onClick={() => setExpanded((prev) => !prev)}
        className="w-full flex items-center gap-2 py-1.5 text-left text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300"
      >
        {getEventIcon(event.type, event.toolName)}
        <span className={`text-sm flex-1 ${event.status === "running" ? "active-step-shimmer" : ""}`}>
          {getEventTitle(event)}
        </span>
        {isExpanded ? (
          <BsChevronDown className="text-xs shrink-0" />
        ) : (
          <BsChevronRight className="text-xs shrink-0" />
        )}
      </button>
      {isExpanded && (
        <div className="pb-2">
          {event.type === "thinking" && event.content && (
            <div className="prose prose-sm dark:prose-invert max-w-none">
              <ReactMarkdown
                components={{
                  img: ({ ...props }) => (
                    <div className="my-2 flex justify-start max-w-full">
                      <img
                        {...props}
                        className="max-h-60 max-w-full object-contain rounded-lg border border-gray-200 dark:border-gray-700"
                        loading="lazy"
                      />
                    </div>
                  ),
                }}
              >
                {event.content}
              </ReactMarkdown>
            </div>
          )}
          {event.type === "tool" && renderToolDetails(event, variantCode)}
        </div>
      )}
    </div>
  );
}

function AgentActivity() {
  const { head, commits, latestCommitHash } = useProjectStore();
  const [expandedStepGroups, setExpandedStepGroups] = useState<
    Record<string, boolean>
  >({});
  const [nowMs, setNowMs] = useState(() => Date.now());
  const appState = useAppStore((s) => s.appState);

  useEffect(() => {
    if (appState !== AppState.CODING) return;
    const intervalId = window.setInterval(() => setNowMs(Date.now()), 1000);
    return () => window.clearInterval(intervalId);
  }, [appState]);

  const currentCommit = head ? commits[head] : null;
  const selectedVariant = currentCommit
    ? currentCommit.variants[currentCommit.selectedVariantIndex]
    : null;
  const selectedVariantStatus = selectedVariant?.status;
  const variantUiKey =
    currentCommit ? `${currentCommit.hash}:${currentCommit.selectedVariantIndex}` : "";

  const variantCode = selectedVariant?.code || "";
  const events = selectedVariant?.agentEvents || [];
  const lastAssistantId = [...events]
    .reverse()
    .find((event) => event.type === "assistant")?.id;
  const requestStartMs =
    selectedVariant?.requestStartedAt ??
    (currentCommit?.dateCreated
      ? new Date(currentCommit.dateCreated).getTime()
      : undefined);

  const isDone = isTerminalVariantStatus(selectedVariantStatus);
  const isLatestCommit = head === latestCommitHash;
  if (!isLatestCommit || events.length === 0) {
    return null;
  }

  const runningDuration = formatElapsedSince(requestStartMs, nowMs);
  const completedEventGroups = groupCompletedAgentEvents(events);

  return (
    <div className="space-y-1 mb-3">
      {isDone ? (
        <>
          {completedEventGroups.map((group) => {
            if (group.type === "assistant") {
              return (
                <AgentEventCard
                  key={group.event.id}
                  event={group.event}
                  autoExpand={group.event.id === lastAssistantId}
                />
              );
            }

            const firstStep = group.events[0];
            const lastStep = group.events[group.events.length - 1];
            const groupUiKey = `${variantUiKey}:${firstStep.id}`;
            const isExpanded = Boolean(expandedStepGroups[groupUiKey]);
            const groupDuration = formatDuration(
              firstStep.startedAt,
              lastStep.endedAt
            );

            return (
              <div key={groupUiKey}>
                <button
                  onClick={() =>
                    setExpandedStepGroups((previous) => ({
                      ...previous,
                      [groupUiKey]: !previous[groupUiKey],
                    }))
                  }
                  className="w-full flex items-center gap-2 rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/60 px-3 py-2 text-left"
                >
                  {isExpanded ? (
                    <BsChevronDown className="text-gray-400 text-xs" />
                  ) : (
                    <BsChevronRight className="text-gray-400 text-xs" />
                  )}
                  <span className="text-xs text-gray-500 dark:text-gray-400">

                    已完成 {group.events.length}  步骤
                    {group.events.length !== 1 ? "s" : ""}
                    {groupDuration ? ` in ${groupDuration}` : ""}
                  </span>
                </button>
                {isExpanded && (
                  <div className="space-y-1">
                    {group.events.map((event) => (
                      <AgentEventCard
                        key={event.id}
                        event={event}
                        variantCode={
                          event.toolName === "create_file"
                            ? variantCode
                            : undefined
                        }
                      />
                    ))}
                  </div>
                )}
              </div>
            );
          })}
        </>
      ) : (
        <>
          <div className="flex items-center justify-between rounded-xl border border-violet-200 dark:border-violet-800 bg-gradient-to-r from-violet-50 to-white dark:from-violet-900/20 dark:to-zinc-900 px-3 py-2 shadow-[0_0_15px_-3px_rgba(139,92,246,0.3)] dark:shadow-[0_0_15px_-3px_rgba(139,92,246,0.4)] transition-all duration-500">
            <div className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
              <WorkingPulse />
              <span>正在处理…</span>
            </div>
            <div className="text-xs font-semibold text-gray-700 dark:text-gray-200">

              已用时 {runningDuration || "--"}
            </div>
          </div>
          {events.map((event) => (
            <AgentEventCard
              key={event.id}
              event={event}
              autoExpand={event.type === "assistant" && event.id === lastAssistantId}
              variantCode={event.toolName === "create_file" ? variantCode : undefined}
            />
          ))}
        </>
      )}
    </div>
  );
}

export default AgentActivity;
