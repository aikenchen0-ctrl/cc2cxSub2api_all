import type { Overview, PipelineJob } from "./api";

export const SETUP_STAGES = new Set([
  "scan_site",
  "generate_audiences",
  "generate_audience_email",
  "regenerate_audience",
]);
export const PROSPECT_STAGES = new Set([
  "find_companies",
  "find_contacts",
  "verify_contact",
  "draft_message",
]);
export const SEND_STAGES = new Set(["send_message"]);

export function activeJobs(
  overview: Overview | null | undefined,
  stages?: Set<string>,
): PipelineJob[] {
  const active = overview?.pipeline?.active ?? [];
  if (!stages) return active;
  return active.filter((job) => stages.has(job.stage));
}

export function isPipelineBusy(
  overview: Overview | null | undefined,
  stages?: Set<string>,
): boolean {
  return activeJobs(overview, stages).length > 0;
}

export function cancelScopeForJobs(jobs: PipelineJob[]): "setup" | "prospecting" | "send" | "all" {
  if (jobs.length === 0) return "all";
  const onlySetup = jobs.every((job) => SETUP_STAGES.has(job.stage));
  if (onlySetup) return "setup";
  const onlyProspect = jobs.every((job) => PROSPECT_STAGES.has(job.stage));
  if (onlyProspect) return "prospecting";
  const onlySend = jobs.every((job) => SEND_STAGES.has(job.stage));
  if (onlySend) return "send";
  return "all";
}

export function summariseActiveJobs(jobs: PipelineJob[]): {
  headline: string;
  detail: string;
  running: PipelineJob | undefined;
} {
  const running = jobs.find((job) => job.status === "running");
  const pending = jobs.filter((job) => job.status === "pending").length;
  const headline =
    jobs.length === 1
      ? running
        ? `Running · ${running.label}`
        : `Queued · ${jobs[0]!.label}`
      : `${jobs.length} steps in progress`;

  const detail = running
    ? `${running.target}${pending > 0 ? ` · ${pending} queued` : ""}`
    : jobs[0]
      ? `${jobs[0].label} · ${jobs[0].target}`
      : "Working…";

  return { headline, detail, running };
}
