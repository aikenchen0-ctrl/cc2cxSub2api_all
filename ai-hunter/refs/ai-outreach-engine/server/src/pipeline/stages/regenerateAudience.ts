import { completeJson } from "../../llm/client";
import {
  REGENERATE_AUDIENCE_SYSTEM,
  REGENERATE_JOB_AUDIENCE_SYSTEM,
  regenerateAudienceUser,
} from "../../llm/prompts";
import { normalizeSiteAnalysis } from "../../db/analysis";
import { audiences, jobs, projects, siteProfiles } from "../../db/repo";
import type { AudienceInput } from "../../db/repo";
import {
  DEFAULT_FUNDED_AFTER_YEAR,
  DEFAULT_STARTUP_SIZES,
} from "../../providers/companyFilters";
import { DEFAULT_SOURCE_ADAPTERS } from "../../providers/sources/catalog";
import { preferencesForPrompt, preferencesFromProject } from "../../workflow/preferences";
import {
  jobBriefPromptBlock,
  jobSearchBriefFromProject,
  normalizeCompanySizes,
} from "../../workflow/jobBrief";

type RegeneratedAudience = AudienceInput & { fundedAfterYear?: number };

/**
 * Refresh a single ICP / job-search lane in place, then queue a focused email rewrite.
 * Does not touch other audiences or delete prospecting data.
 */
export async function regenerateAudience(
  projectId: number,
  payload: { audienceId: number; rewriteEmail?: boolean },
): Promise<string> {
  const project = projects.get(projectId);
  if (!project) throw new Error(`Project ${projectId} not found`);
  const audience = audiences.get(payload.audienceId);
  if (!audience || audience.projectId !== projectId) {
    throw new Error(`Audience ${payload.audienceId} not found`);
  }
  const profile = siteProfiles.latest(projectId);
  if (!profile?.analysis) {
    throw new Error("No profile analysis yet. Run setup first.");
  }

  const analysis = normalizeSiteAnalysis(JSON.parse(profile.analysis));
  const preferences = preferencesForPrompt(preferencesFromProject(project));
  const jobBrief = jobSearchBriefFromProject(project);
  const isCustomer = project.workflow_type === "customer_outreach";
  const isJob = project.workflow_type === "job_outreach";

  const result = await completeJson<RegeneratedAudience>({
    system: isJob ? REGENERATE_JOB_AUDIENCE_SYSTEM : REGENERATE_AUDIENCE_SYSTEM,
    user: regenerateAudienceUser({
      analysis,
      audience: {
        name: audience.name,
        description: audience.description,
        targetRoles: audience.targetRoles,
        jobTitles: audience.jobTitles,
        industries: audience.industries,
        companySizes: audience.companySizes,
        fundedAfterYear: audience.fundedAfterYear,
        searchQueries: audience.searchQueries,
        painPoints: audience.painPoints,
        valueProp: audience.valueProp,
      },
      preferences,
      workflowType: project.workflow_type,
      jobBriefBlock: jobBrief ? jobBriefPromptBlock(jobBrief) : undefined,
    }),
    tier: "reasoning",
    projectId,
    label: "regenerate_audience",
  });

  if (!result.name?.trim()) {
    throw new Error(`Model returned an empty audience for “${audience.name}”.`);
  }

  const modelSizes = normalizeCompanySizes(result.companySizes ?? []);
  const sizes = jobBrief?.companySizes.length
    ? jobBrief.companySizes
    : modelSizes.length
      ? modelSizes
      : (result.companySizes ?? []).filter(Boolean);
  let industries = result.industries ?? [];
  if (isJob && jobBrief?.preferAi) {
    if (!industries.some((item) => /\bai\b/i.test(item))) {
      industries = ["AI", ...industries.filter((i) => !/healthtech|fintech|blockchain|martech/i.test(i))].slice(0, 3);
    }
  }
  audiences.update(audience.id, {
    name: result.name.trim(),
    description: result.description ?? "",
    targetRoles: isJob ? result.targetRoles ?? audience.targetRoles : [],
    jobTitles: result.jobTitles ?? [],
    industries,
    companySizes: sizes.length
      ? sizes
      : isCustomer || isJob
        ? isJob
          ? ["1-10", "11-50"]
          : [...DEFAULT_STARTUP_SIZES]
        : audience.companySizes,
    fundedAfterYear:
      isJob && jobBrief?.fundedAfterYear
        ? jobBrief.fundedAfterYear
        : result.fundedAfterYear && result.fundedAfterYear > 0
          ? result.fundedAfterYear
          : isCustomer || isJob
            ? audience.fundedAfterYear || DEFAULT_FUNDED_AFTER_YEAR
            : 0,
    searchQueries: result.searchQueries ?? [],
    painPoints: result.painPoints ?? [],
    valueProp: result.valueProp ?? "",
    sourceAdapters:
      audience.sourceAdapters.length > 0
        ? audience.sourceAdapters
        : isCustomer
          ? [...DEFAULT_SOURCE_ADAPTERS]
          : isJob
            ? ["web"]
            : [],
  });

  const rewriteEmail = payload.rewriteEmail !== false;
  if (rewriteEmail) {
    jobs.requeue(projectId, "generate_audience_email", { audienceId: audience.id });
  }

  return rewriteEmail
    ? `Refreshed “${result.name.trim()}”. Email rewrite queued.`
    : `Refreshed “${result.name.trim()}”.`;
}
