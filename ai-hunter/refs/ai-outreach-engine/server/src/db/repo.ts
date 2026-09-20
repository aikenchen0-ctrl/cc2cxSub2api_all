import { db } from "./index";
import { normalizeEmailBody } from "../compliance/emailBody";
import type {
  Audience,
  AudienceRow,
  CompanyRow,
  ContactRow,
  DomainEmailPatternRow,
  EmailFormat,
  JobRow,
  JobStage,
  MessageRow,
  MessageStatus,
  ProjectRow,
  SiteProfileRow,
  WorkflowType,
} from "./types";

function parseList(raw: string): string[] {
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.map(String) : [];
  } catch {
    return [];
  }
}

export function toAudience(row: AudienceRow): Audience {
  return {
    id: row.id,
    projectId: row.project_id,
    name: row.name,
    description: row.description,
    targetRoles: parseList(row.target_roles ?? "[]"),
    jobTitles: parseList(row.job_titles),
    industries: parseList(row.industries),
    companySizes: parseList(row.company_sizes),
    sourceAdapters: parseList(row.source_adapters ?? "[]"),
    fundedAfterYear: Number(row.funded_after_year ?? 0) || 0,
    searchQueries: parseList(row.search_queries),
    painPoints: parseList(row.pain_points),
    valueProp: row.value_prop,
    templateSubject: row.template_subject,
    templateBody: normalizeEmailBody(row.template_body),
    templateHtml: row.template_html ?? "",
    nextSearchPage: row.next_search_page,
    origin: row.origin,
    enabled: row.enabled === 1,
  };
}

/* -------------------------------------------------------------- projects */

function normalizeEmailFormat(value: string | null | undefined): EmailFormat {
  // Product sends plain text only; keep column for legacy rows.
  return "plain";
}

function normalizeWorkflowType(value: string | null | undefined): WorkflowType {
  if (value === "investor_outreach" || value === "job_outreach") return value;
  return "customer_outreach";
}

function normalizeProject(row: ProjectRow): ProjectRow {
  return {
    ...row,
    workflow_type: normalizeWorkflowType(row.workflow_type),
    workflow_data: row.workflow_data || "{}",
    email_format: normalizeEmailFormat(row.email_format),
  };
}

export const projects = {
  create(
    name: string,
    domain: string,
    emailFormat: EmailFormat = "plain",
    workflowType: WorkflowType = "customer_outreach",
    workflowData: unknown = {},
  ): ProjectRow {
    const row = db
      .query<ProjectRow, [string, string, string, string, string]>(
        `INSERT INTO projects (name, domain, email_format, workflow_type, workflow_data)
         VALUES (?, ?, ?, ?, ?)
         ON CONFLICT (domain) DO UPDATE SET
           name = excluded.name,
           email_format = excluded.email_format,
           workflow_type = excluded.workflow_type,
           workflow_data = excluded.workflow_data
         RETURNING *`,
      )
      .get(
        name,
        domain,
        emailFormat,
        normalizeWorkflowType(workflowType),
        JSON.stringify(workflowData ?? {}),
      )!;
    return normalizeProject(row);
  },
  list(): ProjectRow[] {
    return db
      .query<ProjectRow, []>(`SELECT * FROM projects ORDER BY id DESC`)
      .all()
      .map(normalizeProject);
  },
  get(id: number): ProjectRow | null {
    const row = db.query<ProjectRow, [number]>(`SELECT * FROM projects WHERE id = ?`).get(id);
    if (!row) return null;
    return normalizeProject(row);
  },
  setEmailFormat(id: number, emailFormat: EmailFormat): ProjectRow | null {
    db.run(`UPDATE projects SET email_format = ? WHERE id = ?`, [emailFormat, id]);
    return projects.get(id);
  },
  setWorkflowData(id: number, workflowData: unknown): ProjectRow | null {
    db.run(`UPDATE projects SET workflow_data = ? WHERE id = ?`, [
      JSON.stringify(workflowData ?? {}),
      id,
    ]);
    return projects.get(id);
  },
  remove(id: number): boolean {
    const result = db.run(`DELETE FROM projects WHERE id = ?`, [id]);
    return result.changes > 0;
  },
};

/* --------------------------------------------------------- site profiles */

export const siteProfiles = {
  create(projectId: number, pages: unknown): SiteProfileRow {
    return db
      .query<SiteProfileRow, [number, string]>(
        `INSERT INTO site_profiles (project_id, pages_json) VALUES (?, ?) RETURNING *`,
      )
      .get(projectId, JSON.stringify(pages))!;
  },
  setAnalysis(id: number, analysis: unknown): void {
    db.run(`UPDATE site_profiles SET analysis = ? WHERE id = ?`, [
      JSON.stringify(analysis),
      id,
    ]);
  },
  latest(projectId: number): SiteProfileRow | null {
    return db
      .query<SiteProfileRow, [number]>(
        `SELECT * FROM site_profiles WHERE project_id = ? ORDER BY id DESC LIMIT 1`,
      )
      .get(projectId);
  },
};

/* ------------------------------------------------------------- audiences */

export type AudienceInput = {
  name: string;
  description?: string;
  targetRoles?: string[];
  jobTitles?: string[];
  industries?: string[];
  companySizes?: string[];
  sourceAdapters?: string[];
  fundedAfterYear?: number;
  searchQueries?: string[];
  painPoints?: string[];
  valueProp?: string;
  templateSubject?: string;
  templateBody?: string;
  templateHtml?: string;
  nextSearchPage?: number;
  origin?: string;
  enabled?: boolean;
};

