export type Capabilities = {
  llm: boolean;
  crawler: boolean;
  search: boolean;
  verify: boolean;
  sender: string;
  sendingLive: boolean;
};

export type AppSettings = {
  blockedCountries: string[];
  blockFreemail: boolean;
  brandLogoUrl: string;
  brandProductUrl: string;
  brandPrimaryColor: string;
  dailySendCap: number;
  minMinutesBetweenSends: number;
  sendWindowStartHour: number;
  sendWindowEndHour: number;
  sendWindowTimezone: string;
  sendOnWeekends: boolean;
  includeUnsubscribeLink: boolean;
  unsubscribeBaseUrl: string;
  localizeEmails: boolean;
  targetMarkets: string[];
  targetCompanySignals: string[];
  maxContactsPerCompany: number;
  crawlMaxPages: number;
};

export type Health = {
  capabilities: Capabilities;
  sender: {
    provider: string;
    fromEmail: string;
    fromName: string;
    dailyCap: number;
    window: string;
    sendOnWeekends: boolean;
    minMinutesBetweenSends: number;
  };
  prospecting: {
    maxContactsPerCompany: number;
    targetMarkets: string[];
    targetCompanySignals: string[];
    localizeEmails: boolean;
  };
  blockers: string[];
};

export type EmailFormat = "html" | "plain";
export type WorkflowType = "customer_outreach" | "investor_outreach" | "job_outreach";

export type Project = {
  id: number;
  name: string;
  domain: string;
  workflow_type: WorkflowType;
  workflow_data: string;
  email_format: EmailFormat;
  created_at: string;
};

export type EmailPreviewResult = {
  format: EmailFormat;
  subject: string;
  text: string;
  html: string | null;
  htmlIsCustom: boolean;
  brand: {
    logoUrl: string | null;
    productUrl: string;
    primaryColor: string;
    logoMode: "image" | "text";
  };
};

export type Audience = {
  id: number;
  projectId: number;
  name: string;
  description: string;
  /** Jobs: roles the candidate wants. Empty for customer/investor. */
  targetRoles: string[];
  /** People to contact (buyers / investors / hiring contacts). */
  jobTitles: string[];
  industries: string[];
  companySizes: string[];
  /** Empty = auto from campaign signals. Includes "web" for Serper. */
  sourceAdapters: string[];
  /** 0 = no filter. Otherwise prefer batch/launch year >= this. */
  fundedAfterYear: number;
  searchQueries: string[];
  painPoints: string[];
  valueProp: string;
  templateSubject: string;
  templateBody: string;
  templateHtml: string;
  nextSearchPage: number;
  origin: string;
  enabled: boolean;
};

export type Competitor = {
  name: string;
  domain: string;
};

export type PricingPlan = {
  name: string;
  price: string;
  note: string;
  highlights: string[];
  trial: string;
};

export type UseCase = {
  title: string;
  description: string;
  audienceHint: string;
};

export type SiteAnalysis = {
  productName: string;
  oneLiner: string;
  features: string[];
  painPointsSolved: string[];
  pricing: string;
  pricingPlans: PricingPlan[];
  competitors: Competitor[];
  platforms: string[];
  useCases: UseCase[];
};

export type PipelineJob = {
  id: number;
  stage: string;
  label: string;
  target: string;
  status: "pending" | "running" | "done" | "failed" | "cancelled";
  attempts: number;
  maxAttempts: number;
  lastError: string | null;
  runAfter: string;
  updatedAt: string;
};

export type PipelineSnapshot = {
  active: PipelineJob[];
  failed: PipelineJob[];
  failedCount: number;
  pendingJobs: number;
};

export type PlaybookPreferences =
  | {
      kind: "customer_outreach";
      targetMarkets: string[];
      targetCompanySignals: string[];
      maxContactsPerCompany: number;
      crawlMaxPages: number;
      localizeEmails: boolean;
    }
  | {
      kind: "investor_outreach";
      investorTypes: string[];
      thesisSignals: string[];
      maxContactsPerFirm: number;
      localizeEmails: boolean;
    }
  | {
      kind: "job_outreach";
      companySizes: string[];
      industries: string[];
      maxContactsPerCompany: number;
      localizeEmails: boolean;
      /** When true, each contact email is written from the JD. When false, lane template is rendered. */
      tailorEmailsToJobListing?: boolean;
    };

