import { CODE_GENERATION_MODEL_DESCRIPTIONS, CodeGenerationModel } from "../../lib/models";
import { STACK_DESCRIPTIONS, Stack } from "../../lib/stacks";

const PROVIDER_BADGE_CLASSES: Record<string, string> = {
  openai: "bg-emerald-900/60 text-emerald-200 border-emerald-700",
  anthropic: "bg-orange-900/60 text-orange-200 border-orange-700",
  gemini: "bg-sky-900/60 text-sky-200 border-sky-700",
};

export function providerBadgeClass(provider: string): string {
  return (
    PROVIDER_BADGE_CLASSES[provider] ||
    "bg-zinc-800 text-zinc-200 border-zinc-600"
  );
}

export function statusLabel(status: string): string {
  switch (status) {
    case "completed": return "已完成";
    case "failed": return "失败";
    case "running": return "运行中";
    case "pending": return "等待中";
    case "cancelled": return "已取消";
    case "error": return "错误";
    default: return "未知状态";
  }
}

export function toolLabel(name: string): string {
  switch (name) {
    case "create_file": return "创建文件";
    case "edit_file": return "编辑文件";
    case "generate_images": return "生成图片";
    case "remove_backgrounds": return "移除背景";
    case "edit_images": return "编辑图片";
    case "retrieve_option": return "读取方案";
    case "save_assets": return "保存素材";
    case "extract_assets": return "提取素材";
    case "screenshot_preview": return "预览截图";
    default: return "其他工具";
  }
}

export function entryPointLabel(entryPoint: string): string {
  switch (entryPoint) {
    case "websocket": return "网页生成";
    case "create": return "创建";
    case "update": return "修改";
    case "generate": return "生成";
    case "generate_code":
    case "generate-code": return "生成代码";
    case "eval":
    case "evals": return "评测";
    case "run_evals": return "运行评测";
    case "image": return "图片";
    case "video": return "视频";
    case "text": return "文本";
    default: return "其他任务";
  }
}

export function providerLabel(provider: string): string {
  switch (provider) {
    case "openai": return "OpenAI";
    case "anthropic": return "Anthropic";
    case "gemini": return "Gemini";
    default: return "其他模型服务";
  }
}

export function modelLabel(model: string | null): string {
  if (!model) return "未知模型";
  return CODE_GENERATION_MODEL_DESCRIPTIONS[model as CodeGenerationModel]?.name ?? model;
}

export function stackLabel(stack: string | null): string {
  return STACK_DESCRIPTIONS[stack as Stack]?.components.join(" + ") ?? "其他技术栈";
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} 字节`;
  const units = ["千字节", "兆字节", "吉字节", "太字节"];
  let value = bytes;
  let unitIndex = -1;
  do {
    value /= 1024;
    unitIndex += 1;
  } while (value >= 1024 && unitIndex < units.length - 1);
  return `${value.toFixed(value >= 100 ? 0 : 1)} ${units[unitIndex]}`;
}

export function formatTimestamp(isoTimestamp: string): string {
  const date = new Date(isoTimestamp);
  if (isNaN(date.getTime())) return isoTimestamp;
  return date.toLocaleString("zh-CN");
}

export function formatCost(costUsd: number | null): string {
  if (costUsd === null) return "—";
  // Round to cents; flag sub-cent totals so a real cost never reads as $0.00.
  if (costUsd > 0 && costUsd < 0.01) return "<$0.01";
  return `$${costUsd.toFixed(2)}`;
}

// formatDurationMs (generation-time.ts) floors at 1s; run timelines need
// millisecond precision for fast tool calls.
export function formatMs(milliseconds: number | null | undefined): string {
  if (milliseconds === null || milliseconds === undefined) return "—";
  if (milliseconds < 1000) return `${Math.round(milliseconds)} 毫秒`;
  if (milliseconds < 60_000) return `${(milliseconds / 1000).toFixed(1)} 秒`;
  const minutes = Math.floor(milliseconds / 60_000);
  const seconds = Math.round((milliseconds % 60_000) / 1000);
  return `${minutes} 分 ${seconds} 秒`;
}