export const audiences = {
  create(projectId: number, input: AudienceInput): Audience {
    const row = db
      .query<AudienceRow, any[]>(
        `INSERT INTO audiences
           (project_id, name, description, target_roles, job_titles, industries, company_sizes,
            source_adapters, funded_after_year, search_queries, pain_points, value_prop,
            template_subject, template_body, template_html, next_search_page, origin, enabled)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
         RETURNING *`,
      )
      .get(
        projectId,
        input.name,
        input.description ?? "",
        JSON.stringify(input.targetRoles ?? []),
        JSON.stringify(input.jobTitles ?? []),
        JSON.stringify(input.industries ?? []),
        JSON.stringify(input.companySizes ?? []),
        JSON.stringify(input.sourceAdapters ?? []),
        input.fundedAfterYear ?? 0,
        JSON.stringify(input.searchQueries ?? []),
        JSON.stringify(input.painPoints ?? []),
        input.valueProp ?? "",
        input.templateSubject ?? "",
        input.templateBody ?? "",
        input.templateHtml ?? "",
        input.nextSearchPage ?? 1,
        input.origin ?? "manual",
        input.enabled === false ? 0 : 1,
      )!;
    return toAudience(row);
  },

  update(id: number, input: Partial<AudienceInput>): Audience | null {
    const existing = db
      .query<AudienceRow, [number]>(`SELECT * FROM audiences WHERE id = ?`)
      .get(id);
    if (!existing) return null;
    const merged = { ...toAudience(existing), ...input };
    const row = db
      .query<AudienceRow, any[]>(
        `UPDATE audiences SET
           name = ?, description = ?, target_roles = ?, job_titles = ?, industries = ?,
           company_sizes = ?, source_adapters = ?, funded_after_year = ?,
           search_queries = ?, pain_points = ?, value_prop = ?,
           template_subject = ?, template_body = ?, template_html = ?,
           next_search_page = ?, enabled = ?, updated_at = datetime('now')
         WHERE id = ? RETURNING *`,
      )
      .get(
        merged.name,
        merged.description ?? "",
        JSON.stringify(merged.targetRoles ?? []),
        JSON.stringify(merged.jobTitles ?? []),
        JSON.stringify(merged.industries ?? []),
        JSON.stringify(merged.companySizes ?? []),
        JSON.stringify(merged.sourceAdapters ?? []),
        merged.fundedAfterYear ?? 0,
        JSON.stringify(merged.searchQueries ?? []),
        JSON.stringify(merged.painPoints ?? []),
        merged.valueProp ?? "",
        merged.templateSubject ?? "",
        merged.templateBody ?? "",
        merged.templateHtml ?? "",
        merged.nextSearchPage ?? 1,
        merged.enabled === false ? 0 : 1,
        id,
      )!;
    return toAudience(row);
  },

  remove(id: number): void {
    db.run(`DELETE FROM audiences WHERE id = ?`, [id]);
  },

  list(projectId: number): Audience[] {
    return db
      .query<AudienceRow, [number]>(
        `SELECT * FROM audiences WHERE project_id = ? ORDER BY id ASC`,
      )
      .all(projectId)
      .map(toAudience);
  },

  get(id: number): Audience | null {
    const row = db
      .query<AudienceRow, [number]>(`SELECT * FROM audiences WHERE id = ?`)
      .get(id);
    return row ? toAudience(row) : null;
  },

  /**
   * Prepare for a fresh generated set.
   * - preserveProspecting=true (default): keep audiences that already have
   *   companies/contacts/messages (mark retained + disable). Only empty
   *   generated suggestions are deleted.
   * - preserveProspecting=false: delete all generated audiences, which CASCADE
   *   wipes their companies, contacts, and messages.
   */
  replaceGenerated(
    projectId: number,
    opts: { preserveProspecting?: boolean } = {},
  ): void {
    const preserve = opts.preserveProspecting !== false;
    if (preserve) {
      db.run(
        `UPDATE audiences
         SET origin = 'retained',
             enabled = 0,
             updated_at = datetime('now')
         WHERE project_id = ?
           AND origin = 'generated'
           AND (
             EXISTS (SELECT 1 FROM companies c WHERE c.audience_id = audiences.id)
             OR EXISTS (SELECT 1 FROM contacts ct WHERE ct.audience_id = audiences.id)
             OR EXISTS (SELECT 1 FROM messages m WHERE m.audience_id = audiences.id)
           )`,
        [projectId],
      );
    }
    db.run(`DELETE FROM audiences WHERE project_id = ? AND origin = 'generated'`, [
      projectId,
    ]);
  },

  advanceSearchPage(id: number): void {
    db.run(
      `UPDATE audiences SET next_search_page = next_search_page + 1,
         updated_at = datetime('now')
       WHERE id = ?`,
      [id],
    );
  },
};

/* ------------------------------------------------------------- companies */

export const companies = {
  existsByDomain(projectId: number, domain: string): boolean {
    return this.getByDomain(projectId, domain) !== null;
  },

  /** Same playbook (workflow_type) across projects — skip re-prospecting. */
  existsByDomainInWorkflow(workflowType: WorkflowType | string, domain: string): boolean {
    const clean = domain.toLowerCase().replace(/^www\./, "");
    return (
      db
        .query<{ n: number }, [string, string]>(
          `SELECT COUNT(*) AS n
             FROM companies c
             JOIN projects p ON p.id = c.project_id
            WHERE p.workflow_type = ? AND c.domain = ?`,
        )
        .get(workflowType, clean)?.n ?? 0
    ) > 0;
  },

  domainsInWorkflow(workflowType: WorkflowType | string): string[] {
    return db
      .query<{ domain: string }, [string]>(
        `SELECT DISTINCT c.domain AS domain
           FROM companies c
           JOIN projects p ON p.id = c.project_id
          WHERE p.workflow_type = ?`,
      )
      .all(workflowType)
      .map((row) => row.domain.toLowerCase());
  },

  getByDomain(projectId: number, domain: string): CompanyRow | null {
    return (
      db
        .query<CompanyRow, [number, string]>(
          `SELECT * FROM companies WHERE project_id = ? AND domain = ? LIMIT 1`,
        )
        .get(projectId, domain.toLowerCase()) ?? null
    );
  },

  upsert(input: {
    projectId: number;
    audienceId: number;
    name: string;
    domain: string;
    description?: string;
    sizeHint?: string;
    country?: string;
    sourceUrl?: string;
  }): CompanyRow | null {
    if (this.existsByDomain(input.projectId, input.domain)) return null;

    return db
      .query<CompanyRow, any[]>(
        `INSERT INTO companies
           (project_id, audience_id, name, domain, description, size_hint, country, source_url)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?)
         ON CONFLICT (project_id, audience_id, domain) DO NOTHING
         RETURNING *`,
      )
      .get(
        input.projectId,
        input.audienceId,
        input.name,
        input.domain.toLowerCase(),
        input.description ?? "",
        input.sizeHint ?? "",
        input.country ?? "",
        input.sourceUrl ?? "",
      );
  },

  /** Reuse an existing company for the domain, or create one under the given audience. */
  ensure(input: {
    projectId: number;
    audienceId: number;
    name: string;
    domain: string;
    description?: string;
    sizeHint?: string;
    country?: string;
    sourceUrl?: string;
  }): CompanyRow {
    const existing = this.getByDomain(input.projectId, input.domain);
    if (existing) return existing;
    const created = this.upsert(input);
    if (created) return created;
    const raced = this.getByDomain(input.projectId, input.domain);
    if (!raced) throw new Error(`Failed to create company for ${input.domain}`);
    return raced;
  },

  listByAudience(audienceId: number): CompanyRow[] {
    return db
      .query<CompanyRow, [number]>(
        `SELECT * FROM companies WHERE audience_id = ? ORDER BY id ASC`,
      )
      .all(audienceId);
  },
  get(id: number): CompanyRow | null {
    return db.query<CompanyRow, [number]>(`SELECT * FROM companies WHERE id = ?`).get(id);
  },
  countByProject(projectId: number): number {
    return (
      db
        .query<{ n: number }, [number]>(
          `SELECT COUNT(*) AS n FROM companies WHERE project_id = ?`,
        )
        .get(projectId)?.n ?? 0
    );
  },
};

