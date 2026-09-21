import { Hono } from "hono";
import type { Context } from "hono";
import { z } from "zod";
import { activeSenderAddresses, capabilities, env } from "../env";
import { getSettings, sendWindowLabel, updateSettings, type SettingsPatch } from "../settings";
import {
  audiences,
  companies,
  contacts,
  events,
  jobs,
  messages,
  projects,
  siteProfiles,
  suppressions,
} from "../db/repo";
import { normalizeSiteAnalysis, summarizePricingPlans } from "../db/analysis";
import type { JobRow, JobStage, WorkflowType } from "../db/types";
import { normalizeEmailBody } from "../compliance/emailBody";
import { withFooter } from "../compliance/footer";
import { preflight } from "../compliance/guards";
import {
  addManualContact,
  addManualContacts,
  parseContactsCsv,
  updateManualContact,
} from "../contacts/manual";
import { STAGE_LABELS } from "../pipeline/stages";
import { cancelProjectJobs, type CancelScope } from "../pipeline/runner";
import { contextFromParts, renderTemplate } from "../pipeline/template";
import { llmAvailable } from "../llm/client";
import { searchAvailable } from "../providers/search";
import { clearVerifyQuotaPause, verificationGate } from "../providers/verifyGate";
import {
  emptyPreferences,
  mergePreferencesIntoWorkflowData,
  parseStoredPreferences,
  preferencesFromProject,
} from "../workflow/preferences";

export const api = new Hono();

export type PipelineJob = {
  id: number;
  stage: JobStage;
  label: string;
  target: string;
  status: JobRow["status"];
  attempts: number;
  maxAttempts: number;
  lastError: string | null;
  runAfter: string;
  updatedAt: string;
};

function describeJobTarget(stage: JobStage, payloadRaw: string): string {
  let payload: Record<string, unknown> = {};
  try {
    payload = JSON.parse(payloadRaw) as Record<string, unknown>;
  } catch {
    /* ignore */
  }

  switch (stage) {
    case "scan_site":
      return "Campaign profile";
    case "generate_audiences":
      return "From campaign profile";
    case "generate_audience_email":
    case "regenerate_audience":
    case "find_companies": {
      const audienceId = Number(payload.audienceId);
      const audience = Number.isFinite(audienceId) ? audiences.get(audienceId) : null;
      return audience ? `Audience · ${audience.name}` : `Audience #${payload.audienceId ?? "?"}`;
    }
    case "find_contacts": {
      const companyId = Number(payload.companyId);
      const company = Number.isFinite(companyId) ? companies.get(companyId) : null;
      return company
        ? `${company.name} · ${company.domain}`
        : `Company #${payload.companyId ?? "?"}`;
    }
    case "verify_contact":
    case "draft_message": {
      const contactId = Number(payload.contactId);
      const contact = Number.isFinite(contactId) ? contacts.get(contactId) : null;
      if (!contact) return `Contact #${payload.contactId ?? "?"}`;
      const name = contact.full_name?.trim() || "Contact";
      return `${name} · ${contact.email}`;
    }
    case "send_message": {
      const messageId = Number(payload.messageId);
      const message = Number.isFinite(messageId) ? messages.get(messageId) : null;
      if (!message) return `Message #${payload.messageId ?? "?"}`;
      const contact = contacts.get(message.contact_id);
      return contact
        ? `${contact.email} · ${message.subject}`
        : message.subject || `Message #${message.id}`;
    }
    default:
      return "Pipeline step";
  }
}

function toPipelineJob(job: JobRow): PipelineJob {
  return {
    id: job.id,
    stage: job.stage,
    label: STAGE_LABELS[job.stage] ?? job.stage,
    target: describeJobTarget(job.stage, job.payload),
    status: job.status,
    attempts: job.attempts,
    maxAttempts: job.max_attempts,
    lastError: job.last_error,
    runAfter: job.run_after,
    updatedAt: job.updated_at,
  };
}

function pipelineSnapshot(projectId: number) {
  const active = jobs.listByStatuses(projectId, ["running", "pending"], 40).map(toPipelineJob);
  const failed = jobs.listByStatuses(projectId, ["failed"], 100).map(toPipelineJob);
  return {
    active,
    failed,
    failedCount: jobs.failedCount(projectId),
    pendingJobs: jobs.pendingCount(projectId),
  };
}

