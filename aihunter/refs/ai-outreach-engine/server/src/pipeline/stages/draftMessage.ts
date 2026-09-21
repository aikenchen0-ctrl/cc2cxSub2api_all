import { activeModel, completeJson } from "../../llm/client";
import {
  DRAFT_EMAIL_SYSTEM,
  DRAFT_JOB_EMAIL_SYSTEM,
  EXTRACT_JOB_LISTING_SYSTEM,
  draftEmailUser,
} from "../../llm/prompts";
import { env } from "../../env";
import { audiences, companies, contacts, messages, projects, siteProfiles } from "../../db/repo";
import { normalizeEmailBody } from "../../compliance/emailBody";
import { newUnsubscribeToken } from "../../compliance/footer";
import {
  localizeEmailsForProject,
  tailorJobEmailsEnabled,
} from "../../workflow/preferences";
import { jobBriefPromptBlock, jobSearchBriefFromProject } from "../../workflow/jobBrief";
import { peekJobListing } from "../../providers/crawler";
import { screenContact } from "../../compliance/guards";
import { contextFromParts, renderTemplate } from "../template";

type Draft = { subject: string; body: string; personalisation: string };

type JdFields = {
  role: string;
  techStack: string;
  companyFocus: string;
  workStyle: string;
  hookDetail: string;
};

const COUNTRY_LANGUAGE: Record<string, string> = {
  AT: "German",
  BE: "Dutch or French",
  BR: "Portuguese",
  CH: "German",
  CL: "Spanish",
  CO: "Spanish",
  DE: "German",
  ES: "Spanish",
  FR: "French",
  IT: "Italian",
  MX: "Spanish",
  PE: "Spanish",
  PT: "Portuguese",
};

/** Phrasing that reads as machine-written and/or trips spam classifiers. */
const BANNED_PHRASES = [
  "i hope this email finds you well",
  "i hope this finds you well",
  "i came across your profile",
  "reaching out to see if",
  "reaching out because",
  "i wanted to introduce",
  "noticed your company is in a growth phase",
  "game-changing",
  "revolutionary",
  "cutting-edge",
  "circle back",
  "touch base",
  "as per my last",
  "act now",
  "limited time",
  "click here",
  "exclusive offer",
  "dear sir",
  "dear madam",
  "to whom it may concern",
  "dear recruiter",
  "passionate about",
  "teams building products like",
];

function critique(draft: Draft, workflowType: string): string[] {
  const problems: string[] = [];
  const words = draft.body.trim().split(/\s+/).length;
  if (words > 160) problems.push(`body is ${words} words, too long for a cold email`);
  if (words < 30) problems.push(`body is only ${words} words`);
  if (draft.subject.length > 60) problems.push("subject is over 60 characters");
  if (!draft.subject.trim()) problems.push("subject is empty");
  if (/\{\{|\}\}|\[[A-Z_]+\]/.test(draft.body + draft.subject)) {
    problems.push("contains an unfilled merge placeholder");
  }
  if (/—|–/.test(draft.body + draft.subject)) {
    problems.push("contains an em/en dash");
  }
  const haystack = `${draft.subject} ${draft.body}`.toLowerCase();
  for (const phrase of BANNED_PHRASES) {
    if (haystack.includes(phrase)) problems.push(`uses the phrase "${phrase}"`);
  }
  if (/https?:\/\//i.test(draft.body) || /www\./i.test(draft.body)) {
    problems.push("contains a URL");
  }
  return problems;
}

function localLanguage(
  country: string,
  project: { workflow_type: "customer_outreach" | "investor_outreach" | "job_outreach"; workflow_data: string },
): string {
  if (!localizeEmailsForProject(project)) return "English";
  return COUNTRY_LANGUAGE[country.toUpperCase()] ?? "English";
}