/* ---------------------------------------------------- domain email patterns */

/**
 * Global (cross-project) memory of which local-part formats work for a domain.
 * Ranked by hit_count so discovery can probe the top formats before defaults.
 */
export const domainEmailPatterns = {
  /** Top pattern keys for a domain, highest hit_count first. */
  ranked(domain: string, limit = 2): string[] {
    const normalised = domain.trim().toLowerCase();
    if (!normalised || limit <= 0) return [];
    return db
      .query<{ pattern_key: string }, [string, number]>(
        `SELECT pattern_key FROM domain_email_patterns
         WHERE domain = ?
         ORDER BY hit_count DESC, last_seen_at DESC, id DESC
         LIMIT ?`,
      )
      .all(normalised, limit)
      .map((row) => row.pattern_key);
  },

  recordHit(domain: string, patternKey: string): DomainEmailPatternRow {
    const normalised = domain.trim().toLowerCase();
    db.run(
      `INSERT INTO domain_email_patterns (domain, pattern_key, hit_count, last_seen_at)
       VALUES (?, ?, 1, datetime('now'))
       ON CONFLICT (domain, pattern_key) DO UPDATE SET
         hit_count = hit_count + 1,
         last_seen_at = datetime('now')`,
      [normalised, patternKey],
    );
    return db
      .query<DomainEmailPatternRow, [string, string]>(
        `SELECT * FROM domain_email_patterns WHERE domain = ? AND pattern_key = ?`,
      )
      .get(normalised, patternKey)!;
  },

  listForDomain(domain: string): DomainEmailPatternRow[] {
    return db
      .query<DomainEmailPatternRow, [string]>(
        `SELECT * FROM domain_email_patterns
         WHERE domain = ?
         ORDER BY hit_count DESC, last_seen_at DESC`,
      )
      .all(domain.trim().toLowerCase());
  },
};

/* -------------------------------------------------------------- contacts */

