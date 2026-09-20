import { completeJson } from "../../llm/client";
import {
  GENERATE_AUDIENCE_EMAIL_SYSTEM,
  GENERATE_JOB_AUDIENCE_EMAIL_SYSTEM,
  generateAudienceEmailUser,
} from "../../llm/prompts";
import { normalizeEmailBody } from "../../compliance/emailBody";
import { normalizeSiteAnalysis } from "../../db/analysis";
import { audiences, projects, siteProfiles } from "../../db/repo";
import { env } from "../../env";
import { preferencesForPrompt, preferencesFromProject } from "../../workflow/preferences";
import { jobBriefPromptBlock, jobSearchBriefFromProject } from "../../workflow/jobBrief";
import { JOB_LANE_MERGE_FIELDS } from "../template";

type EmailTemplateResult = {
  templateSubject?: string;
  templateBody?: string;
};

const FORBIDDEN_JOB_MERGES = /\{\{\s*(observation|painPoint|valueProp|pain_point|value_prop)\s*\}\}/i;
const ALLOWED_JOB_MERGE_RE = new RegExp(
  `\\{\\{\\s*(${JOB_LANE_MERGE_FIELDS.join("|")})\\s*\\}\\}`,
  "g",
);

function scrubJobTemplate(body: string): string {
  return body
    .replace(FORBIDDEN_JOB_MERGES, "")
    .replace(/https?:\/\/\S+/gi, "")
    .replace(/www\.\S+/gi, "")
    .replace(/[ \t]+\n/g, "\n")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

function hasForbiddenJobMerges(text: string): boolean {
  if (FORBIDDEN_JOB_MERGES.test(text)) return true;
  const leftovers = text.replace(ALLOWED_JOB_MERGE_RE, "").match(/\{\{[^}]+\}\}/);
  return Boolean(leftovers);
}

function missingRequiredJobMerges(text: string): string[] {
  const required = ["firstName", "company", "senderFullName", "role"] as const;
  return required.filter((field) => !new RegExp(`\\{\\{\\s*${field}\\s*\\}\\}`, "i").test(text));
}

/**
 * Write (or rewrite) the cold-email template for ONE audience.
 * Kept separate from bulk ICP generation so the model focuses on copy quality.
 */
export async function generateAudienceEmail(
  projectId: number,
  payload: { audienceId: number },
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
  const senderFirstName =
    (env.sender.fromName || "").trim().split(/\s+/)[0] || "Rohit";
  const senderFullName = (env.sender.fromName || "").trim() || senderFirstName;
  const isJob = project.workflow_type === "job_outreach";

  const user = generateAudienceEmailUser({
    analysis,
    audience: {
      name: audience.name,
      description: audience.description,
      targetRoles: audience.targetRoles,
      jobTitles: audience.jobTitles,
      industries: audience.industries,
      companySizes: audience.companySizes,
      painPoints: audience.painPoints,
      valueProp: audience.valueProp,
    },
    preferences,
    senderFirstName: isJob ? senderFullName : senderFirstName,
    workflowType: project.workflow_type,
    jobBriefBlock: jobBrief ? jobBriefPromptBlock(jobBrief) : undefined,
  });

  let result = await completeJson<EmailTemplateResult>({
    system: isJob ? GENERATE_JOB_AUDIENCE_EMAIL_SYSTEM : GENERATE_AUDIENCE_EMAIL_SYSTEM,
    user,
    tier: "reasoning",
    projectId,
    label: "generate_audience_email",
  });

  if (isJob) {
    const combined = `${result.templateSubject ?? ""}\n${result.templateBody ?? ""}`;
    const missing = missingRequiredJobMerges(combined);
    const badMerges = hasForbiddenJobMerges(combined);
    if (badMerges || missing.length > 0) {
      result = await completeJson<EmailTemplateResult>({
        system: GENERATE_JOB_AUDIENCE_EMAIL_SYSTEM,
        user: `${user}

Your previous attempt was rejected.
${badMerges ? "- It used forbidden merge fields ({{observation}}/{{painPoint}}/{{valueProp}} or unknown fields)." : ""}
${missing.length ? `- Missing required placeholders: ${missing.map((m) => `{{${m}}}`).join(", ")}` : ""}
Also include {{techStack}}, {{companyFocus}}, and {{workStyle}} where they read naturally.

Previous attempt:
${JSON.stringify(result, null, 2)}`,
        tier: "reasoning",
        projectId,
        label: "generate_audience_email_retry",
      });
    }
  }

  const templateSubject = (result.templateSubject ?? "").trim();
  let templateBody = result.templateBody ? normalizeEmailBody(result.templateBody) : "";
  if (!templateSubject || !templateBody) {
    throw new Error(`Model returned an empty email template for “${audience.name}”.`);
  }
  if (isJob) {
    templateBody = scrubJobTemplate(templateBody);
    if (hasForbiddenJobMerges(`${templateSubject}\n${templateBody}`)) {
      throw new Error(
        `Email template for “${audience.name}” still had invalid merge fields after rewrite. Try Rewrite email again.`,
      );
    }
  }

  audiences.update(audience.id, { templateSubject, templateBody });
  return `Wrote email template for “${audience.name}”.`;
}