const domainSchema = z
  .string()
  .trim()
  .min(3)
  .transform((value) =>
    value
      .replace(/^https?:\/\//i, "")
      .replace(/\/.*$/, "")
      .replace(/^www\./i, "")
      .toLowerCase(),
  )
  .refine((value) => /^[a-z0-9-]+(\.[a-z0-9-]+)+$/.test(value), "not a valid domain");

const audienceSchema = z.object({
  name: z.string().trim().min(1),
  description: z.string().default(""),
  targetRoles: z.array(z.string()).default([]),
  jobTitles: z.array(z.string()).default([]),
  industries: z.array(z.string()).default([]),
  companySizes: z.array(z.string()).default([]),
  sourceAdapters: z.array(z.string()).default([]),
  fundedAfterYear: z.number().int().min(0).max(2100).default(0),
  searchQueries: z.array(z.string()).default([]),
  painPoints: z.array(z.string()).default([]),
  valueProp: z.string().default(""),
  templateSubject: z.string().default(""),
  templateBody: z.string().default(""),
  templateHtml: z.string().default(""),
  enabled: z.boolean().default(true),
});

function parseId(value: string | undefined): number {
  const id = Number.parseInt(value ?? "", 10);
  if (!Number.isFinite(id)) throw new Error("invalid id");
  return id;
}

/* ----------------------------------------------------------------- status */

api.get("/health", (c) => {
  const settings = getSettings();
  return c.json({
    ok: true,
    capabilities: capabilities(),
    sender: {
      provider: env.sender.provider,
      fromEmail: activeSenderAddresses().fromEmail,
      fromName: env.sender.fromName,
      dailyCap: settings.dailySendCap,
      window: sendWindowLabel(settings),
      sendOnWeekends: settings.sendOnWeekends,
      minMinutesBetweenSends: settings.minMinutesBetweenSends,
    },
    prospecting: {
      maxContactsPerCompany: settings.maxContactsPerCompany,
      targetMarkets: settings.targetMarkets,
      targetCompanySignals: settings.targetCompanySignals,
      localizeEmails: settings.localizeEmails,
    },
    blockers: preflight(),
  });
});

const csvListSchema = z
  .union([z.array(z.string()), z.string()])
  .transform((value) =>
    (Array.isArray(value) ? value : value.split(","))
      .map((part) => part.trim())
      .filter(Boolean),
  );

const settingsSchema = z.object({
  blockedCountries: csvListSchema.optional(),
  blockFreemail: z.boolean().optional(),
  brandLogoUrl: z.string().optional(),
  brandProductUrl: z.string().optional(),
  brandPrimaryColor: z.string().optional(),
  dailySendCap: z.number().int().min(1).max(500).optional(),
  minMinutesBetweenSends: z.number().int().min(0).max(120).optional(),
  sendWindowStartHour: z.number().int().min(0).max(23).optional(),
  sendWindowEndHour: z.number().int().min(1).max(24).optional(),
  sendWindowTimezone: z.string().trim().min(1).optional(),
  sendOnWeekends: z.boolean().optional(),
  includeUnsubscribeLink: z.boolean().optional(),
  unsubscribeBaseUrl: z.string().trim().min(1).optional(),
  localizeEmails: z.boolean().optional(),
  targetMarkets: csvListSchema.optional(),
  targetCompanySignals: csvListSchema.optional(),
  maxContactsPerCompany: z.number().int().min(1).max(15).optional(),
  crawlMaxPages: z.number().int().min(1).max(50).optional(),
});

api.get("/settings", (c) => c.json({ settings: getSettings() }));

api.patch("/settings", async (c) => {
  const body = await c.req.json().catch(() => ({}));
  const parsed = settingsSchema.safeParse(body);
  if (!parsed.success) {
    return c.json({ error: parsed.error.issues[0]?.message ?? "invalid request" }, 400);
  }
  const settings = updateSettings(parsed.data as SettingsPatch);
  return c.json({ settings });
});

/* --------------------------------------------------------------- projects */

const emailFormatSchema = z.enum(["html", "plain"]);
const workflowTypeSchema = z
  .enum(["customer_outreach", "investor_outreach", "job_outreach"])
  .default("customer_outreach");

function slugPart(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40);
}

function campaignKey(workflowType: WorkflowType, seed: string): string {
  const slug = slugPart(seed) || workflowType.replace(/_/g, "-");
  const suffix = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
  return `${slug}-${suffix}.local`;
}

function cleanLines(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).map((item) => item.trim()).filter(Boolean);
  if (typeof value !== "string") return [];
  return value
    .split(/\n|,/)
    .map((item) => item.trim())
    .filter(Boolean);
}

api.post("/projects", async (c) => {
  const body = await c.req.json().catch(() => ({}));
  const parsed = z
    .object({
      workflowType: workflowTypeSchema,
      domain: z.string().trim().optional(),
      name: z.string().trim().optional(),
      emailFormat: emailFormatSchema.optional(),
      raiseStage: z.string().trim().optional(),
      checkSize: z.string().trim().optional(),
      geography: z.string().trim().optional(),
      sectors: z.union([z.string(), z.array(z.string())]).optional(),
      traction: z.string().trim().optional(),
      fundraisingNotes: z.string().trim().optional(),
      resumeText: z.string().trim().optional(),
      targetRoles: z.union([z.string(), z.array(z.string())]).optional(),
      locations: z.union([z.string(), z.array(z.string())]).optional(),
      remotePreference: z.string().trim().optional(),
      seniority: z.string().trim().optional(),
      jobSearchNotes: z.string().trim().optional(),
      linkedinUrl: z.string().trim().optional(),
      resumeUrl: z.string().trim().optional(),
      preferences: z.record(z.string(), z.unknown()).optional(),
    })
    .safeParse(body);
  if (!parsed.success) {
    return c.json({ error: parsed.error.issues[0]?.message ?? "invalid request" }, 400);
  }
  if (!llmAvailable()) return c.json({ error: "No LLM key configured (OPENAI_API_KEY)." }, 400);
  const workflowType = parsed.data.workflowType;
  const rawDomain = parsed.data.domain ?? "";
  const domainResult = rawDomain ? domainSchema.safeParse(rawDomain) : null;
  if ((workflowType === "customer_outreach" || workflowType === "investor_outreach") && !domainResult?.success) {
    return c.json({ error: "A valid product domain is required for this campaign type." }, 400);
  }
  if (workflowType === "job_outreach" && (parsed.data.resumeText ?? "").trim().length < 80) {
    return c.json({ error: "Paste at least 80 characters of resume/profile text." }, 400);
  }

  const preferences = parseStoredPreferences(
    workflowType,
    parsed.data.preferences ?? emptyPreferences(workflowType),
  );
  const workflowData = mergePreferencesIntoWorkflowData(
    workflowType,
    workflowType === "investor_outreach"
      ? {
          productDomain: domainResult?.success ? domainResult.data : rawDomain,
          raiseStage: parsed.data.raiseStage ?? "",
          checkSize: parsed.data.checkSize ?? "",
          geography: parsed.data.geography ?? "",
          sectors: cleanLines(parsed.data.sectors),
          traction: parsed.data.traction ?? "",
          notes: parsed.data.fundraisingNotes ?? "",
        }
      : workflowType === "job_outreach"
        ? {
            resumeText: parsed.data.resumeText ?? "",
            targetRoles: cleanLines(parsed.data.targetRoles),
            locations: cleanLines(parsed.data.locations),
            remotePreference: parsed.data.remotePreference ?? "",
            seniority: parsed.data.seniority ?? "",
            notes: parsed.data.jobSearchNotes ?? "",
            linkedinUrl: parsed.data.linkedinUrl ?? "",
            resumeUrl: parsed.data.resumeUrl ?? "",
          }
        : {},
    preferences,
  );
  const domain: string =
    workflowType === "customer_outreach"
      ? (domainResult?.success ? domainResult.data : rawDomain)
      : campaignKey(
          workflowType,
          workflowType === "investor_outreach"
            ? `${domainResult?.success ? domainResult.data : rawDomain} investors`
            : parsed.data.name || cleanLines(parsed.data.targetRoles)[0] || "job-search",
        );
  const defaultName: string =
    workflowType === "investor_outreach"
      ? `${domainResult?.success ? domainResult.data : rawDomain} investor outreach`
      : workflowType === "job_outreach"
        ? parsed.data.name || `${cleanLines(parsed.data.targetRoles)[0] ?? "Job"} outreach`
        : domain;
  const project = projects.create(
    parsed.data.name || defaultName,
    domain,
    "plain",
    workflowType,
    workflowData,
  );
  jobs.enqueue(project.id, "scan_site");
  return c.json({ project, preferences: preferencesFromProject(project) }, 201);
});