export const contacts = {
  upsert(input: {
    projectId: number;
    audienceId: number;
    companyId: number;
    fullName: string;
    title: string;
    email: string;
    emailSource: string;
    evidenceUrl?: string;
    country?: string;
  }): ContactRow | null {
    return db
      .query<ContactRow, any[]>(
        `INSERT INTO contacts
           (project_id, audience_id, company_id, full_name, title, email,
            email_source, evidence_url, country)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
         ON CONFLICT (project_id, email) DO NOTHING
         RETURNING *`,
      )
      .get(
        input.projectId,
        input.audienceId,
        input.companyId,
        input.fullName,
        input.title,
        input.email.toLowerCase(),
        input.emailSource,
        input.evidenceUrl ?? "",
        input.country ?? "",
      );
  },

  setVerification(id: number, status: string, score: number | null): void {
    db.run(`UPDATE contacts SET verify_status = ?, verify_score = ? WHERE id = ?`, [
      status,
      score,
      id,
    ]);
  },

  get(id: number): ContactRow | null {
    return db.query<ContactRow, [number]>(`SELECT * FROM contacts WHERE id = ?`).get(id);
  },

  /** Deletes the contact; messages cascade. Clears queued verify/draft jobs for it. */
  remove(id: number): boolean {
    const existing = this.get(id);
    if (!existing) return false;

    const messageIds = db
      .query<{ id: number }, [number]>(`SELECT id FROM messages WHERE contact_id = ?`)
      .all(id)
      .map((row) => row.id);

    db.run(`DELETE FROM contacts WHERE id = ?`, [id]);

    db.run(
      `DELETE FROM jobs
       WHERE status IN ('pending', 'failed')
         AND stage IN ('verify_contact', 'draft_message')
         AND payload = ?`,
      [JSON.stringify({ contactId: id })],
    );
    for (const messageId of messageIds) {
      db.run(
        `DELETE FROM jobs
         WHERE status IN ('pending', 'failed')
           AND stage = 'send_message'
           AND payload = ?`,
        [JSON.stringify({ messageId })],
      );
    }
    return true;
  },

  listByProject(projectId: number, limit = 500): ContactRow[] {
    return this.page(projectId, { limit }).contacts;
  },

  page(
    projectId: number,
    opts: { limit?: number; beforeId?: number; query?: string } = {},
  ) {
    return this.pageAll({ ...opts, projectId });
  },

  pageAll(
    opts: {
      projectId?: number;
      workflowType?: WorkflowType;
      limit?: number;
      beforeId?: number;
      query?: string;
    } = {},
  ): {
    contacts: Array<
      ContactRow & {
        company_name: string;
        company_domain: string;
        audience_name: string;
        project_name: string;
        workflow_type: WorkflowType;
      }
    >;
    total: number;
    nextBeforeId: number | null;
  } {
    type ContactPageRow = ContactRow & {
      company_name: string;
      company_domain: string;
      company_source_url: string;
      audience_name: string;
      project_name: string;
      workflow_type: WorkflowType;
    };
    const select = `SELECT c.*,
            co.name AS company_name, co.domain AS company_domain,
            co.source_url AS company_source_url,
            a.name AS audience_name,
            p.name AS project_name, p.workflow_type AS workflow_type
     FROM contacts c
     JOIN companies co ON co.id = c.company_id
     JOIN audiences a ON a.id = c.audience_id
     JOIN projects p ON p.id = c.project_id`;
    const limit = Math.min(Math.max(opts.limit ?? 50, 1), 100);
    const filterClauses: string[] = [];
    const filterParams: Array<string | number> = [];
    if (typeof opts.projectId === "number") {
      filterClauses.push("c.project_id = ?");
      filterParams.push(opts.projectId);
    }
    if (opts.workflowType) {
      filterClauses.push("p.workflow_type = ?");
      filterParams.push(opts.workflowType);
    }
    if (opts.query?.trim()) {
      const q = `%${opts.query.trim().toLowerCase()}%`;
      filterClauses.push(`(
        lower(c.email) LIKE ?
        OR lower(c.full_name) LIKE ?
        OR lower(c.title) LIKE ?
        OR lower(co.name) LIKE ?
        OR lower(a.name) LIKE ?
        OR lower(p.name) LIKE ?
      )`);
      filterParams.push(q, q, q, q, q, q);
    }
    const pageClauses = [...filterClauses];
    const pageParams = [...filterParams];
    if (typeof opts.beforeId === "number") {
      pageClauses.push("c.id < ?");
      pageParams.push(opts.beforeId);
    }
    const where = pageClauses.length ? `WHERE ${pageClauses.join(" AND ")}` : "";
    const contacts = db
      .query<ContactPageRow, any[]>(
        `${select}
         ${where}
         ORDER BY c.id DESC LIMIT ?`,
      )
      .all(...pageParams, limit)
      .map((row) => ({ ...row, workflow_type: normalizeWorkflowType(row.workflow_type) }));

    const countWhere = filterClauses.length ? `WHERE ${filterClauses.join(" AND ")}` : "";
    const total =
      db
        .query<{ n: number }, any[]>(
          `SELECT COUNT(*) AS n
           FROM contacts c
           JOIN companies co ON co.id = c.company_id
           JOIN audiences a ON a.id = c.audience_id
           JOIN projects p ON p.id = c.project_id
           ${countWhere}`,
        )
        .get(...filterParams)?.n ?? 0;
    const nextBeforeId =
      contacts.length === limit ? (contacts[contacts.length - 1]?.id ?? null) : null;

    return { contacts, total, nextBeforeId };
  },

  emailTaken(projectId: number, email: string, exceptId?: number): boolean {
    if (typeof exceptId === "number") {
      return (
        (db
          .query<{ n: number }, [number, string, number]>(
            `SELECT COUNT(*) AS n FROM contacts
             WHERE project_id = ? AND email = ? AND id != ?`,
          )
          .get(projectId, email.toLowerCase(), exceptId)?.n ?? 0) > 0
      );
    }
    return (
      (db
        .query<{ n: number }, [number, string]>(
          `SELECT COUNT(*) AS n FROM contacts WHERE project_id = ? AND email = ?`,
        )
        .get(projectId, email.toLowerCase())?.n ?? 0) > 0
    );
  },

  update(
    id: number,
    input: {
      audienceId: number;
      companyId: number;
      fullName: string;
      title: string;
      email: string;
      evidenceUrl: string;
      country: string;
    },
  ): ContactRow | null {
    return (
      db
        .query<ContactRow, any[]>(
          `UPDATE contacts SET
             audience_id = ?, company_id = ?, full_name = ?, title = ?,
             email = ?, evidence_url = ?, country = ?
           WHERE id = ?
           RETURNING *`,
        )
        .get(
          input.audienceId,
          input.companyId,
          input.fullName,
          input.title,
          input.email.toLowerCase(),
          input.evidenceUrl,
          input.country,
          id,
        ) ?? null
    );
  },

  withCompany(contact: ContactRow): ContactRow & {
    company_name: string;
    company_domain: string;
    company_source_url: string;
    audience_name: string;
    project_name: string;
    workflow_type: WorkflowType;
  } {
    const company = companies.get(contact.company_id);
    const audience = audiences.get(contact.audience_id);
    const project = projects.get(contact.project_id);
    return {
      ...contact,
      company_name: company?.name ?? "",
      company_domain: company?.domain ?? "",
      company_source_url: company?.source_url ?? "",
      audience_name: audience?.name ?? "",
      project_name: project?.name ?? "",
      workflow_type: normalizeWorkflowType(project?.workflow_type),
    };
  },

  listByCompany(companyId: number): ContactRow[] {
    return db
      .query<ContactRow, [number]>(
        `SELECT * FROM contacts WHERE company_id = ? ORDER BY id ASC`,
      )
      .all(companyId);
  },

  countByCompany(companyId: number): number {
    return (
      db
        .query<{ n: number }, [number]>(
          `SELECT COUNT(*) AS n FROM contacts WHERE company_id = ?`,
        )
        .get(companyId)?.n ?? 0
    );
  },

  countByProject(projectId: number): number {
    return (
      db
        .query<{ n: number }, [number]>(
          `SELECT COUNT(*) AS n FROM contacts WHERE project_id = ?`,
        )
        .get(projectId)?.n ?? 0
    );
  },

  sendableUnsentCount(projectId: number): number {
    return (
      db
        .query<{ n: number }, [number]>(
          `SELECT COUNT(*) AS n
           FROM contacts c
           WHERE c.project_id = ?
             AND c.verify_status IN ('valid', 'risky', 'unverified')
             AND NOT EXISTS (
               SELECT 1 FROM messages m
               WHERE m.contact_id = c.id AND m.status = 'sent'
             )`,
        )
        .get(projectId)?.n ?? 0
    );
  },

  listForExport(
    opts: {
      projectId?: number;
      workflowType?: WorkflowType;
      query?: string;
    } = {},
  ): Array<{
    email: string;
    full_name: string;
    title: string;
    company_name: string;
    company_domain: string;
    audience_name: string;
    project_name: string;
    workflow_type: WorkflowType;
    email_source: string;
    verify_status: string;
    verify_score: number | null;
    evidence_url: string;
    country: string;
    created_at: string;
  }> {
    type ExportRow = {
      email: string;
      full_name: string;
      title: string;
      company_name: string;
      company_domain: string;
      audience_name: string;
      project_name: string;
      workflow_type: WorkflowType;
      email_source: string;
      verify_status: string;
      verify_score: number | null;
      evidence_url: string;
      country: string;
      created_at: string;
    };
    const clauses: string[] = [];
    const params: Array<string | number> = [];
    if (typeof opts.projectId === "number") {
      clauses.push("c.project_id = ?");
      params.push(opts.projectId);
    }
    if (opts.workflowType) {
      clauses.push("p.workflow_type = ?");
      params.push(opts.workflowType);
    }
    if (opts.query?.trim()) {
      const q = `%${opts.query.trim().toLowerCase()}%`;
      clauses.push(`(
        lower(c.email) LIKE ?
        OR lower(c.full_name) LIKE ?
        OR lower(c.title) LIKE ?
        OR lower(co.name) LIKE ?
        OR lower(a.name) LIKE ?
        OR lower(p.name) LIKE ?
      )`);
      params.push(q, q, q, q, q, q);
    }
    const where = clauses.length ? `WHERE ${clauses.join(" AND ")}` : "";
    return db
      .query<ExportRow, any[]>(
        `SELECT c.email, c.full_name, c.title,
                co.name AS company_name, co.domain AS company_domain,
                a.name AS audience_name,
                p.name AS project_name, p.workflow_type AS workflow_type,
                c.email_source, c.verify_status, c.verify_score,
                c.evidence_url, c.country, c.created_at
         FROM contacts c
         JOIN companies co ON co.id = c.company_id
         JOIN audiences a ON a.id = c.audience_id
         JOIN projects p ON p.id = c.project_id
         ${where}
         ORDER BY c.id ASC`,
      )
      .all(...params)
      .map((row) => ({ ...row, workflow_type: normalizeWorkflowType(row.workflow_type) }));
  },
};

