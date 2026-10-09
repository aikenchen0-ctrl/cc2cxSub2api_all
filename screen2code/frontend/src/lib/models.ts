// Keep in sync with backend (llm.py)
// Order here matches dropdown order
export enum CodeGenerationModel {
  CLAUDE_OPUS_5_LOW = "claude-opus-5 (low effort)",
  CLAUDE_OPUS_5_MEDIUM = "claude-opus-5 (medium effort)",
  CLAUDE_OPUS_5_HIGH = "claude-opus-5 (high effort)",
  CLAUDE_OPUS_5_XHIGH = "claude-opus-5 (xhigh effort)",
  CLAUDE_OPUS_5_MAX = "claude-opus-5 (max effort)",
  CLAUDE_OPUS_4_8_LOW = "claude-opus-4-8 (low effort)",
  CLAUDE_OPUS_4_8_MEDIUM = "claude-opus-4-8 (medium effort)",
  CLAUDE_OPUS_4_8_HIGH = "claude-opus-4-8 (high effort)",
  CLAUDE_OPUS_4_8_XHIGH = "claude-opus-4-8 (xhigh effort)",
  CLAUDE_OPUS_4_8_MAX = "claude-opus-4-8 (max effort)",
  CLAUDE_FABLE_5_MAX = "claude-fable-5 (max effort)",
  CLAUDE_SONNET_4_6 = "claude-sonnet-4-6",
  GPT_5_5_NONE = "gpt-5.5 (no thinking)",
  GPT_5_5_LOW = "gpt-5.5 (low thinking)",
  GPT_5_5_MEDIUM = "gpt-5.5 (medium thinking)",
  GPT_5_5_HIGH = "gpt-5.5 (high thinking)",
  GPT_6_ASTRA_LOW = "gpt-6-astra (low thinking)",
  GPT_6_ASTRA_HIGH = "gpt-6-astra (high thinking)",
  GPT_5_6_SOL_NONE = "gpt-5.6-sol (no thinking)",
  GPT_5_6_SOL_LOW = "gpt-5.6-sol (low thinking)",
  GPT_5_6_SOL_MEDIUM = "gpt-5.6-sol (medium thinking)",
  GPT_5_6_SOL_HIGH = "gpt-5.6-sol (high thinking)",
  GPT_5_6_SOL_XHIGH = "gpt-5.6-sol (xhigh thinking)",
  GPT_5_6_SOL_MAX = "gpt-5.6-sol (max thinking)",
  GPT_5_6_TERRA_LOW = "gpt-5.6-terra (low thinking)",
  GPT_5_5_XHIGH = "gpt-5.5 (xhigh thinking)",
  GPT_5_4_MINI_LOW = "gpt-5.4-mini (low thinking)",
  GEMINI_3_FLASH_PREVIEW_HIGH = "gemini-3-flash-preview (high thinking)",
  GEMINI_3_FLASH_PREVIEW_MINIMAL = "gemini-3-flash-preview (minimal thinking)",
  GEMINI_3_1_PRO_PREVIEW_HIGH = "gemini-3.1-pro-preview (high thinking)",
  GEMINI_3_1_PRO_PREVIEW_MEDIUM = "gemini-3.1-pro-preview (medium thinking)",
  GEMINI_3_1_PRO_PREVIEW_LOW = "gemini-3.1-pro-preview (low thinking)",
  GEMINI_3_5_FLASH_HIGH = "gemini-3.5-flash (high thinking)",
  GEMINI_3_5_FLASH_MEDIUM = "gemini-3.5-flash (medium thinking)",
  GEMINI_3_5_FLASH_LOW = "gemini-3.5-flash (low thinking)",
  GEMINI_3_5_FLASH_MINIMAL = "gemini-3.5-flash (minimal thinking)",
  GEMINI_3_6_FLASH_HIGH = "gemini-3.6-flash (high thinking)",
  GEMINI_3_6_FLASH_MEDIUM = "gemini-3.6-flash (medium thinking)",
  GEMINI_3_6_FLASH_LOW = "gemini-3.6-flash (low thinking)",
  GEMINI_3_6_FLASH_MINIMAL = "gemini-3.6-flash (minimal thinking)",
}

export type VariantLabelTone = "fast" | "max";

export interface VariantLabel {
  text: string;
  tone: VariantLabelTone;
}

export interface VariantLabelContext {
  inputMode: "image" | "video" | "text";
  generationType: "create" | "update";
}

// Per-model badge text. Only these models are labelled. Heavyweight
// variants read "Max" (sol max anchors image create; 3.1 Pro high anchors
// video); Flash-minimal is the only variant fast enough to earn "Fast".
const VARIANT_LABELS: Partial<Record<CodeGenerationModel, VariantLabel>> = {
  [CodeGenerationModel.GEMINI_3_FLASH_PREVIEW_MINIMAL]: { text: "快速", tone: "fast" },
  [CodeGenerationModel.GEMINI_3_1_PRO_PREVIEW_HIGH]: { text: "高质量", tone: "max" },
  [CodeGenerationModel.GPT_5_5_HIGH]: { text: "高质量", tone: "max" },
  [CodeGenerationModel.GPT_6_ASTRA_HIGH]: { text: "高质量", tone: "max" },
  [CodeGenerationModel.GPT_5_6_SOL_MAX]: { text: "高质量", tone: "max" },
};

