import { audiences, companies, contacts, jobs } from "../db/repo";
import type { Audience, ContactRow, WorkflowType } from "../db/types";

export type ManualContactInput = {
  email: string;
  fullName?: string;
  title?: string;
  companyName?: string;
  companyDomain?: string;
  country?: string;
  evidenceUrl?: string;
  /** Omit to keep existing (on update) / Manual contacts (on create). null = Manual contacts. */
  audienceId?: number | null;
};

export type ContactView = ContactRow & {
  company_name: string;
  company_domain: string;
  audience_name: string;
  project_name: string;
  workflow_type: WorkflowType;
};

export type ManualContactResult =
  | { status: "created"; contact: ContactView }
  | { status: "updated"; contact: ContactView }
  | { status: "skipped"; email: string; reason: string }
  | { status: "error"; email: string; reason: string };

const EMAIL_RE = /^[^@\s]+@[^@\s]+\.[a-z]{2,}$/i;
const MAX_IMPORT = 500;

function normaliseDomain(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/^https?:\/\//, "")
    .replace(/^www\./, "")
    .split("/")[0]!
    .replace(/:\d+$/, "");
}

function domainFromEmail(email: string): string {
  return (email.split("@")[1] ?? "").toLowerCase();
}

function companyLabel(name: string | undefined, domain: string): string {
  const trimmed = name?.trim();
  if (trimmed) return trimmed;
  const stem = domain.split(".")[0] ?? domain;
  return stem ? stem.charAt(0).toUpperCase() + stem.slice(1) : domain;
}

/**
 * Prefer an explicit audience. With none selected, always use (or create)
 * "Manual contacts" — never silently attach to a prospecting audience.
 */
export function resolveAudience(projectId: number, audienceId?: number): Audience {
  if (typeof audienceId === "number") {
    const audience = audiences.get(audienceId);
    if (!audience || audience.projectId !== projectId) {
      throw new Error("Audience not found for this project");
    }
    return audience;
  }

  const existing = audiences.list(projectId).find((row) => row.name === "Manual contacts");
  if (existing) return existing;

  return audiences.create(projectId, {
    name: "Manual contacts",
    description: "Contacts added or imported by hand.",
    origin: "manual",
  });
}

/**
 * Persist a user-supplied contact, inventing company/audience rows when needed,
 * then queue a draft so it shows up under Review & send.
 */
export function addManualContact(
  projectId: number,
  input: ManualContactInput,
  source: "manual" | "imported" = "manual",
): ManualContactResult {
  const email = input.email.trim().toLowerCase();
  if (!EMAIL_RE.test(email)) {
    return { status: "error", email: input.email || "(empty)", reason: "invalid email address" };
  }

  let audience: Audience;
  try {
    audience = resolveAudience(projectId, input.audienceId ?? undefined);
  } catch (error) {
    return {
      status: "error",
      email,
      reason: error instanceof Error ? error.message : "invalid audience",
    };
  }

  const domain = normaliseDomain(input.companyDomain || domainFromEmail(email));
  if (!domain || !domain.includes(".")) {
    return { status: "error", email, reason: "could not determine company domain" };
  }

  const company = companies.ensure({
    projectId,
    audienceId: audience.id,
    name: companyLabel(input.companyName, domain),
    domain,
    country: input.country?.trim() ?? "",
    sourceUrl: source === "imported" ? "import" : "manual",
  });

  const row = contacts.upsert({
    projectId,
    audienceId: audience.id,
    companyId: company.id,
    fullName: input.fullName?.trim() || "Contact",
    title: input.title?.trim() || "",
    email,
    emailSource: source,
    evidenceUrl: input.evidenceUrl?.trim() || source,
    country: input.country?.trim() || company.country || "",
  });

  if (!row) {
    return { status: "skipped", email, reason: "already exists in this project" };
  }

  // User-supplied addresses are trusted; skip the paid verifier and draft next.
  contacts.setVerification(row.id, "valid", null);
  jobs.ensureDraft(projectId, row.id);

  return {
    status: "created",
    contact: contacts.withCompany({ ...row, verify_status: "valid" }),
  };
}