/* -------------------------------------------------------------- messages */

export const messages = {
  create(input: {
    projectId: number;
    contactId: number;
    audienceId: number;
    subject: string;
    body: string;
    htmlBody?: string;
    model: string;
    personalisation: string;
    unsubscribeToken: string;
  }): MessageRow {
    return db
      .query<MessageRow, any[]>(
        `INSERT INTO messages
           (project_id, contact_id, audience_id, subject, body, html_body, model,
            personalisation, unsubscribe_token)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *`,
      )
      .get(
        input.projectId,
        input.contactId,
        input.audienceId,
        input.subject,
        input.body,
        input.htmlBody ?? "",
        input.model,
        input.personalisation,
        input.unsubscribeToken,
      )!;
  },

  get(id: number): MessageRow | null {
    return db.query<MessageRow, [number]>(`SELECT * FROM messages WHERE id = ?`).get(id);
  },

  getByToken(token: string): MessageRow | null {
    return db
      .query<MessageRow, [string]>(`SELECT * FROM messages WHERE unsubscribe_token = ?`)
      .get(token);
  },

  setStatus(id: number, status: MessageStatus, patch: Partial<MessageRow> = {}): void {
    db.run(
      `UPDATE messages SET status = ?, provider_message_id = COALESCE(?, provider_message_id),
         error = ?, approved_at = COALESCE(?, approved_at), sent_at = COALESCE(?, sent_at),
         email_format = COALESCE(?, email_format),
         sent_text_body = COALESCE(?, sent_text_body),
         sent_html_body = COALESCE(?, sent_html_body)
       WHERE id = ?`,
      [
        status,
        patch.provider_message_id ?? null,
        patch.error ?? null,
        patch.approved_at ?? null,
        patch.sent_at ?? null,
        patch.email_format ?? null,
        patch.sent_text_body ?? null,
        patch.sent_html_body ?? null,
        id,
      ],
    );
  },

  updateContent(id: number, subject: string, body: string, htmlBody = ""): void {
    db.run(`UPDATE messages SET subject = ?, body = ?, html_body = ? WHERE id = ?`, [
      subject,
      body,
      htmlBody,
      id,
    ]);
  },

  existsForContact(contactId: number): boolean {
    return (
      (db
        .query<{ n: number }, [number]>(
          `SELECT COUNT(*) AS n FROM messages WHERE contact_id = ?`,
        )
        .get(contactId)?.n ?? 0) > 0
    );
  },

  hasSentForContact(contactId: number): boolean {
    return (
      (db
        .query<{ n: number }, [number]>(
          `SELECT COUNT(*) AS n FROM messages WHERE contact_id = ? AND status = 'sent'`,
        )
        .get(contactId)?.n ?? 0) > 0
    );
  },

  /**
   * Drop leftover draft/approved/etc. rows for contacts who already have a sent
   * message (e.g. after a bad redraft). Keeps Sent clean for Approve all.
   */
  removeUnsentWhereAlreadySent(projectId: number): number {
    const stale = db
      .query<{ id: number; contact_id: number }, [number]>(
        `SELECT m.id, m.contact_id
         FROM messages m
         WHERE m.project_id = ?
           AND m.status IN ('draft', 'approved', 'failed', 'skipped', 'sending')
           AND EXISTS (
             SELECT 1 FROM messages s
             WHERE s.contact_id = m.contact_id AND s.status = 'sent'
           )`,
      )
      .all(projectId);

    if (stale.length === 0) return 0;

    for (const row of stale) {
      db.run(
        `DELETE FROM jobs
         WHERE status IN ('pending', 'failed', 'running')
           AND stage = 'send_message'
           AND payload = ?`,
        [JSON.stringify({ messageId: row.id })],
      );
      db.run(
        `DELETE FROM jobs
         WHERE status IN ('pending', 'failed')
           AND stage = 'draft_message'
           AND payload IN (?, ?)`,
        [
          JSON.stringify({ contactId: row.contact_id }),
          JSON.stringify({ contactId: row.contact_id, forceNew: true }),
        ],
      );
    }

    const result = db.run(
      `DELETE FROM messages
       WHERE project_id = ?
         AND status IN ('draft', 'approved', 'failed', 'skipped', 'sending')
         AND EXISTS (
           SELECT 1 FROM messages s
           WHERE s.contact_id = messages.contact_id AND s.status = 'sent'
         )`,
      [projectId],
    );
    return result.changes;
  },

  /** Drop unsent drafts so “draft again” / bulk redraft replace copy instead of stacking rows. */
  removeUnsentForContact(contactId: number): number {
    const messageIds = db
      .query<{ id: number }, [number]>(
        `SELECT id FROM messages
         WHERE contact_id = ?
           AND status IN ('draft', 'approved', 'failed', 'skipped', 'sending')`,
      )
      .all(contactId)
      .map((row) => row.id);

    const result = db.run(
      `DELETE FROM messages
       WHERE contact_id = ?
         AND status IN ('draft', 'approved', 'failed', 'skipped', 'sending')`,
      [contactId],
    );

    for (const messageId of messageIds) {
      db.run(
        `DELETE FROM jobs
         WHERE status IN ('pending', 'failed', 'running')
           AND stage = 'send_message'
           AND payload = ?`,
        [JSON.stringify({ messageId })],
      );
    }
    db.run(
      `DELETE FROM jobs
       WHERE status IN ('pending', 'failed')
         AND stage = 'draft_message'
         AND payload = ?`,
      [JSON.stringify({ contactId })],
    );
    // Also clear forceNew payloads if any were queued with that shape.
    db.run(
      `DELETE FROM jobs
       WHERE status IN ('pending', 'failed')
         AND stage = 'draft_message'
         AND payload = ?`,
      [JSON.stringify({ contactId, forceNew: true })],
    );
    return result.changes;
  },

  listByProject(projectId: number, status?: MessageStatus) {
    const sql = `
      SELECT m.*, c.email, c.full_name, c.title, c.verify_status, c.country,
             co.name AS company_name, co.domain AS company_domain,
             co.source_url AS company_source_url, a.name AS audience_name
      FROM messages m
      JOIN contacts c ON c.id = m.contact_id
      JOIN companies co ON co.id = c.company_id
      JOIN audiences a ON a.id = m.audience_id
      WHERE m.project_id = ?${status ? ` AND m.status = ?` : ""}
      ORDER BY m.id DESC LIMIT 300`;
    return status
      ? db.query<any, [number, string]>(sql).all(projectId, status)
      : db.query<any, [number]>(sql).all(projectId);
  },

  sentInLastDay(projectId: number): number {
    return (
      db
        .query<{ n: number }, [number]>(
          `SELECT COUNT(*) AS n FROM messages
           WHERE project_id = ? AND status = 'sent'
             AND sent_at > datetime('now', '-1 day')`,
        )
        .get(projectId)?.n ?? 0
    );
  },

  totalSent(projectId: number): number {
    return (
      db
        .query<{ n: number }, [number]>(
          `SELECT COUNT(*) AS n FROM messages WHERE project_id = ? AND status = 'sent'`,
        )
        .get(projectId)?.n ?? 0
    );
  },

  lastSentAt(projectId: number): string | null {
    return (
      db
        .query<{ sent_at: string | null }, [number]>(
          `SELECT sent_at FROM messages WHERE project_id = ? AND status = 'sent'
           ORDER BY sent_at DESC LIMIT 1`,
        )
        .get(projectId)?.sent_at ?? null
    );
  },

  countsByStatus(projectId: number): Record<string, number> {
    const rows = db
      .query<{ status: string; n: number }, [number]>(
        `SELECT status, COUNT(*) AS n FROM messages WHERE project_id = ? GROUP BY status`,
      )
      .all(projectId);
    return Object.fromEntries(rows.map((r) => [r.status, r.n]));
  },
};