export type Overview = {
  project: Project;
  preferences?: PlaybookPreferences;
  analysis: SiteAnalysis | null;
  pagesCrawled: number;
  audiences: Audience[];
  counts: {
    companies: number;
    contacts: number;
    sendableUnsentContacts: number;
    messages: Record<string, number>;
    sentTotal: number;
    sentLast24h: number;
    pendingJobs: number;
    failedJobs?: number;
  };
  jobs: Array<{ stage: string; status: string; n: number; label: string; lastError: string | null }>;
  pipeline?: PipelineSnapshot;
  verification?: {
    allow: boolean;
    kind: "quota" | null;
    reason: string | null;
  };
  estimatedSpendUsd: number;
};

export type Contact = {
  id: number;
  project_id: number;
  audience_id: number;
  company_id: number;
  full_name: string;
  title: string;
  email: string;
  email_source: string;
  evidence_url: string;
  country: string;
  verify_status: string;
  created_at: string;
  company_name: string;
  company_domain: string;
  /** Job listing / careers discovery URL when available. */
  company_source_url?: string;
  audience_name: string;
  project_name: string;
  workflow_type: WorkflowType;
};

export type ContactsPage = {
  contacts: Contact[];
  total: number;
  nextBeforeId: number | null;
};

export type SendSchedule = {
  state: "scheduled" | "sending";
  runAfter: string;
  reason: string | null;
};

export type Message = {
  id: number;
  contact_id: number;
  subject: string;
  body: string;
  html_body: string;
  email_format: EmailFormat | "";
  sent_text_body: string;
  sent_html_body: string;
  status: string;
  model: string;
  personalisation: string;
  error: string | null;
  sent_at: string | null;
  created_at: string;
  email: string;
  full_name: string;
  title: string;
  verify_status: string;
  country: string;
  company_name: string;
  company_domain: string;
  /** Job listing / careers discovery URL when available. */
  company_source_url?: string;
  audience_name: string;
  sendSchedule: SendSchedule | null;
};

export type ActivityEvent = {
  id: number;
  kind: string;
  ref: string;
  data: string;
  cost_usd: number;
  created_at: string;
};

export type EventsPage = {
  events: ActivityEvent[];
  total: number;
  nextBeforeId: number | null;
};

export type Suppression = {
  email: string | null;
  domain: string | null;
  reason: string;
  created_at: string;
};

export type RetryJobsResult = {
  retried: number;
  scope: "selected" | "stage" | "all_failed";
};

export type DismissJobsResult = {
  dismissed: number;
  scope: "selected" | "all_failed";
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  const text = await res.text();
  const payload = text ? JSON.parse(text) : {};
  if (!res.ok) throw new Error(payload.error ?? `Request failed (${res.status})`);
  return payload as T;
}

const post = <T,>(path: string, body?: unknown) =>
  request<T>(path, { method: "POST", body: body ? JSON.stringify(body) : undefined });

