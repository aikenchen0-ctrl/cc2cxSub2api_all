import { completeJson } from "../../llm/client";
import {
  GENERATE_AUDIENCES_SYSTEM,
  GENERATE_INVESTOR_AUDIENCES_SYSTEM,
  GENERATE_JOB_AUDIENCES_SYSTEM,
  generateAudiencesUser,
  generateJobAudiencesUser,
  generateWorkflowAudiencesUser,
} from "../../llm/prompts";
import { normalizeSiteAnalysis } from "../../db/analysis";
import { audiences, jobs, projects, siteProfiles } from "../../db/repo";
import type { AudienceInput } from "../../db/repo";
import {
  preferencesForPrompt,
  preferencesFromProject,
} from "../../workflow/preferences";
import {
  jobBriefPromptBlock,
  jobSearchBriefFromProject,
  normalizeCompanySizes,
} from "../../workflow/jobBrief";
import {
  DEFAULT_FUNDED_AFTER_YEAR,
  DEFAULT_STARTUP_SIZES,
} from "../../providers/companyFilters";
import { DEFAULT_SOURCE_ADAPTERS } from "../../providers/sources/catalog";

type GeneratedAudiences = { audiences: AudienceInput[] };

const DEFAULT_JOB_CONTACT_TITLES = [
  "Hiring Manager",
  "Engineering Manager",
  "Head of Engineering",
  "CTO",
  "Recruiter",
  "Head of Talent",
  "Founder",
];

/**
 * Server-side defaults when the model omits filters.
 * - customer: startup sizes + portfolio sources + fundedAfterYear
 * - jobs: startup sizes + web-only sources (no YC adapters — they don't run for jobs)
 * Email templates stay empty — each audience gets a dedicated email job.
 */
function normalizeAudience(
  candidate: AudienceInput,
  workflowType: string,
  jobBrief?: ReturnType<typeof jobSearchBriefFromProject>,
): AudienceInput {
  const isCustomer = workflowType === "customer_outreach";
  const isJob = workflowType === "job_outreach";
  const modelSizes = normalizeCompanySizes(candidate.companySizes ?? []);
  const briefSizes = jobBrief?.companySizes ?? [];
  const sizes = briefSizes.length
    ? briefSizes
    : modelSizes.length
      ? modelSizes
      : (candidate.companySizes ?? []).filter(Boolean);
  const contactTitles = (candidate.jobTitles ?? []).filter(Boolean);
  let targetRoles = (candidate.targetRoles ?? []).filter(Boolean);
  // Keep lane roles inside the user's stated list when we have one.
  if (isJob && jobBrief?.targetRoles.length) {
    const allowed = jobBrief.targetRoles.map((role) => role.toLowerCase());
    const filtered = targetRoles.filter((role) =>
      allowed.some(
        (want) =>
          role.toLowerCase().includes(want) ||
          want.includes(role.toLowerCase()) ||
          // loose synonym: frontend / full stack / founding / gtm / product / architect
          want.split(/\s+/).filter((w) => w.length > 3).some((w) => role.toLowerCase().includes(w)),
      ),
    );
    if (filtered.length > 0) targetRoles = filtered;
    else targetRoles = jobBrief.targetRoles.slice(0, 5);
  }
  let industries = (candidate.industries ?? []).filter(Boolean);
  if (isJob && jobBrief?.preferAi) {
    const aiFirst = industries.filter((item) =>
      /\bai\b|llm|machine learning|artificial/i.test(item),
    );
    // Drop random vertical padding when user asked for AI.
    const padded = industries.filter((item) =>
      /healthtech|fintech|blockchain|martech|edtech|crypto/i.test(item),
    );
    if (aiFirst.length === 0) industries = ["AI", ...industries.filter((i) => !padded.includes(i))].slice(0, 3);
    else industries = [...aiFirst, ...industries.filter((i) => !aiFirst.includes(i) && !padded.includes(i))].slice(0, 3);
  } else if (isJob && jobBrief?.industries.length) {
    industries = jobBrief.industries;
  }

  const fundedAfterYear =
    isJob && jobBrief?.fundedAfterYear
      ? jobBrief.fundedAfterYear
      : candidate.fundedAfterYear && candidate.fundedAfterYear > 0
        ? candidate.fundedAfterYear
        : isCustomer
          ? DEFAULT_FUNDED_AFTER_YEAR
          : isJob
            ? DEFAULT_FUNDED_AFTER_YEAR
            : 0;

  return {
    ...candidate,
    templateBody: "",
    templateSubject: "",
    targetRoles: isJob ? targetRoles : [],
    industries: isJob ? industries : candidate.industries,
    jobTitles:
      contactTitles.length > 0
        ? contactTitles
        : isJob
          ? [...DEFAULT_JOB_CONTACT_TITLES]
          : contactTitles,
    companySizes: sizes.length
      ? sizes
      : isCustomer || isJob
        ? isJob
          ? ["1-10", "11-50"]
          : [...DEFAULT_STARTUP_SIZES]
        : [],
    sourceAdapters:
      (candidate.sourceAdapters ?? []).length > 0
        ? candidate.sourceAdapters
        : isCustomer
          ? [...DEFAULT_SOURCE_ADAPTERS]
          : isJob
            ? ["web"]
            : [],
    fundedAfterYear,
  };
}