/* ----------------------------------------------------------- suppression */

export const suppressions = {
  add(email: string | null, domain: string | null, reason: string, projectId?: number) {
    db.run(
      `INSERT INTO suppressions (project_id, email, domain, reason) VALUES (?, ?, ?, ?)
       ON CONFLICT DO NOTHING`,
      [projectId ?? null, email?.toLowerCase() ?? null, domain?.toLowerCase() ?? null, reason],
    );
  },
  isSuppressed(email: string): boolean {
    const domain = email.split("@")[1]?.toLowerCase() ?? "";
    const hit = db
      .query<{ n: number }, [string, string]>(
        `SELECT COUNT(*) AS n FROM suppressions WHERE email = ? OR domain = ?`,
      )
      .get(email.toLowerCase(), domain);
    return (hit?.n ?? 0) > 0;
  },
  list(): Array<{ email: string | null; domain: string | null; reason: string; created_at: string }> {
    return db
      .query<any, []>(`SELECT email, domain, reason, created_at FROM suppressions ORDER BY id DESC LIMIT 200`)
      .all();
  },
};

/* ------------------------------------------------------------------ jobs */

const STALE_RUNNING_MINUTES = 15;

function recoverStaleRunning(maxAgeMinutes = STALE_RUNNING_MINUTES): number {
  const result = db.run(
    `UPDATE jobs
     SET status = 'pending',
         last_error = COALESCE(last_error, 'Recovered stale running job after worker restart or timeout'),
         run_after = datetime('now'),
         updated_at = datetime('now')
     WHERE status = 'running'
       AND updated_at <= datetime('now', ?)`,
    [`-${maxAgeMinutes} minutes`],
  );
  return result.changes;
}

/**
 * Schedule guards (weekend, send window, throttle, daily cap) defer jobs into
 * the future. After a config change / restart those delays should be rechecked
 * immediately instead of waiting out a stale timer.
 */
function releaseScheduleDeferred(): number {
  const result = db.run(
    `UPDATE jobs
     SET run_after = datetime('now'), updated_at = datetime('now')
     WHERE status = 'pending'
       AND run_after > datetime('now')
       AND (
         last_error LIKE 'weekend%'
         OR last_error LIKE 'outside the send window%'
         OR last_error LIKE 'throttled%'
         OR last_error LIKE 'daily cap%'
       )`,
  );
  return result.changes;
}