api.patch("/projects/:id", async (c) => {
  const id = parseId(c.req.param("id"));
  const existing = projects.get(id);
  if (!existing) return c.json({ error: "not found" }, 404);
  const body = await c.req.json().catch(() => ({}));
  const parsed = z
    .object({
      preferences: z.record(z.string(), z.unknown()).optional(),
      linkedinUrl: z.string().trim().optional(),
      resumeUrl: z.string().trim().optional(),
    })
    .safeParse(body);
  if (!parsed.success) {
    return c.json({ error: parsed.error.issues[0]?.message ?? "invalid request" }, 400);
  }
  const hasPrefs = Boolean(parsed.data.preferences);
  const hasJobLinks =
    parsed.data.linkedinUrl !== undefined || parsed.data.resumeUrl !== undefined;
  if (!hasPrefs && !hasJobLinks) {
    return c.json({ error: "Provide preferences and/or linkedinUrl/resumeUrl" }, 400);
  }

  let currentData: Record<string, unknown> = {};
  try {
    currentData = JSON.parse(existing.workflow_data) as Record<string, unknown>;
  } catch {
    currentData = {};
  }

  let nextData = currentData;
  if (hasPrefs) {
    nextData = mergePreferencesIntoWorkflowData(
      existing.workflow_type,
      nextData,
      parsed.data.preferences!,
    );
  }
  if (existing.workflow_type === "job_outreach" && hasJobLinks) {
    nextData = {
      ...nextData,
      ...(parsed.data.linkedinUrl !== undefined
        ? { linkedinUrl: parsed.data.linkedinUrl }
        : {}),
      ...(parsed.data.resumeUrl !== undefined ? { resumeUrl: parsed.data.resumeUrl } : {}),
    };
  }

  const project = projects.setWorkflowData(id, nextData) ?? existing;
  return c.json({ project, preferences: preferencesFromProject(project) });
});

/** Live preview of how a body will look when sent (plain text). */
api.post("/projects/:id/email-preview", async (c) => {
  const id = parseId(c.req.param("id"));
  const project = projects.get(id);
  if (!project) return c.json({ error: "not found" }, 404);

  const parsed = z
    .object({
      subject: z.string().optional().default(""),
      body: z.string(),
      /** Custom HTML override. Empty/omitted = auto-wrap plain body. */
      htmlBody: z.string().optional().default(""),
      /** When true, fill {{merge}} fields with sample values for audience templates. */
      fillSampleMergeFields: z.boolean().optional().default(false),
      painPoint: z.string().optional().default(""),
      valueProp: z.string().optional().default(""),
    })
    .safeParse(await c.req.json().catch(() => ({})));
  if (!parsed.success) {
    return c.json({ error: parsed.error.issues[0]?.message ?? "invalid request" }, 400);
  }

  let subject = parsed.data.subject;
  let body = parsed.data.body;
  let htmlBody = parsed.data.htmlBody;
  if (parsed.data.fillSampleMergeFields) {
    const context = contextFromParts({
      fullName: "Alex Rivera",
      title: "Founder",
      company: "Northstar Labs",
      companyDomain: "northstar.example",
      companyDescription: "Northstar Labs just closed a seed round and runs weekly investor updates.",
      painPoint: parsed.data.painPoint,
      valueProp: parsed.data.valueProp,
      // Sample JD fills so job-lane template previews render cleanly.
      role: "Senior Frontend Engineer",
      techStack: "React, TypeScript",
      companyFocus: "AI meeting notes for sales teams",
      workStyle: "Remote",
    });
    subject = renderTemplate(subject, context);
    body = renderTemplate(body, context);
    htmlBody = renderTemplate(htmlBody, context);
  }

  body = normalizeEmailBody(body);
  subject = subject.trim();
  const token = "preview";

  return c.json({
    format: "plain" as const,
    subject,
    text: withFooter(body, token),
    html: null,
    htmlIsCustom: false,
    brand: (() => {
      const settings = getSettings();
      return {
        logoUrl: settings.brandLogoUrl || null,
        productUrl: settings.brandProductUrl,
        primaryColor: settings.brandPrimaryColor,
        logoMode: settings.brandLogoUrl ? ("image" as const) : ("text" as const),
      };
    })(),
  });
});