export type GenerateAudiencesPayload = {
  /** When true (default), keep prior ICPs that already have prospecting data. */
  preserveProspecting?: boolean;
};

export async function generateAudiences(
  projectId: number,
  payload: GenerateAudiencesPayload = {},
): Promise<string> {
  const project = projects.get(projectId);
  if (!project) throw new Error(`Project ${projectId} not found`);
  const profile = siteProfiles.latest(projectId);
  if (!profile?.analysis) {
    throw new Error("No profile analysis yet. Run setup first.");
  }

  const analysis = normalizeSiteAnalysis(JSON.parse(profile.analysis));
  const preferences = preferencesForPrompt(preferencesFromProject(project));
  const jobBrief = jobSearchBriefFromProject(project);
  const jobBriefBlock = jobBrief ? jobBriefPromptBlock(jobBrief) : undefined;
  const workflowData = (() => {
    try {
      return JSON.parse(project.workflow_data);
    } catch {
      return {};
    }
  })();
  const system =
    project.workflow_type === "investor_outreach"
      ? GENERATE_INVESTOR_AUDIENCES_SYSTEM
      : project.workflow_type === "job_outreach"
        ? GENERATE_JOB_AUDIENCES_SYSTEM
        : GENERATE_AUDIENCES_SYSTEM;
  const user =
    project.workflow_type === "customer_outreach"
      ? generateAudiencesUser(analysis, preferences)
      : project.workflow_type === "job_outreach"
        ? generateJobAudiencesUser({ analysis, workflowData, preferences, jobBriefBlock })
        : generateWorkflowAudiencesUser({ analysis, workflowData, preferences });

  const result = await completeJson<GeneratedAudiences>({
    system,
    user,
    tier: "reasoning",
    projectId,
    label: "generate_audiences",
  });

  const preserveProspecting = payload.preserveProspecting !== false;
  audiences.replaceGenerated(projectId, { preserveProspecting });
  const createdIds: number[] = [];
  for (const candidate of result.audiences ?? []) {
    if (!candidate.name) continue;
    const created = audiences.create(projectId, {
      ...normalizeAudience(candidate, project.workflow_type, jobBrief),
      origin: "generated",
    });
    createdIds.push(created.id);
    // One focused LLM call per audience — much better copy than batch templates.
    jobs.enqueue(projectId, "generate_audience_email", { audienceId: created.id });
  }

  const kept = audiences.list(projectId).filter((row) => row.origin === "retained").length;
  const created = createdIds.length;
  const unit =
    project.workflow_type === "job_outreach" ? "job-search lane" : "target audience";
  if (preserveProspecting && kept > 0) {
    return `Generated ${created} ${unit}${created === 1 ? "" : "s"} (email drafts queued). Kept ${kept} previous audience(s) with contacts/messages.`;
  }
  return `Generated ${created} ${unit}${created === 1 ? "" : "s"}. Email templates queued individually.`;
}