export const jobs = {
  enqueue(projectId: number, stage: JobStage, payload: object = {}, delaySeconds = 0) {
    recoverStaleRunning();
    const payloadJson = JSON.stringify(payload);
    const active = db
      .query<{ id: number }, [number, JobStage, string]>(
        `SELECT id FROM jobs
         WHERE project_id = ? AND stage = ? AND payload = ?
           AND status IN ('pending', 'running')
         LIMIT 1`,
      )
      .get(projectId, stage, payloadJson);
    if (active) return;

    db.run(
      `INSERT INTO jobs (project_id, stage, payload, run_after)
       VALUES (?, ?, ?, datetime('now', ?))`,
      [projectId, stage, payloadJson, `+${delaySeconds} seconds`],
    );
  },

  /**
   * Make sure work for this stage+payload will run, without duplicating it.
   * pending/running/done → no-op; failed → requeue that row; missing → enqueue.
   */
  ensure(projectId: number, stage: JobStage, payload: object = {}): void {
    recoverStaleRunning();
    const payloadJson = JSON.stringify(payload);
    const existing = db
      .query<JobRow, [number, JobStage, string]>(
        `SELECT * FROM jobs
         WHERE project_id = ? AND stage = ? AND payload = ?
         ORDER BY id DESC LIMIT 1`,
      )
      .get(projectId, stage, payloadJson);

    if (!existing) {
      this.enqueue(projectId, stage, payload);
      return;
    }
    if (existing.status === "pending" || existing.status === "running" || existing.status === "done") {
      return;
    }
    if (existing.status === "failed") {
      this.retryFailed(projectId, { jobIds: [existing.id] });
    }
  },

  /**
   * Queue a draft when the contact still has no message. Unlike ensure(), this
   * re-runs even if a previous draft_message job finished as a soft-skip.
   */
  ensureDraft(projectId: number, contactId: number): void {
    if (messages.existsForContact(contactId)) return;
    const payload = { contactId };
    const payloadJson = JSON.stringify(payload);
    db.run(
      `DELETE FROM jobs
       WHERE project_id = ? AND stage = 'draft_message' AND payload = ?
         AND status IN ('done', 'failed')`,
      [projectId, payloadJson],
    );
    this.ensure(projectId, "draft_message", payload);
  },

  /**
   * Force a stage to run again even if a prior attempt finished as done/failed.
   * Used when find_contacts soft-succeeded with zero people and should be retried
   * after a pipeline fix — ensure() would no-op on status=done.
   */
  requeue(projectId: number, stage: JobStage, payload: object = {}): void {
    recoverStaleRunning();
    const payloadJson = JSON.stringify(payload);
    const active = db
      .query<{ id: number }, [number, JobStage, string]>(
        `SELECT id FROM jobs
         WHERE project_id = ? AND stage = ? AND payload = ?
           AND status IN ('pending', 'running')
         LIMIT 1`,
      )
      .get(projectId, stage, payloadJson);
    if (active) return;

    db.run(
      `DELETE FROM jobs
       WHERE project_id = ? AND stage = ? AND payload = ?
         AND status IN ('done', 'failed')`,
      [projectId, stage, payloadJson],
    );
    this.enqueue(projectId, stage, payload);
  },

  claimNext(): JobRow | null {
    recoverStaleRunning();
    const job = db
      .query<JobRow, []>(
        `SELECT * FROM jobs WHERE status = 'pending' AND run_after <= datetime('now')
         ORDER BY id ASC LIMIT 1`,
      )
      .get();
    if (!job) return null;
    const claimed = db.run(
      `UPDATE jobs SET status = 'running', attempts = attempts + 1,
         updated_at = datetime('now')
       WHERE id = ? AND status = 'pending'`,
      [job.id],
    );
    return claimed.changes > 0 ? { ...job, status: "running" } : null;
  },

  complete(id: number): void {
    db.run(
      `UPDATE jobs SET status = 'done', last_error = NULL, dismissed_at = NULL,
         updated_at = datetime('now') WHERE id = ?`,
      [id],
    );
  },

  markCancelled(id: number, reason = "Cancelled by user"): void {
    db.run(
      `UPDATE jobs SET status = 'cancelled', last_error = ?, updated_at = datetime('now')
       WHERE id = ? AND status IN ('pending', 'running')`,
      [reason, id],
    );
  },

  /**
   * Cancel pending work immediately. Running jobs are returned so the runner can
   * cooperatively abort them at the next checkpoint.
   */
  cancelActive(
    projectId: number,
    opts: { stages?: JobStage[] } = {},
  ): { cancelledPending: number; runningJobIds: number[] } {
    const stages = opts.stages;
    if (stages && stages.length > 0) {
      const placeholders = stages.map(() => "?").join(", ");
      const cancelledPending = db.run(
        `UPDATE jobs SET status = 'cancelled', last_error = 'Cancelled by user',
           updated_at = datetime('now')
         WHERE project_id = ? AND status = 'pending' AND stage IN (${placeholders})`,
        [projectId, ...stages],
      ).changes;
      const runningJobIds = db
        .query<{ id: number }, (number | string)[]>(
          `SELECT id FROM jobs
           WHERE project_id = ? AND status = 'running' AND stage IN (${placeholders})`,
        )
        .all(projectId, ...stages)
        .map((row) => row.id);
      return { cancelledPending, runningJobIds };
    }

    const cancelledPending = db.run(
      `UPDATE jobs SET status = 'cancelled', last_error = 'Cancelled by user',
         updated_at = datetime('now')
       WHERE project_id = ? AND status = 'pending'`,
      [projectId],
    ).changes;
    const runningJobIds = db
      .query<{ id: number }, [number]>(
        `SELECT id FROM jobs WHERE project_id = ? AND status = 'running'`,
      )
      .all(projectId)
      .map((row) => row.id);
    return { cancelledPending, runningJobIds };
  },

  fail(job: JobRow, error: string): void {
    const exhausted = job.attempts >= job.max_attempts;
    const backoffSeconds = 2 ** job.attempts * 15;
    db.run(
      `UPDATE jobs SET status = ?, last_error = ?, dismissed_at = NULL,
         run_after = datetime('now', ?), updated_at = datetime('now')
       WHERE id = ?`,
      [
        exhausted ? "failed" : "pending",
        error.slice(0, 2000),
        `+${backoffSeconds} seconds`,
        job.id,
      ],
    );
  },

  /**
   * Requeue terminal failures only. Never touches pending/running/done jobs,
   * so a retry cannot restart the whole pipeline or re-burn succeeded work.
   */
  retryFailed(
    projectId: number,
    opts: { jobIds?: number[]; stage?: string } = {},
  ): number {
    if (opts.jobIds && opts.jobIds.length > 0) {
      const placeholders = opts.jobIds.map(() => "?").join(", ");
      const result = db.run(
        `UPDATE jobs SET status = 'pending', attempts = 0, dismissed_at = NULL,
           run_after = datetime('now'), updated_at = datetime('now')
         WHERE project_id = ? AND status = 'failed' AND id IN (${placeholders})`,
        [projectId, ...opts.jobIds],
      );
      return result.changes;
    }

    if (opts.stage) {
      const result = db.run(
        `UPDATE jobs SET status = 'pending', attempts = 0, dismissed_at = NULL,
           run_after = datetime('now'), updated_at = datetime('now')
         WHERE project_id = ? AND status = 'failed' AND stage = ?
           AND dismissed_at IS NULL`,
        [projectId, opts.stage],
      );
      return result.changes;
    }

    const result = db.run(
      `UPDATE jobs SET status = 'pending', attempts = 0, dismissed_at = NULL,
         run_after = datetime('now'), updated_at = datetime('now')
       WHERE project_id = ? AND status = 'failed' AND dismissed_at IS NULL`,
      [projectId],
    );
    return result.changes;
  },

  /**
   * Hide failed jobs from Needs attention / Setup / Audiences prompts.
   * Activity log events are unchanged.
   */
  dismissFailed(
    projectId: number,
    opts: { jobIds?: number[]; all?: boolean } = {},
  ): number {
    if (opts.jobIds && opts.jobIds.length > 0) {
      const placeholders = opts.jobIds.map(() => "?").join(", ");
      const result = db.run(
        `UPDATE jobs SET dismissed_at = datetime('now'), updated_at = datetime('now')
         WHERE project_id = ? AND status = 'failed' AND dismissed_at IS NULL
           AND id IN (${placeholders})`,
        [projectId, ...opts.jobIds],
      );
      return result.changes;
    }

    if (opts.all) {
      const result = db.run(
        `UPDATE jobs SET dismissed_at = datetime('now'), updated_at = datetime('now')
         WHERE project_id = ? AND status = 'failed' AND dismissed_at IS NULL`,
        [projectId],
      );
      return result.changes;
    }

    return 0;
  },

  recoverStaleRunning,
  releaseScheduleDeferred,

  summary(projectId: number) {
    return db
      .query<{ stage: string; status: string; n: number; lastError: string | null }, [number]>(
        `SELECT stage, status, COUNT(*) AS n, MAX(last_error) AS lastError
         FROM jobs
         WHERE project_id = ?
           AND NOT (status = 'failed' AND dismissed_at IS NOT NULL)
         GROUP BY stage, status ORDER BY stage`,
      )
      .all(projectId);
  },

  listByStatuses(
    projectId: number,
    statuses: Array<JobRow["status"]>,
    limit = 100,
  ): JobRow[] {
    if (statuses.length === 0) return [];
    const placeholders = statuses.map(() => "?").join(", ");
    const includeFailed = statuses.includes("failed");
    return db
      .query<JobRow, (number | string)[]>(
        `SELECT * FROM jobs
         WHERE project_id = ? AND status IN (${placeholders})
           ${includeFailed ? "AND (status != 'failed' OR dismissed_at IS NULL)" : ""}
         ORDER BY
           CASE status
             WHEN 'running' THEN 0
             WHEN 'pending' THEN 1
             WHEN 'failed' THEN 2
             ELSE 3
           END,
           updated_at DESC, id DESC
         LIMIT ?`,
      )
      .all(projectId, ...statuses, limit);
  },

  pendingCount(projectId: number): number {
    return (
      db
        .query<{ n: number }, [number]>(
          `SELECT COUNT(*) AS n FROM jobs WHERE project_id = ?
             AND status IN ('pending', 'running')`,
        )
        .get(projectId)?.n ?? 0
    );
  },

  failedCount(projectId: number): number {
    return (
      db
        .query<{ n: number }, [number]>(
          `SELECT COUNT(*) AS n FROM jobs
           WHERE project_id = ? AND status = 'failed' AND dismissed_at IS NULL`,
        )
        .get(projectId)?.n ?? 0
    );
  },

  /** Active send_message jobs keyed by message id, for UI schedule display. */
  activeSendsByMessage(projectId: number): Map<
    number,
    { status: string; runAfter: string; lastError: string | null }
  > {
    const rows = db
      .query<
        { payload: string; status: string; run_after: string; last_error: string | null },
        [number]
      >(
        `SELECT payload, status, run_after, last_error FROM jobs
         WHERE project_id = ? AND stage = 'send_message'
           AND status IN ('pending', 'running')`,
      )
      .all(projectId);

    const map = new Map<number, { status: string; runAfter: string; lastError: string | null }>();
    for (const row of rows) {
      try {
        const payload = JSON.parse(row.payload) as { messageId?: number };
        if (typeof payload.messageId !== "number") continue;
        map.set(payload.messageId, {
          status: row.status,
          runAfter: row.run_after,
          lastError: row.last_error,
        });
      } catch {
        // ignore malformed payloads
      }
    }
    return map;
  },
};