api.get("/projects", (c) => c.json({ projects: projects.list() }));

api.get("/projects/:id", (c) => {
  const id = parseId(c.req.param("id"));
  const project = projects.get(id);
  if (!project) return c.json({ error: "not found" }, 404);

  const profile = siteProfiles.latest(id);
  const pipeline = pipelineSnapshot(id);
  const verify = verificationGate(id);
  // Keep draft counts honest for the sidebar / Approve all badge.
  messages.removeUnsentWhereAlreadySent(id);
  return c.json({
    project,
    preferences: preferencesFromProject(project),
    analysis: profile?.analysis
      ? normalizeSiteAnalysis(JSON.parse(profile.analysis))
      : null,
    pagesCrawled: profile ? (JSON.parse(profile.pages_json) as unknown[]).length : 0,
    audiences: audiences.list(id),
    counts: {
      companies: companies.countByProject(id),
      contacts: contacts.countByProject(id),
      sendableUnsentContacts: contacts.sendableUnsentCount(id),
      messages: messages.countsByStatus(id),
      sentTotal: messages.totalSent(id),
      sentLast24h: messages.sentInLastDay(id),
      pendingJobs: pipeline.pendingJobs,
      failedJobs: pipeline.failedCount,
    },
    jobs: jobs.summary(id).map((row) => ({
      ...row,
      label: STAGE_LABELS[row.stage as never] ?? row.stage,
    })),
    pipeline,
    verification: {
      allow: verify.allow,
      kind: verify.kind ?? null,
      reason: verify.reason ?? null,
    },
    estimatedSpendUsd: Number(events.totalCost(id).toFixed(4)),
  });
});

api.delete("/projects/:id", (c) => {
  const id = parseId(c.req.param("id"));
  const project = projects.get(id);
  if (!project) return c.json({ error: "not found" }, 404);

  const deleted = projects.remove(id);
  return c.json({ deleted });
});

api.get("/projects/:id/stream", (c) => {
  const id = parseId(c.req.param("id"));
  if (!projects.get(id)) return c.json({ error: "not found" }, 404);

  const encoder = new TextEncoder();
  let timer: ReturnType<typeof setInterval> | null = null;
  let lastSignature = "";

  const stream = new ReadableStream({
    start(controller) {
      const snapshot = () => {
        const pipeline = pipelineSnapshot(id);
        const payload = {
          pendingJobs: pipeline.pendingJobs,
          failedJobs: pipeline.failedCount,
          latestEventId: events.latestId(id),
          pipeline,
          jobs: jobs.summary(id),
          messages: messages.countsByStatus(id),
          sentTotal: messages.totalSent(id),
          sentLast24h: messages.sentInLastDay(id),
          at: new Date().toISOString(),
        };
        const signature = JSON.stringify({
          pendingJobs: payload.pendingJobs,
          failedJobs: payload.failedJobs,
          latestEventId: payload.latestEventId,
          jobs: payload.jobs,
          messages: payload.messages,
          sentTotal: payload.sentTotal,
          sentLast24h: payload.sentLast24h,
          active: payload.pipeline.active.map((job) => [
            job.id,
            job.status,
            job.runAfter,
            job.attempts,
          ]),
          failed: payload.pipeline.failed.map((job) => [job.id, job.lastError]),
        });
        return { payload, signature };
      };

      const sendIfChanged = () => {
        try {
          const { payload, signature } = snapshot();
          if (signature === lastSignature) {
            controller.enqueue(encoder.encode(`: heartbeat ${payload.at}\n\n`));
            return;
          }
          lastSignature = signature;
          controller.enqueue(
            encoder.encode(`event: update\ndata: ${JSON.stringify(payload)}\n\n`),
          );
        } catch {
          /* client may have disconnected mid-write */
        }
      };
      sendIfChanged();
      timer = setInterval(sendIfChanged, 2000);
    },
    cancel() {
      if (timer) clearInterval(timer);
    },
  });

  return new Response(stream, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
    },
  });
});

api.post("/projects/:id/rescan", async (c) => {
  const id = parseId(c.req.param("id"));
  const project = projects.get(id);
  if (!project) return c.json({ error: "not found" }, 404);
  if (!llmAvailable()) return c.json({ error: "No LLM key configured (OPENAI_API_KEY)." }, 400);

  const parsed = z
    .object({
      regenerateAudiences: z.boolean().optional().default(false),
      preserveProspecting: z.boolean().optional().default(true),
    })
    .safeParse(await c.req.json().catch(() => ({})));
  const opts = parsed.success
    ? parsed.data
    : { regenerateAudiences: false, preserveProspecting: true };

  jobs.enqueue(id, "scan_site", {
    regenerateAudiences: opts.regenerateAudiences,
    preserveProspecting: opts.preserveProspecting,
  });
  return c.json({
    queued: "scan_site",
    regenerateAudiences: opts.regenerateAudiences,
    preserveProspecting: opts.preserveProspecting,
  });
});

api.post("/audiences/:id/regenerate", (c) => {
  const id = parseId(c.req.param("id"));
  const audience = audiences.get(id);
  if (!audience) return c.json({ error: "not found" }, 404);
  if (!llmAvailable()) {
    return c.json({ error: "No LLM key configured. Set OPENAI_API_KEY." }, 400);
  }
  jobs.requeue(audience.projectId, "regenerate_audience", {
    audienceId: id,
    rewriteEmail: true,
  });
  return c.json({ queued: "regenerate_audience", audienceId: id });
});