export const api = {
  health: () => request<Health>("/health"),
  settings: () => request<{ settings: AppSettings }>("/settings"),
  updateSettings: (body: Partial<AppSettings>) =>
    request<{ settings: AppSettings }>("/settings", {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  listProjects: () => request<{ projects: Project[] }>("/projects"),
  createProject: (body: {
    workflowType?: WorkflowType;
    domain?: string;
    name?: string;
    emailFormat?: EmailFormat;
    raiseStage?: string;
    checkSize?: string;
    geography?: string;
    sectors?: string[];
    traction?: string;
    fundraisingNotes?: string;
    resumeText?: string;
    targetRoles?: string[];
    locations?: string[];
    remotePreference?: string;
    seniority?: string;
    jobSearchNotes?: string;
    linkedinUrl?: string;
    resumeUrl?: string;
    preferences?: Record<string, unknown>;
  }) =>
    post<{ project: Project; preferences: PlaybookPreferences }>("/projects", body),
  updateProject: (
    id: number,
    body: {
      preferences?: Record<string, unknown>;
      linkedinUrl?: string;
      resumeUrl?: string;
    },
  ) =>
    request<{ project: Project; preferences?: PlaybookPreferences }>(`/projects/${id}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  deleteProject: (id: number) => request<{ deleted: boolean }>(`/projects/${id}`, { method: "DELETE" }),
  emailPreview: (
    id: number,
    body: {
      subject?: string;
      body: string;
      htmlBody?: string;
      fillSampleMergeFields?: boolean;
      painPoint?: string;
      valueProp?: string;
    },
  ) => post<EmailPreviewResult>(`/projects/${id}/email-preview`, body),
  overview: (id: number) => request<Overview>(`/projects/${id}`),
  rescan: (
    id: number,
    opts?: { regenerateAudiences?: boolean; preserveProspecting?: boolean },
  ) =>
    post<{
      queued: string;
      regenerateAudiences: boolean;
      preserveProspecting: boolean;
    }>(`/projects/${id}/rescan`, opts ?? {}),
  regenerateAudiences: (id: number, opts?: { preserveProspecting?: boolean }) =>
    post<{ queued: string; preserveProspecting: boolean }>(
      `/projects/${id}/audiences/regenerate`,
      opts ?? {},
    ),
  /** Refresh one ICP in place, then rewrite its email template. */
  regenerateAudience: (audienceId: number) =>
    post<{ queued: string; audienceId: number }>(`/audiences/${audienceId}/regenerate`),
  /** Rewrite only the cold-email template for one audience. */
  regenerateAudienceEmail: (audienceId: number) =>
    post<{ queued: string; audienceId: number }>(
      `/audiences/${audienceId}/regenerate-email`,
    ),
  updateAnalysis: (id: number, body: SiteAnalysis) =>
    request<{ analysis: SiteAnalysis }>(`/projects/${id}/analysis`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),

  createAudience: (projectId: number, body: Partial<Audience>) =>
    post<{ audience: Audience }>(`/projects/${projectId}/audiences`, body),
  listAudiences: (projectId: number) =>
    request<{ audiences: Audience[] }>(`/projects/${projectId}/audiences`),
  updateAudience: (id: number, body: Partial<Audience>) =>
    request<{ audience: Audience }>(`/audiences/${id}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  deleteAudience: (id: number) => request(`/audiences/${id}`, { method: "DELETE" }),
  prospect: (id: number) =>
    post<{
      queued: string;
      searchPage: number;
      nextSearchPageAfter: number;
      note: string;
      verification: {
        allow: boolean;
        kind: "quota" | null;
        reason: string | null;
      };
    }>(`/audiences/${id}/prospect`),

  contacts: (opts?: {
    projectId?: number;
    workflowType?: WorkflowType;
    limit?: number;
    beforeId?: number;
    q?: string;
  }) => {
    const params = new URLSearchParams();
    if (opts?.projectId) params.set("projectId", String(opts.projectId));
    if (opts?.workflowType) params.set("workflowType", opts.workflowType);
    if (opts?.limit) params.set("limit", String(opts.limit));
    if (opts?.beforeId) params.set("beforeId", String(opts.beforeId));
    if (opts?.q?.trim()) params.set("q", opts.q.trim());
    const query = params.toString();
    return request<ContactsPage>(`/contacts${query ? `?${query}` : ""}`);
  },
  createContact: (
    projectId: number,
    body: {
      email: string;
      fullName?: string;
      title?: string;
      companyName?: string;
      companyDomain?: string;
      country?: string;
      evidenceUrl?: string;
      audienceId?: number;
    },
  ) => post<{ contact: Contact; drafted: boolean }>(`/projects/${projectId}/contacts`, body),
  importContacts: (
    projectId: number,
    body: {
      audienceId?: number;
      csv?: string;
      contacts?: Array<{
        email: string;
        fullName?: string;
        title?: string;
        companyName?: string;
        companyDomain?: string;
        country?: string;
        evidenceUrl?: string;
        audienceId?: number;
      }>;
    },
  ) =>
    post<{
      created: number;
      skipped: number;
      errors: Array<{ email: string; reason: string }>;
      contacts: Contact[];
      drafted: boolean;
    }>(`/projects/${projectId}/contacts/import`, body),
  updateContact: (
    id: number,
    body: {
      email: string;
      fullName?: string;
      title?: string;
      companyName?: string;
      companyDomain?: string;
      country?: string;
      evidenceUrl?: string;
      audienceId?: number | null;
    },
  ) =>
    request<{ contact: Contact }>(`/contacts/${id}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  deleteContact: (id: number) => request<{ deleted: boolean }>(`/contacts/${id}`, { method: "DELETE" }),
  draftContactAgain: (id: number) => post<{ queued: string }>(`/contacts/${id}/draft`),
  exportContactsCsvUrl: (opts?: {
    projectId?: number;
    workflowType?: WorkflowType;
    q?: string;
  }) => {
    const params = new URLSearchParams();
    if (opts?.projectId) params.set("projectId", String(opts.projectId));
    if (opts?.workflowType) params.set("workflowType", opts.workflowType);
    if (opts?.q?.trim()) params.set("q", opts.q.trim());
    const query = params.toString();
    return `/api/contacts/export.csv${query ? `?${query}` : ""}`;
  },
  messages: (projectId: number, status?: string) =>
    request<{ messages: Message[] }>(
      `/projects/${projectId}/messages${status ? `?status=${status}` : ""}`,
    ),
  updateMessage: (
    id: number,
    subject: string,
    body: string,
    htmlBody = "",
  ) =>
    request(`/messages/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ subject, body, htmlBody }),
    }),
  approve: (id: number) => post(`/messages/${id}/approve`),
  reject: (id: number, suppress: boolean) => post(`/messages/${id}/reject`, { suppress }),
  approveAll: (projectId: number) =>
    post<{ approved: number }>(`/projects/${projectId}/messages/approve-all`),
  /** Rebuild every contact draft from current audience templates (replaces unsent). */
  redraftAll: (projectId: number) =>
    post<{ queued: number; note: string }>(`/projects/${projectId}/messages/redraft-all`),

  events: (projectId: number, opts?: { limit?: number; beforeId?: number }) => {
    const params = new URLSearchParams();
    if (opts?.limit) params.set("limit", String(opts.limit));
    if (opts?.beforeId) params.set("beforeId", String(opts.beforeId));
    const query = params.toString();
    return request<EventsPage>(`/projects/${projectId}/events${query ? `?${query}` : ""}`);
  },
  retryJobs: (
    projectId: number,
    opts?: { jobIds?: number[]; stage?: string },
  ) => post<RetryJobsResult>(`/projects/${projectId}/jobs/retry`, opts ?? {}),
  /** After topping up Reoon: clear local quota pause and queue verify for unverified contacts. */
  resumeVerify: (projectId: number) =>
    post<{ cleared: number; queued: number; note: string }>(
      `/projects/${projectId}/verify/resume`,
    ),
  dismissJobs: (
    projectId: number,
    opts: { jobIds?: number[]; all?: boolean },
  ) => post<DismissJobsResult>(`/projects/${projectId}/jobs/dismiss`, opts),
  cancelJobs: (
    projectId: number,
    opts?: { scope?: "setup" | "prospecting" | "send" | "all" },
  ) =>
    post<{ cancelledPending: number; runningCancelled: number; scope: string }>(
      `/projects/${projectId}/jobs/cancel`,
      opts ?? { scope: "all" },
    ),
  suppressions: () => request<{ suppressions: Suppression[] }>("/suppressions"),
  suppress: (email: string) => post("/suppressions", { email, reason: "manual" }),
};