/* ---------------------------------------------------------------- events */

export type EventRow = {
  id: number;
  kind: string;
  ref: string;
  data: string;
  cost_usd: number;
  created_at: string;
};

export const events = {
  log(kind: string, opts: { projectId?: number; ref?: string; data?: unknown; costUsd?: number } = {}) {
    db.run(`INSERT INTO events (project_id, kind, ref, data, cost_usd) VALUES (?, ?, ?, ?, ?)`, [
      opts.projectId ?? null,
      kind,
      opts.ref ?? "",
      JSON.stringify(opts.data ?? {}),
      opts.costUsd ?? 0,
    ]);
  },
  recent(projectId: number, limit = 100) {
    return this.page(projectId, { limit }).events;
  },
  page(
    projectId: number,
    opts: { limit?: number; beforeId?: number } = {},
  ): { events: EventRow[]; total: number; nextBeforeId: number | null } {
    const limit = Math.min(Math.max(opts.limit ?? 50, 1), 100);
    const events =
      typeof opts.beforeId === "number"
        ? db
            .query<EventRow, [number, number, number]>(
              `SELECT id, kind, ref, data, cost_usd, created_at FROM events
               WHERE project_id = ? AND id < ?
               ORDER BY id DESC LIMIT ?`,
            )
            .all(projectId, opts.beforeId, limit)
        : db
            .query<EventRow, [number, number]>(
              `SELECT id, kind, ref, data, cost_usd, created_at FROM events
               WHERE project_id = ? ORDER BY id DESC LIMIT ?`,
            )
            .all(projectId, limit);

    const total =
      db
        .query<{ n: number }, [number]>(
          `SELECT COUNT(*) AS n FROM events WHERE project_id = ?`,
        )
        .get(projectId)?.n ?? 0;

    const nextBeforeId =
      events.length === limit ? (events[events.length - 1]?.id ?? null) : null;

    return { events, total, nextBeforeId };
  },
  latestId(projectId: number): number {
    return (
      db
        .query<{ id: number | null }, [number]>(
          `SELECT MAX(id) AS id FROM events WHERE project_id = ?`,
        )
        .get(projectId)?.id ?? 0
    );
  },
  totalCost(projectId: number): number {
    return (
      db
        .query<{ total: number | null }, [number]>(
          `SELECT SUM(cost_usd) AS total FROM events WHERE project_id = ?`,
        )
        .get(projectId)?.total ?? 0
    );
  },
};