api.post("/audiences/:id/regenerate-email", (c) => {
  const id = parseId(c.req.param("id"));
  const audience = audiences.get(id);
  if (!audience) return c.json({ error: "not found" }, 404);
  if (!llmAvailable()) {
    return c.json({ error: "No LLM key configured. Set OPENAI_API_KEY." }, 400);
  }
  jobs.requeue(audience.projectId, "generate_audience_email", { audienceId: id });
  return c.json({ queued: "generate_audience_email", audienceId: id });
});

api.post("/projects/:id/audiences/regenerate", async (c) => {
  const id = parseId(c.req.param("id"));
  if (!projects.get(id)) return c.json({ error: "not found" }, 404);
  if (!siteProfiles.latest(id)?.analysis) {
    return c.json({ error: "Finish setup analysis first." }, 400);
  }

  const parsed = z
    .object({
      preserveProspecting: z.boolean().optional().default(true),
    })
    .safeParse(await c.req.json().catch(() => ({})));
  const preserveProspecting = parsed.success ? parsed.data.preserveProspecting : true;

  jobs.enqueue(id, "generate_audiences", { preserveProspecting });
  return c.json({ queued: "generate_audiences", preserveProspecting });
});

api.patch("/projects/:id/analysis", async (c) => {
  const id = parseId(c.req.param("id"));
  const profile = siteProfiles.latest(id);
  if (!profile?.analysis) {
    return c.json({ error: "No analysis to edit. Complete setup first." }, 404);
  }

  const body = await c.req.json().catch(() => null);
  if (!body || typeof body !== "object") {
    return c.json({ error: "Invalid analysis payload" }, 400);
  }

  // Normalize (don't over-validate with brittle schemas) so edits like adding a
  // competitor domain or removing a row never fail on empty optional fields.
  const analysis = normalizeSiteAnalysis(body);
  if (!analysis.productName.trim()) {
    return c.json({ error: "Product name is required" }, 400);
  }
  analysis.pricing = analysis.pricing || summarizePricingPlans(analysis.pricingPlans);

  siteProfiles.setAnalysis(profile.id, analysis);
  return c.json({ analysis });
});

/* -------------------------------------------------------------- audiences */

api.get("/projects/:id/audiences", (c) =>
  c.json({ audiences: audiences.list(parseId(c.req.param("id"))) }),
);

api.post("/projects/:id/audiences", async (c) => {
  const id = parseId(c.req.param("id"));
  if (!projects.get(id)) return c.json({ error: "not found" }, 404);
  const parsed = audienceSchema.safeParse(await c.req.json().catch(() => ({})));
  if (!parsed.success) {
    return c.json({ error: parsed.error.issues[0]?.message ?? "invalid audience" }, 400);
  }
  const data = {
    ...parsed.data,
    templateBody: normalizeEmailBody(parsed.data.templateBody),
  };
  return c.json({ audience: audiences.create(id, { ...data, origin: "manual" }) }, 201);
});

api.patch("/audiences/:id", async (c) => {
  const id = parseId(c.req.param("id"));
  const parsed = audienceSchema.partial().safeParse(await c.req.json().catch(() => ({})));
  if (!parsed.success) {
    return c.json({ error: parsed.error.issues[0]?.message ?? "invalid audience" }, 400);
  }
  const data = {
    ...parsed.data,
    ...(parsed.data.templateBody !== undefined
      ? { templateBody: normalizeEmailBody(parsed.data.templateBody) }
      : {}),
  };
  const updated = audiences.update(id, data);
  return updated ? c.json({ audience: updated }) : c.json({ error: "not found" }, 404);
});

api.delete("/audiences/:id", (c) => {
  audiences.remove(parseId(c.req.param("id")));
  return c.json({ deleted: true });
});

api.post("/audiences/:id/prospect", (c) => {
  const id = parseId(c.req.param("id"));
  const audience = audiences.get(id);
  if (!audience) return c.json({ error: "not found" }, 404);
  if (!searchAvailable()) {
    return c.json(
      {
        error:
          "No search key configured. Set SERPER_API_KEY.",
      },
      400,
    );
  }
  if (audience.searchQueries.length === 0) {
    return c.json({ error: "This audience has no search queries. Add at least one." }, 400);
  }
  const searchPage = audience.nextSearchPage;
  const verify = verificationGate(audience.projectId);

  // Prior find_contacts runs may have soft-succeeded with 0 people (e.g. Serper
  // stripped LinkedIn). Re-queue those companies immediately so a pipeline fix
  // doesn't require a fresh company batch before contacts appear.
  let contactJobsRequeued = 0;
  for (const company of companies.listByAudience(id)) {
    if (contacts.listByCompany(company.id).length > 0) continue;
    jobs.requeue(audience.projectId, "find_contacts", { companyId: company.id });
    contactJobsRequeued += 1;
  }

  jobs.enqueue(audience.projectId, "find_companies", { audienceId: id });
  return c.json({
    queued: "find_companies",
    contactJobsRequeued,
    searchPage,
    nextSearchPageAfter: searchPage + 1,
    note:
      `Searching Google page ${searchPage} (not repeating the previous page). ` +
      `Already-saved company domains are skipped.` +
      (contactJobsRequeued > 0
        ? ` Re-queued contact discovery for ${contactJobsRequeued} compan${contactJobsRequeued === 1 ? "y" : "ies"} with no people yet.`
        : "") +
      (verify.allow
        ? ""
        : ` Email verify is paused (${verify.kind}) — company search still runs.`),
    verification: {
      allow: verify.allow,
      kind: verify.kind ?? null,
      reason: verify.reason ?? null,
    },
  });
});

