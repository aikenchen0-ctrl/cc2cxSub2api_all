import { env } from "../env";
import { events } from "../db/repo";

const OPENAI_URL = "https://api.openai.com/v1/chat/completions";
const LLM_TIMEOUT_MS = 180_000;

/**
 * Published rates per million tokens, used only to show a running spend
 * estimate in the UI. Update when provider pricing changes.
 */
const RATES: Record<string, { input: number; output: number }> = {
  "gpt-5-mini": { input: 0.25, output: 2.0 },
  "gpt-5.4-nano": { input: 0.05, output: 0.4 },
  "gpt-5": { input: 1.25, output: 10.0 },
};

function estimateCost(model: string, inputTokens: number, outputTokens: number): number {
  const rate = RATES[model];
  if (!rate) return 0;
  return (inputTokens / 1e6) * rate.input + (outputTokens / 1e6) * rate.output;
}

/**
 * GPT-5 family models take reasoning_effort instead of temperature.
 * Strategic GTM tasks (site analysis, ICP generation) get medium effort;
 * extraction / drafting stay low for latency + cost.
 */
function reasoningEffort(model: string, label?: string): string | null {
  if (!model.startsWith("gpt-5")) return null;
  if (model.includes("nano")) return "low";
  const strategic = new Set([
    "analyse_site",
    "analyse_resume",
    "generate_audiences",
    "generate_audience_email",
    "regenerate_audience",
    "qualify_companies",
  ]);
  if (label && strategic.has(label)) return "medium";
  return "low";
}

export type LlmTier = "reasoning" | "cheap";

function modelFor(tier: LlmTier): string {
  return tier === "cheap" ? env.llm.cheapModel : env.llm.reasoningModel;
}

/** Pull the first JSON object or array out of a model response. */
function parseJson<T>(raw: string): T {
  const trimmed = raw.trim().replace(/^```(?:json)?\s*/i, "").replace(/```$/, "");
  try {
    return JSON.parse(trimmed) as T;
  } catch {
    const match = trimmed.match(/[[{][\s\S]*[\]}]/);
    if (!match) throw new Error(`Model did not return JSON: ${trimmed.slice(0, 200)}`);
    return JSON.parse(match[0]) as T;
  }
}

async function callOpenAi(args: {
  model: string;
  system: string;
  user: string;
  json: boolean;
  label?: string;
}): Promise<{ text: string; inputTokens: number; outputTokens: number }> {
  const body: Record<string, unknown> = {
    model: args.model,
    messages: [
      { role: "developer", content: args.system },
      { role: "user", content: args.user },
    ],
    max_completion_tokens: 8192,
  };
  const effort = reasoningEffort(args.model, args.label);
  if (effort) body.reasoning_effort = effort;
  if (args.json) body.response_format = { type: "json_object" };

  const res = await fetch(OPENAI_URL, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${env.llm.openaiKey}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
    signal: AbortSignal.timeout(LLM_TIMEOUT_MS),
  }).catch((error) => {
    if (error instanceof Error && (error.name === "TimeoutError" || error.name === "AbortError")) {
      throw new Error(`OpenAI timed out after ${LLM_TIMEOUT_MS / 1000}s`);
    }
    throw error;
  });
  if (!res.ok) throw new Error(`OpenAI error ${res.status}: ${(await res.text()).slice(0, 300)}`);

  const payload = (await res.json()) as {
    choices?: Array<{ message?: { content?: string } }>;
    usage?: { prompt_tokens?: number; completion_tokens?: number };
  };
  return {
    text: payload.choices?.[0]?.message?.content ?? "",
    inputTokens: payload.usage?.prompt_tokens ?? 0,
    outputTokens: payload.usage?.completion_tokens ?? 0,
  };
}

export function llmAvailable(): boolean {
  return Boolean(env.llm.openaiKey);
}

export async function complete(args: {
  system: string;
  user: string;
  tier?: LlmTier;
  json?: boolean;
  projectId?: number;
  label?: string;
}): Promise<string> {
  if (!llmAvailable()) {
    throw new Error("No LLM key configured. Set OPENAI_API_KEY.");
  }
  const model = modelFor(args.tier ?? "reasoning");
  const json = args.json ?? false;
  const result = await callOpenAi({
    model,
    system: args.system,
    user: args.user,
    json,
    label: args.label,
  });

  events.log("llm_call", {
    projectId: args.projectId,
    ref: args.label ?? "",
    data: {
      model,
      inputTokens: result.inputTokens,
      outputTokens: result.outputTokens,
    },
    costUsd: estimateCost(model, result.inputTokens, result.outputTokens),
  });

  return result.text;
}

export async function completeJson<T>(args: {
  system: string;
  user: string;
  tier?: LlmTier;
  projectId?: number;
  label?: string;
}): Promise<T> {
  const raw = await complete({ ...args, json: true });
  return parseJson<T>(raw);
}

export function activeModel(tier: LlmTier = "reasoning"): string {
  return modelFor(tier);
}