function parseWorkflowData(raw: string): Record<string, unknown> {
  try {
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === "object" ? (parsed as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

function cleanJdFields(raw: Partial<JdFields> | null | undefined, fallbackRole: string): JdFields {
  const work = String(raw?.workStyle ?? "").trim();
  const normalizedWork = /remote/i.test(work)
    ? "Remote"
    : /hybrid/i.test(work)
      ? "Hybrid"
      : /on[-\s]?site|office|in[-\s]?office/i.test(work)
        ? "On-site"
        : "";
  return {
    role: String(raw?.role ?? "").trim() || fallbackRole,
    techStack: String(raw?.techStack ?? "")
      .trim()
      .replace(/\s+/g, " ")
      .slice(0, 120),
    companyFocus: String(raw?.companyFocus ?? "")
      .trim()
      .replace(/\s+/g, " ")
      .slice(0, 120),
    workStyle: normalizedWork,
    hookDetail: String(raw?.hookDetail ?? "")
      .trim()
      .replace(/\s+/g, " ")
      .slice(0, 160),
  };
}

async function localizeDraft(args: {
  draft: Draft;
  language: string;
  projectId: number;
}): Promise<Draft> {
  if (args.language === "English") return args.draft;
  return completeJson<Draft>({
    system: `Translate a short B2B cold email while preserving the exact meaning, ask, tone, and sender voice.

Rules:
- Keep the subject under 60 characters if possible.
- Keep the body concise and natural in ${args.language}.
- Do not add new claims, facts, offers, or invented legal/compliance footers.
- Return JSON only: { "subject": string, "body": string, "personalisation": string }`,
    user: JSON.stringify(args.draft, null, 2),
    tier: "cheap",
    projectId: args.projectId,
    label: "localize_email",
  });
}

async function extractJdFields(args: {
  projectId: number;
  companyName: string;
  companyDescription: string;
  listingTitle?: string;
  listingExcerpt?: string;
  fallbackRole: string;
}): Promise<JdFields> {
  if (!args.listingExcerpt && !args.listingTitle && !args.companyDescription) {
    return cleanJdFields(null, args.fallbackRole);
  }
  try {
    const extracted = await completeJson<Partial<JdFields>>({
      system: EXTRACT_JOB_LISTING_SYSTEM,
      user: `Company: ${args.companyName}
Hiring signal: ${args.companyDescription || "none"}
Listing title: ${args.listingTitle || "unknown"}

Listing text:
${(args.listingExcerpt || args.companyDescription || "").slice(0, 3500)}`,
      tier: "cheap",
      projectId: args.projectId,
      label: "extract_job_listing",
    });
    return cleanJdFields(extracted, args.fallbackRole);
  } catch {
    return cleanJdFields(
      {
        role: args.fallbackRole,
        companyFocus: args.companyDescription.slice(0, 120),
        hookDetail: args.listingTitle || "",
      },
      args.fallbackRole,
    );
  }
}

/** Clean empty merge leftovers after rendering a lane template. */
function cleanupJobTemplateRender(text: string): string {
  return text
    .replace(/\(\s*\)/g, "")
    .replace(/\[\s*\]/g, "")
    .replace(/\s+([,.;:])/g, "$1")
    .replace(/,\s*,/g, ",")
    .replace(/\s{2,}/g, " ")
    .replace(/[ \t]+\n/g, "\n")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

export async function draftMessage(
  projectId: number,
  payload: { contactId: number; forceNew?: boolean },
): Promise<string> {
  const contact = contacts.get(payload.contactId);
  if (!contact) throw new Error(`Contact ${payload.contactId} not found`);
  if (payload.forceNew) {
    // Never create a second outreach draft after a successful send.
    if (messages.hasSentForContact(contact.id)) {
      return `${contact.email} already sent; skipped redraft.`;
    }
    messages.removeUnsentForContact(contact.id);
  } else if (messages.existsForContact(contact.id)) {
    return `${contact.email} already has a message; skipped.`;
  }

  const verdict = screenContact(contact);
  if (!verdict.ok) return `${contact.email} skipped: ${verdict.reason}`;

  const company = companies.get(contact.company_id);
  const audience = audiences.get(contact.audience_id);
  const profile = siteProfiles.latest(projectId);
  const project = projects.get(projectId);
  if (!company || !audience) throw new Error("Contact is missing its company or audience");
  if (!project) throw new Error(`Project ${projectId} not found`);
  if (!profile?.analysis) throw new Error("No campaign analysis available to write from");

  const isJob = project.workflow_type === "job_outreach";
  const workflowData = parseWorkflowData(project.workflow_data);
  // Never feed profile URLs into the drafter — cold emails stay link-free for inboxing.
  delete workflowData.linkedinUrl;
  delete workflowData.resumeUrl;

  // Customer/investor: cheap merge render when a template exists.
  if (!isJob && audience.templateBody.trim()) {
    const context = contextFromParts({
      fullName: contact.full_name,
      title: contact.title,
      company: company.name,
      companyDomain: company.domain,
      companyDescription: company.description,
      painPoint: audience.painPoints[0] ?? "",
      valueProp: audience.valueProp,
    });
    let subject = renderTemplate(
      audience.templateSubject || "{{company}} and meeting notes",
      context,
    ).trim();
    let body = normalizeEmailBody(renderTemplate(audience.templateBody, context));
    const language = localLanguage(company.country, project);
    if (language !== "English") {
      const localized = await localizeDraft({
        draft: {
          subject,
          body,
          personalisation: `Translated from the "${audience.name}" template into ${language}.`,
        },
        language,
        projectId,
      });
      subject = localized.subject.trim();
      body = normalizeEmailBody(localized.body);
    }
    const remaining = critique({ subject, body, personalisation: "" }, project.workflow_type);

    const message = messages.create({
      projectId,
      contactId: contact.id,
      audienceId: audience.id,
      subject,
      body,
      htmlBody: "",
      model: language === "English" ? "audience-template" : `audience-template-${language}`,
      personalisation: [
        `Rendered from "${audience.name}" template.`,
        language !== "English" ? `Localized to ${language}.` : "",
        remaining.length ? `Review flags: ${remaining.join("; ")}` : "",
      ]
        .filter(Boolean)
        .join(" | "),
      unsubscribeToken: newUnsubscribeToken(),
    });

    return `Rendered template message ${message.id} for ${contact.email}.`;
  }

  // Jobs — two modes (checkbox on Audiences):
  // 1) default: render lane template with JD merge fills (no per-contact email LLM)
  // 2) tailorEmailsToJobListing: LLM email written from the job listing
  const tailorFromListing = isJob && tailorJobEmailsEnabled(project);
  const jobListing =
    isJob && company.source_url
      ? await peekJobListing(company.source_url).catch(() => null)
      : null;
  const fallbackRole = audience.targetRoles[0] || "";

  if (isJob && !tailorFromListing && audience.templateBody.trim()) {
    const jdFields = await extractJdFields({
      projectId,
      companyName: company.name,
      companyDescription: company.description,
      listingTitle: jobListing?.title,
      listingExcerpt: jobListing?.excerpt,
      fallbackRole,
    });
    const context = contextFromParts({
      fullName: contact.full_name,
      title: contact.title,
      company: company.name,
      companyDomain: company.domain,
      companyDescription: company.description,
      painPoint: audience.painPoints[0] ?? "",
      valueProp: audience.valueProp,
      role: jdFields.role,
      techStack: jdFields.techStack,
      companyFocus: jdFields.companyFocus,
      workStyle: jdFields.workStyle,
    });
    let subject = cleanupJobTemplateRender(
      renderTemplate(audience.templateSubject || "{{role}} at {{company}}", context),
    );
    let body = normalizeEmailBody(
      cleanupJobTemplateRender(renderTemplate(audience.templateBody, context)),
    );
    // Strip any leftover merge fields if the template used unsupported keys.
    subject = subject.replace(/\{\{[^}]+\}\}/g, "").replace(/\s{2,}/g, " ").trim();
    body = body.replace(/\{\{[^}]+\}\}/g, "").replace(/[ \t]+\n/g, "\n").replace(/\n{3,}/g, "\n\n").trim();

    const remaining = critique({ subject, body, personalisation: "" }, project.workflow_type);
    const usedFields = [
      jdFields.role && `role=${jdFields.role}`,
      jdFields.techStack && `stack=${jdFields.techStack}`,
      jdFields.companyFocus && `focus=${jdFields.companyFocus}`,
      jdFields.workStyle && `work=${jdFields.workStyle}`,
    ]
      .filter(Boolean)
      .join("; ");

    const message = messages.create({
      projectId,
      contactId: contact.id,
      audienceId: audience.id,
      subject,
      body,
      htmlBody: "",
      model: "lane-template",
      personalisation: [
        `Rendered from “${audience.name}” lane template.`,
        usedFields ? `JD fills: ${usedFields}` : "",
        jobListing ? `Listing: ${jobListing.title || company.source_url}` : "",
        remaining.length ? `Review flags: ${remaining.join("; ")}` : "",
      ]
        .filter(Boolean)
        .join(" | "),
      unsubscribeToken: newUnsubscribeToken(),
    });

    return `Rendered lane template message ${message.id} for ${contact.email}.`;
  }

  const jdFields = isJob
    ? await extractJdFields({
        projectId,
        companyName: company.name,
        companyDescription: company.description,
        listingTitle: jobListing?.title,
        listingExcerpt: jobListing?.excerpt,
        fallbackRole,
      })
    : null;

  const jobBrief = isJob ? jobSearchBriefFromProject(project) : null;
  const user = draftEmailUser({
    senderName: env.sender.fromName,
    workflowType: project.workflow_type,
    workflowData,
    product: JSON.parse(profile.analysis),
    audienceName: audience.name,
    targetRoles: audience.targetRoles,
    valueProp: audience.valueProp,
    painPoints: audience.painPoints,
    templateSubject: audience.templateSubject,
    templateBody: audience.templateBody,
    contactName: contact.full_name,
    contactTitle: contact.title,
    companyName: company.name,
    companyDomain: company.domain,
    companyDescription: company.description,
    companySourceUrl: company.source_url,
    jobListingTitle: jobListing?.title,
    jobListingExcerpt: jobListing?.excerpt,
    jdFields: jdFields ?? undefined,
    evidenceUrl: contact.evidence_url,
    jobBriefBlock: jobBrief ? jobBriefPromptBlock(jobBrief) : undefined,
    tailorFromListing,
  });

  const system = isJob ? DRAFT_JOB_EMAIL_SYSTEM : DRAFT_EMAIL_SYSTEM;

  let draft = await completeJson<Draft>({
    system,
    user,
    tier: "reasoning",
    projectId,
    label: "draft_email",
  });

  const problems = critique(draft, project.workflow_type);
  if (problems.length > 0) {
    draft = await completeJson<Draft>({
      system,
      user: `${user}\n\nYour previous attempt was rejected for these reasons:\n- ${problems.join(
        "\n- ",
      )}\n\nPrevious attempt:\n${JSON.stringify(draft, null, 2)}\n\nRewrite it, fixing every issue.${
        isJob
          ? "\nWrite a natural email from the job listing. Leave no {{merge}} fields."
          : ""
      }`,
      tier: "reasoning",
      projectId,
      label: "draft_email_retry",
    });
  }

  let body = normalizeEmailBody(draft.body);
  if (isJob) {
    body = body
      .replace(/https?:\/\/\S+/gi, "")
      .replace(/www\.\S+/gi, "")
      .replace(/[ \t]+\n/g, "\n")
      .replace(/\n{3,}/g, "\n\n")
      .trim();
  }
  const remaining = critique(
    { subject: draft.subject.trim(), body, personalisation: draft.personalisation },
    project.workflow_type,
  );
  const usedFields = jdFields
    ? [
        jdFields.role && `role=${jdFields.role}`,
        jdFields.techStack && `stack=${jdFields.techStack}`,
        jdFields.companyFocus && `focus=${jdFields.companyFocus}`,
        jdFields.workStyle && `work=${jdFields.workStyle}`,
      ]
        .filter(Boolean)
        .join("; ")
    : "";
  const message = messages.create({
    projectId,
    contactId: contact.id,
    audienceId: audience.id,
    subject: draft.subject.trim(),
    body,
    model: activeModel("reasoning"),
    personalisation: [
      draft.personalisation,
      usedFields ? `JD fields: ${usedFields}` : "",
      jobListing ? `Listing: ${jobListing.title || company.source_url}` : "",
      remaining.length ? `Review flags: ${remaining.join("; ")}` : "",
    ]
      .filter(Boolean)
      .join(" | "),
    unsubscribeToken: newUnsubscribeToken(),
  });

  return `Drafted message ${message.id} for ${contact.email}${
    isJob ? " (from job listing)" : ""
  }.`;
}