/* ------------------------------------------------------ companies/contacts */

api.get("/audiences/:id/companies", (c) =>
  c.json({ companies: companies.listByAudience(parseId(c.req.param("id"))) }),
);

function contactPageOptions(c: Context) {
  const limitRaw = Number.parseInt(c.req.query("limit") ?? "", 10);
  const beforeRaw = Number.parseInt(c.req.query("beforeId") ?? "", 10);
  const projectRaw = c.req.query("projectId");
  const projectIdRaw = Number.parseInt(projectRaw ?? "", 10);
  const workflowRaw = c.req.query("workflowType");
  const workflowParsed = workflowRaw ? workflowTypeSchema.safeParse(workflowRaw) : null;
  if (workflowRaw && !workflowParsed?.success) {
    return { error: "invalid workflow type" as const };
  }
  return {
    limit: Number.isFinite(limitRaw) ? limitRaw : 50,
    beforeId: Number.isFinite(beforeRaw) ? beforeRaw : undefined,
    projectId: Number.isFinite(projectIdRaw) ? projectIdRaw : undefined,
    workflowType: workflowParsed?.success ? workflowParsed.data : undefined,
    query: c.req.query("q") ?? undefined,
  };
}

api.get("/contacts", (c) => {
  const opts = contactPageOptions(c);
  if ("error" in opts) return c.json({ error: opts.error }, 400);
  return c.json(contacts.pageAll(opts));
});

api.get("/projects/:id/contacts", (c) => {
  const id = parseId(c.req.param("id"));
  const opts = contactPageOptions(c);
  if ("error" in opts) return c.json({ error: opts.error }, 400);
  return c.json(
    contacts.page(id, {
      limit: opts.limit,
      beforeId: opts.beforeId,
      query: opts.query,
    }),
  );
});

const manualContactSchema = z.object({
  email: z.string().trim().min(1),
  fullName: z.string().trim().optional(),
  title: z.string().trim().optional(),
  companyName: z.string().trim().optional(),
  companyDomain: z.string().trim().optional(),
  country: z.string().trim().optional(),
  evidenceUrl: z.string().trim().optional(),
  audienceId: z.number().int().positive().nullable().optional(),
});

api.post("/projects/:id/contacts", async (c) => {
  const id = parseId(c.req.param("id"));
  if (!projects.get(id)) return c.json({ error: "not found" }, 404);

  const parsed = manualContactSchema.safeParse(await c.req.json().catch(() => ({})));
  if (!parsed.success) {
    return c.json({ error: parsed.error.issues[0]?.message ?? "invalid contact" }, 400);
  }

  const result = addManualContact(id, parsed.data, "manual");
  if (result.status === "error") return c.json({ error: result.reason }, 400);
  if (result.status === "skipped") {
    return c.json({ error: result.reason, email: result.email }, 409);
  }
  return c.json({ contact: result.contact, drafted: true }, 201);
});

api.post("/projects/:id/contacts/import", async (c) => {
  const id = parseId(c.req.param("id"));
  if (!projects.get(id)) return c.json({ error: "not found" }, 404);

  const body = await c.req.json().catch(() => ({}));
  const parsed = z
    .object({
      audienceId: z.number().int().positive().optional(),
      csv: z.string().optional(),
      contacts: z.array(manualContactSchema).optional(),
    })
    .safeParse(body);
  if (!parsed.success) {
    return c.json({ error: parsed.error.issues[0]?.message ?? "invalid import" }, 400);
  }

  let rows = parsed.data.contacts ?? [];
  if (parsed.data.csv?.trim()) {
    rows = [...rows, ...parseContactsCsv(parsed.data.csv)];
  }
  if (rows.length === 0) {
    return c.json({ error: "Provide a CSV string or a contacts array" }, 400);
  }

  const withAudience = rows.map((row) => ({
    ...row,
    audienceId: row.audienceId ?? parsed.data.audienceId,
  }));

  try {
    const result = addManualContacts(id, withAudience, "imported");
    return c.json({
      created: result.created,
      skipped: result.skipped,
      errors: result.errors,
      contacts: result.contacts,
      drafted: result.created > 0,
    });
  } catch (error) {
    return c.json(
      { error: error instanceof Error ? error.message : "Import failed" },
      400,
    );
  }
});

api.patch("/contacts/:id", async (c) => {
  const id = parseId(c.req.param("id"));
  if (!contacts.get(id)) return c.json({ error: "not found" }, 404);

  const parsed = manualContactSchema.safeParse(await c.req.json().catch(() => ({})));
  if (!parsed.success) {
    return c.json({ error: parsed.error.issues[0]?.message ?? "invalid contact" }, 400);
  }

  const result = updateManualContact(id, parsed.data);
  if (result.status === "error") return c.json({ error: result.reason }, 400);
  if (result.status !== "updated") return c.json({ error: "update failed" }, 400);
  return c.json({ contact: result.contact });
});

api.delete("/contacts/:id", (c) => {
  const id = parseId(c.req.param("id"));
  const contact = contacts.get(id);
  if (!contact) return c.json({ error: "not found" }, 404);
  contacts.remove(id);
  return c.json({ deleted: true });
});

api.post("/contacts/:id/draft", (c) => {
  const id = parseId(c.req.param("id"));
  const contact = contacts.get(id);
  if (!contact) return c.json({ error: "not found" }, 404);
  jobs.enqueue(contact.project_id, "draft_message", { contactId: id, forceNew: true });
  return c.json({ queued: "draft_message" });
});