// Badges are only shown on create flows and on any video flow. In particular
// image/text update runs reuse Flash-minimal but should stay unlabelled.
export function getVariantLabel(
  model: string | undefined,
  context: VariantLabelContext
): VariantLabel | null {
  if (!model) return null;
  const showLabels =
    context.generationType === "create" || context.inputMode === "video";
  if (!showLabels) return null;
  return VARIANT_LABELS[model as CodeGenerationModel] ?? null;
}

// Will generate a static error if a model in the enum above is not in the descriptions
export const CODE_GENERATION_MODEL_DESCRIPTIONS: {
  [key in CodeGenerationModel]: { name: string };
} = {
  "gpt-5.6-sol (no thinking)": {
    name: "GPT 5.6 Sol (无推理)",
  },
  "gpt-5.6-sol (low thinking)": {
    name: "GPT 5.6 Sol (低推理)",
  },
  "gpt-5.6-sol (medium thinking)": {
    name: "GPT 5.6 Sol (中推理)",
  },
  "gpt-5.6-sol (high thinking)": {
    name: "GPT 5.6 Sol (高推理)",
  },
  "gpt-5.6-sol (xhigh thinking)": {
    name: "GPT 5.6 Sol (超高推理)",
  },
  "gpt-5.6-sol (max thinking)": {
    name: "GPT 5.6 Sol (最高推理)",
  },
  "gpt-5.6-terra (low thinking)": {
    name: "GPT 5.6 Terra (低推理)",
  },
  "gpt-5.5 (no thinking)": {
    name: "GPT 5.5 (无推理)",
  },
  "gpt-5.5 (low thinking)": {
    name: "GPT 5.5 (低推理)",
  },
  "gpt-5.5 (medium thinking)": {
    name: "GPT 5.5 (中推理)",
  },
  "gpt-5.5 (high thinking)": {
    name: "GPT 5.5 (高推理)",
  },
  "gpt-6-astra (low thinking)": {
    name: "GPT 6 Astra (低推理)",
  },
  "gpt-6-astra (high thinking)": {
    name: "GPT 6 Astra (高推理)",
  },
  "gpt-5.5 (xhigh thinking)": {
    name: "GPT 5.5 (超高推理)",
  },
  "gpt-5.4-mini (low thinking)": {
    name: "GPT 5.4 Mini (低推理)",
  },
  "claude-opus-5 (low effort)": {
    name: "Claude Opus 5 (低推理)",
  },
  "claude-opus-5 (medium effort)": {
    name: "Claude Opus 5 (中推理)",
  },
  "claude-opus-5 (high effort)": {
    name: "Claude Opus 5 (高推理)",
  },
  "claude-opus-5 (xhigh effort)": {
    name: "Claude Opus 5 (超高推理)",
  },
  "claude-opus-5 (max effort)": {
    name: "Claude Opus 5 (最高推理)",
  },
  "claude-opus-4-8 (low effort)": {
    name: "Claude Opus 4.8 (低推理)",
  },
  "claude-opus-4-8 (medium effort)": {
    name: "Claude Opus 4.8 (中推理)",
  },
  "claude-opus-4-8 (high effort)": {
    name: "Claude Opus 4.8 (高推理)",
  },
  "claude-opus-4-8 (xhigh effort)": {
    name: "Claude Opus 4.8 (超高推理)",
  },
  "claude-opus-4-8 (max effort)": {
    name: "Claude Opus 4.8 (最高推理)",
  },
  "claude-fable-5 (max effort)": {
    name: "Claude Fable 5 (最高推理)",
  },
  "claude-sonnet-4-6": { name: "Claude Sonnet 4.6" },
  "gemini-3.5-flash (high thinking)": {
    name: "Gemini 3.5 Flash (高推理)",
  },
  "gemini-3.5-flash (medium thinking)": {
    name: "Gemini 3.5 Flash (中推理)",
  },
  "gemini-3.5-flash (low thinking)": {
    name: "Gemini 3.5 Flash (低推理)",
  },
  "gemini-3.5-flash (minimal thinking)": {
    name: "Gemini 3.5 Flash (极低推理)",
  },
  "gemini-3.6-flash (high thinking)": {
    name: "Gemini 3.6 Flash (高推理)",
  },
  "gemini-3.6-flash (medium thinking)": {
    name: "Gemini 3.6 Flash (中推理)",
  },
  "gemini-3.6-flash (low thinking)": {
    name: "Gemini 3.6 Flash (低推理)",
  },
  "gemini-3.6-flash (minimal thinking)": {
    name: "Gemini 3.6 Flash (极低推理)",
  },
  "gemini-3-flash-preview (high thinking)": {
    name: "Gemini 3 Flash (高推理)",
  },
  "gemini-3-flash-preview (minimal thinking)": {
    name: "Gemini 3 Flash (极低推理)",
  },
  "gemini-3.1-pro-preview (high thinking)": {
    name: "Gemini 3.1 Pro (高推理)",
  },
  "gemini-3.1-pro-preview (medium thinking)": {
    name: "Gemini 3.1 Pro (中推理)",
  },
  "gemini-3.1-pro-preview (low thinking)": {
    name: "Gemini 3.1 Pro (低推理)",
  },
};