export function updateManualContact(
  contactId: number,
  input: ManualContactInput,
): ManualContactResult {
  const existing = contacts.get(contactId);
  if (!existing) {
    return { status: "error", email: input.email || "(unknown)", reason: "contact not found" };
  }

  const email = (input.email ?? existing.email).trim().toLowerCase();
  if (!EMAIL_RE.test(email)) {
    return { status: "error", email: input.email || existing.email, reason: "invalid email address" };
  }
  if (contacts.emailTaken(existing.project_id, email, contactId)) {
    return { status: "error", email, reason: "another contact already uses this email" };
  }

  let audience: Audience;
  try {
    const audienceId =
      input.audienceId === null
        ? undefined
        : (input.audienceId ?? existing.audience_id);
    audience = resolveAudience(existing.project_id, audienceId);
  } catch (error) {
    return {
      status: "error",
      email,
      reason: error instanceof Error ? error.message : "invalid audience",
    };
  }

  const domain = normaliseDomain(
    input.companyDomain || domainFromEmail(email) || companies.get(existing.company_id)?.domain || "",
  );
  if (!domain || !domain.includes(".")) {
    return { status: "error", email, reason: "could not determine company domain" };
  }

  const company = companies.ensure({
    projectId: existing.project_id,
    audienceId: audience.id,
    name: companyLabel(input.companyName, domain),
    domain,
    country: input.country?.trim() ?? existing.country,
  });

  const row = contacts.update(contactId, {
    audienceId: audience.id,
    companyId: company.id,
    fullName: input.fullName?.trim() || existing.full_name || "Contact",
    title: input.title !== undefined ? input.title.trim() : existing.title,
    email,
    evidenceUrl:
      input.evidenceUrl !== undefined ? input.evidenceUrl.trim() : existing.evidence_url,
    country: input.country?.trim() || existing.country || company.country || "",
  });

  if (!row) {
    return { status: "error", email, reason: "contact not found" };
  }

  return { status: "updated", contact: contacts.withCompany(row) };
}

export function addManualContacts(
  projectId: number,
  rows: ManualContactInput[],
  source: "manual" | "imported" = "imported",
): {
  created: number;
  skipped: number;
  errors: Array<{ email: string; reason: string }>;
  contacts: ContactView[];
} {
  if (rows.length > MAX_IMPORT) {
    throw new Error(`Import is limited to ${MAX_IMPORT} contacts at a time`);
  }

  const created: ContactView[] = [];
  const errors: Array<{ email: string; reason: string }> = [];
  let skipped = 0;

  for (const row of rows) {
    const result = addManualContact(projectId, row, source);
    if (result.status === "created") created.push(result.contact);
    else if (result.status === "skipped") skipped += 1;
    else if (result.status === "error") {
      errors.push({ email: result.email, reason: result.reason });
    }
  }

  return { created: created.length, skipped, errors, contacts: created };
}

/** Minimal RFC4180-ish CSV parse (handles quoted commas/newlines). */
export function parseContactsCsv(csv: string): ManualContactInput[] {
  const text = csv.replace(/^\uFEFF/, "").trim();
  if (!text) return [];

  const rows: string[][] = [];
  let field = "";
  let row: string[] = [];
  let inQuotes = false;

  for (let i = 0; i < text.length; i += 1) {
    const ch = text[i]!;
    const next = text[i + 1];
    if (inQuotes) {
      if (ch === '"' && next === '"') {
        field += '"';
        i += 1;
      } else if (ch === '"') {
        inQuotes = false;
      } else {
        field += ch;
      }
      continue;
    }
    if (ch === '"') {
      inQuotes = true;
      continue;
    }
    if (ch === ",") {
      row.push(field);
      field = "";
      continue;
    }
    if (ch === "\n" || ch === "\r") {
      if (ch === "\r" && next === "\n") i += 1;
      row.push(field);
      field = "";
      if (row.some((cell) => cell.trim())) rows.push(row);
      row = [];
      continue;
    }
    field += ch;
  }
  row.push(field);
  if (row.some((cell) => cell.trim())) rows.push(row);

  if (rows.length === 0) return [];

  const header = rows[0]!.map((cell) => cell.trim().toLowerCase());
  const hasHeader = header.includes("email");
  const dataRows = hasHeader ? rows.slice(1) : rows;
  const index = (name: string) => header.indexOf(name);

  return dataRows
    .map((cells) => {
      if (!hasHeader) {
        return {
          email: cells[0]?.trim() ?? "",
          fullName: cells[1]?.trim() || undefined,
          title: cells[2]?.trim() || undefined,
          companyName: cells[3]?.trim() || undefined,
          companyDomain: cells[4]?.trim() || undefined,
        };
      }
      const get = (name: string) => {
        const i = index(name);
        return i >= 0 ? cells[i]?.trim() || undefined : undefined;
      };
      return {
        email: get("email") ?? "",
        fullName: get("full_name") ?? get("name"),
        title: get("title"),
        companyName: get("company_name") ?? get("company"),
        companyDomain: get("company_domain") ?? get("domain"),
        country: get("country"),
        evidenceUrl: get("evidence_url"),
      };
    })
    .filter((row) => row.email);
}