function contactsCsvResponse(
  rows: ReturnType<typeof contacts.listForExport>,
  filenameSeed: string,
) {
  const header = [
    "email",
    "full_name",
    "title",
    "company_name",
    "company_domain",
    "audience_name",
    "campaign_name",
    "playbook",
    "email_source",
    "verify_status",
    "verify_score",
    "evidence_url",
    "country",
    "created_at",
  ];
  const escape = (value: string | number | null | undefined) => {
    const text = value == null ? "" : String(value);
    if (/[",\n\r]/.test(text)) return `"${text.replace(/"/g, '""')}"`;
    return text;
  };
  const lines = [
    header.join(","),
    ...rows.map((row) =>
      [
        row.email,
        row.full_name,
        row.title,
        row.company_name,
        row.company_domain,
        row.audience_name,
        row.project_name,
        row.workflow_type,
        row.email_source,
        row.verify_status,
        row.verify_score,
        row.evidence_url,
        row.country,
        row.created_at,
      ]
        .map(escape)
        .join(","),
    ),
  ];
  // UTF-8 BOM so Excel opens international characters correctly.
  const csv = `\uFEFF${lines.join("\n")}\n`;
  const stamp = new Date().toISOString().slice(0, 10);
  const filename = `${filenameSeed.replace(/[^a-z0-9.-]+/gi, "_")}-contacts-${stamp}.csv`;

  return new Response(csv, {
    headers: {
      "Content-Type": "text/csv; charset=utf-8",
      "Content-Disposition": `attachment; filename="${filename}"`,
      "Cache-Control": "no-store",
    },
  });
}

api.get("/contacts/export.csv", (c) => {
  const opts = contactPageOptions(c);
  if ("error" in opts) return c.json({ error: opts.error }, 400);
  const rows = contacts.listForExport({
    projectId: opts.projectId,
    workflowType: opts.workflowType,
    query: opts.query,
  });
  const seed =
    typeof opts.projectId === "number"
      ? (projects.get(opts.projectId)?.domain ?? `campaign-${opts.projectId}`)
      : opts.workflowType
        ? opts.workflowType.replace(/_/g, "-")
        : "all-playbooks";
  return contactsCsvResponse(rows, seed);
});

api.get("/projects/:id/contacts/export.csv", (c) => {
  const id = parseId(c.req.param("id"));
  const project = projects.get(id);
  if (!project) return c.json({ error: "not found" }, 404);
  return contactsCsvResponse(contacts.listForExport({ projectId: id }), project.domain);
});

/* --------------------------------------------------------------- messages */

api.get("/projects/:id/messages", (c) => {
  const id = parseId(c.req.param("id"));
  // Drop ghost drafts for people already emailed so Draft/Approve-all stay clean.
  messages.removeUnsentWhereAlreadySent(id);
  const status = c.req.query("status");
  const parsed = z
    .enum(["draft", "approved", "sending", "sent", "failed", "skipped"])
    .optional()
    .safeParse(status || undefined);
  const rows = messages.listByProject(id, parsed.success ? parsed.data : undefined);
  const sendJobs = jobs.activeSendsByMessage(id);
  return c.json({
    messages: rows.map((row) => {
      const job = sendJobs.get(row.id as number);
      if (!job) return { ...row, sendSchedule: null };
      return {
        ...row,
        sendSchedule: {
          state: job.status === "running" ? "sending" : "scheduled",
          runAfter: job.runAfter,
          reason: job.lastError,
        },
      };
    }),
  });
});

api.patch("/messages/:id", async (c) => {
  const id = parseId(c.req.param("id"));
  const parsed = z
    .object({
      subject: z.string().trim().min(1),
      body: z.string().trim().min(1),
      htmlBody: z.string().optional().default(""),
    })
    .safeParse(await c.req.json().catch(() => ({})));
  if (!parsed.success) return c.json({ error: "subject and body are required" }, 400);

  const message = messages.get(id);
  if (!message) return c.json({ error: "not found" }, 404);
  if (message.status === "sent") return c.json({ error: "already sent" }, 409);

  messages.updateContent(
    id,
    parsed.data.subject,
    normalizeEmailBody(parsed.data.body),
    parsed.data.htmlBody.trim(),
  );
  return c.json({ message: messages.get(id) });
});

api.post("/messages/:id/approve", (c) => {
  const id = parseId(c.req.param("id"));
  const message = messages.get(id);
  if (!message) return c.json({ error: "not found" }, 404);
  if (message.status === "sent") return c.json({ error: "already sent" }, 409);

  messages.setStatus(id, "approved", { approved_at: new Date().toISOString() });
  jobs.enqueue(message.project_id, "send_message", { messageId: id });
  return c.json({ message: messages.get(id) });
});

api.post("/messages/:id/reject", async (c) => {
  const id = parseId(c.req.param("id"));
  const message = messages.get(id);
  if (!message) return c.json({ error: "not found" }, 404);
  const body = await c.req.json().catch(() => ({}) as { suppress?: boolean });

  messages.setStatus(id, "skipped", { error: "rejected in review" });
  if (body.suppress) {
    const contact = contacts.get(message.contact_id);
    if (contact) suppressions.add(contact.email, null, "rejected in review", message.project_id);
  }
  return c.json({ message: messages.get(id) });
});

api.post("/projects/:id/messages/approve-all", (c) => {
  const id = parseId(c.req.param("id"));
  messages.removeUnsentWhereAlreadySent(id);
  const drafts = messages.listByProject(id, "draft") as Array<{ id: number }>;
  for (const draft of drafts) {
    messages.setStatus(draft.id, "approved", { approved_at: new Date().toISOString() });
    jobs.enqueue(id, "send_message", { messageId: draft.id });
  }
  return c.json({ approved: drafts.length });
});

/**
 * Rebuild drafts for every contact from the current audience templates.
 * Replaces unsent messages; leaves already-sent mail alone.
 */
api.post("/projects/:id/messages/redraft-all", (c) => {
  const id = parseId(c.req.param("id"));
  if (!projects.get(id)) return c.json({ error: "not found" }, 404);

  const rows = contacts.listByProject(id, 5000);
  let queued = 0;
  for (const contact of rows) {
    if (contact.verify_status === "invalid") continue;
    if (messages.hasSentForContact(contact.id)) continue;
    if (!contact.full_name.trim() && contact.email_source !== "manual" && contact.email_source !== "imported") {
      continue;
    }
    jobs.requeue(id, "draft_message", { contactId: contact.id, forceNew: true });
    queued += 1;
  }

  return c.json({
    queued,
    note:
      queued > 0
        ? `Queued redraft for ${queued} contact(s) from their audience templates.`
        : "No contacts eligible to redraft.",
  });
});

/* -------------------------------------------------------- ops + suppression */

api.get("/projects/:id/events", (c) => {
  const id = parseId(c.req.param("id"));
  const limitRaw = Number.parseInt(c.req.query("limit") ?? "", 10);
  const beforeRaw = Number.parseInt(c.req.query("beforeId") ?? "", 10);
  const page = events.page(id, {
    limit: Number.isFinite(limitRaw) ? limitRaw : 50,
    beforeId: Number.isFinite(beforeRaw) ? beforeRaw : undefined,
  });
  return c.json(page);
});

api.post("/projects/:id/jobs/retry", async (c) => {
  const id = parseId(c.req.param("id"));
  if (!projects.get(id)) return c.json({ error: "not found" }, 404);

  const parsed = z
    .object({
      jobIds: z.array(z.number().int().positive()).max(200).optional(),
      stage: z
        .enum([
          "scan_site",
          "generate_audiences",
          "generate_audience_email",
          "regenerate_audience",
          "find_companies",
          "find_contacts",
          "verify_contact",
          "draft_message",
          "send_message",
        ])
        .optional(),
    })
    .safeParse(await c.req.json().catch(() => ({})));

  if (!parsed.success) {
    return c.json({ error: "Invalid retry request" }, 400);
  }

  const retried = jobs.retryFailed(id, parsed.data);
  return c.json({
    retried,
    scope: parsed.data.jobIds?.length
      ? "selected"
      : parsed.data.stage
        ? "stage"
        : "all_failed",
  });
});

/**
 * After topping up Reoon: lift the local quota pause and queue verify jobs for
 * any contacts still sitting as unverified.
 */
api.post("/projects/:id/verify/resume", (c) => {
  const id = parseId(c.req.param("id"));
  if (!projects.get(id)) return c.json({ error: "not found" }, 404);

  const cleared = clearVerifyQuotaPause(id);
  const gate = verificationGate(id);
  if (!gate.allow) {
    return c.json(
      {
        error: gate.reason ?? "Verification still paused",
        cleared,
        verification: gate,
      },
      400,
    );
  }

  const unverified = contacts
    .listByProject(id, 2000)
    .filter((row) => row.verify_status === "unverified");
  let queued = 0;
  for (const contact of unverified) {
    jobs.requeue(id, "verify_contact", { contactId: contact.id });
    queued += 1;
  }

  return c.json({
    cleared,
    queued,
    verification: gate,
    note:
      queued > 0
        ? `Queued verify for ${queued} unverified contact(s).`
        : "Quota pause cleared. No unverified contacts to verify.",
  });
});

api.post("/projects/:id/jobs/dismiss", async (c) => {
  const id = parseId(c.req.param("id"));
  if (!projects.get(id)) return c.json({ error: "not found" }, 404);

  const parsed = z
    .object({
      jobIds: z.array(z.number().int().positive()).max(200).optional(),
      all: z.boolean().optional(),
    })
    .safeParse(await c.req.json().catch(() => ({})));

  if (!parsed.success) {
    return c.json({ error: "Invalid dismiss request" }, 400);
  }
  if (!parsed.data.jobIds?.length && !parsed.data.all) {
    return c.json({ error: "Provide jobIds or all: true" }, 400);
  }

  const dismissed = jobs.dismissFailed(id, parsed.data);
  return c.json({
    dismissed,
    scope: parsed.data.all ? "all_failed" : "selected",
  });
});

api.post("/projects/:id/jobs/cancel", async (c) => {
  const id = parseId(c.req.param("id"));
  if (!projects.get(id)) return c.json({ error: "not found" }, 404);

  const parsed = z
    .object({
      scope: z.enum(["setup", "prospecting", "send", "all"]).default("all"),
    })
    .safeParse(await c.req.json().catch(() => ({})));

  if (!parsed.success) {
    return c.json({ error: "Invalid cancel request" }, 400);
  }

  const result = cancelProjectJobs(id, parsed.data.scope as CancelScope);
  return c.json(result);
});

api.get("/suppressions", (c) => c.json({ suppressions: suppressions.list() }));

api.post("/suppressions", async (c) => {
  const parsed = z
    .object({
      email: z.string().trim().email().optional(),
      domain: z.string().trim().optional(),
      reason: z.string().default("manual"),
    })
    .refine((value) => value.email || value.domain, "email or domain is required")
    .safeParse(await c.req.json().catch(() => ({})));
  if (!parsed.success) return c.json({ error: "email or domain is required" }, 400);

  suppressions.add(parsed.data.email ?? null, parsed.data.domain ?? null, parsed.data.reason);
  return c.json({ added: true }, 201);
});
