export type EmailFormat = "html" | "plain";
export type WorkflowType = "customer_outreach" | "investor_outreach" | "job_outreach";

export type ProjectRow = {
  id: number;
  name: string;
  domain: string;
  workflow_type: WorkflowType;
  workflow_data: string;
  email_format: EmailFormat;
  created_at: string;
};

export type SiteProfileRow = {
  id: number;
  project_id: number;
  pages_json: string;
  analysis: string | null;
  created_at: string;
};

export type AudienceRow = {
  id: number;
  project_id: number;
  name: string;
  description: string;
  target_roles: string;
  job_titles: string;
  industries: string;
  company_sizes: string;
  source_adapters: string;
  funded_after_year: number;
  search_queries: string;
  pain_points: string;
  value_prop: string;
  template_subject: string;
  template_body: string;
  template_html: string;
  next_search_page: number;
  origin: string;
  enabled: number;
  created_at: string;
  updated_at: string;
};

export type CompanyRow = {
  id: number;
  project_id: number;
  audience_id: number;
  name: string;
  domain: string;
  description: string;
  size_hint: string;
  country: string;
  source_url: string;
  created_at: string;
};

export type ContactRow = {
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
  verify_score: number | null;
  created_at: string;
};

export type DomainEmailPatternRow = {
  id: number;
  domain: string;
  pattern_key: string;
  hit_count: number;
  last_seen_at: string;
  created_at: string;
};

export type MessageStatus =
  | "draft"
  | "approved"
  | "sending"
  | "sent"
  | "failed"
  | "skipped";

export type MessageRow = {
  id: number;
  project_id: number;
  contact_id: number;
  audience_id: number;
  subject: string;
  body: string;
  html_body: string;
  email_format: EmailFormat | "";
  sent_text_body: string;
  sent_html_body: string;
  status: MessageStatus;
  model: string;
  personalisation: string;
  unsubscribe_token: string;
  provider_message_id: string | null;
  error: string | null;
  approved_at: string | null;
  sent_at: string | null;
  created_at: string;
};

export type JobStage =
  | "scan_site"
  | "generate_audiences"
  | "generate_audience_email"
  | "regenerate_audience"
  | "find_companies"
  | "find_contacts"
  | "verify_contact"
  | "draft_message"
  | "send_message";

export type JobRow = {
  id: number;
  project_id: number;
  stage: JobStage;
  payload: string;
  status: "pending" | "running" | "done" | "failed" | "cancelled";
  attempts: number;
  max_attempts: number;
  last_error: string | null;
  dismissed_at: string | null;
  run_after: string;
  created_at: string;
  updated_at: string;
};

/** An audience with its JSON columns parsed, as the API and UI see it. */
export type Audience = {
  id: number;
  projectId: number;
  name: string;
  description: string;
  /**
   * Jobs playbook: roles the candidate wants (drives job/employer search).
   * Empty for customer/investor audiences.
   */
  targetRoles: string[];
  /**
   * People to find via LinkedIn/people search.
   * Customer = buyers; investor = partners; jobs = hiring contacts.
   */
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
  /** Apex domain when known, otherwise "". */
  domain: string;
};

export type PricingPlan = {
  name: string;
  price: string;
  /** Free plan, trial length, annual billing, seat limits, etc. */
  note: string;
  /** What makes this plan different / what's included. */
  highlights: string[];
  /** e.g. "14-day free trial", "Free forever", "No free trial". */
  trial: string;
};

export type UseCase = {
  title: string;
  description: string;
  /** Who typically needs this use case — helps audience generation. */
  audienceHint: string;
};

export type SiteAnalysis = {
  productName: string;
  oneLiner: string;
  features: string[];
  painPointsSolved: string[];
  /** Freeform summary kept for prompts and legacy rows. */
  pricing: string;
  pricingPlans: PricingPlan[];
  competitors: Competitor[];
  platforms: string[];
  useCases: UseCase[];
};
