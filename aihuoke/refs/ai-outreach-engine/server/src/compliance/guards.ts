import { activeSenderAddresses, env } from "../env";
import { getSettings } from "../settings";
import { messages, suppressions } from "../db/repo";
import type { ContactRow } from "../db/types";

/**
 * Consumer mailbox providers. Cold-emailing these is both a compliance problem
 * (B2C needs prior consent almost everywhere) and a deliverability problem,
 * because complaints from them are weighted heavily by Gmail and Outlook.
 */
const FREEMAIL_DOMAINS = new Set([
  "gmail.com",
  "googlemail.com",
  "yahoo.com",
  "yahoo.co.uk",
  "yahoo.co.in",
  "hotmail.com",
  "outlook.com",
  "live.com",
  "msn.com",
  "aol.com",
  "icloud.com",
  "me.com",
  "mac.com",
  "proton.me",
  "protonmail.com",
  "gmx.com",
  "gmx.de",
  "mail.com",
  "yandex.ru",
  "zoho.com",
  "rediffmail.com",
  "qq.com",
  "163.com",
]);

/**
 * Role / departmental / generic inboxes. Cold outreach to these is ignored and
 * wastes verify credits — never discover, verify, or draft to them.
 *
 * Match is token-aware: sales.team@, sales-representative@, company@company.com
 * are rejected even when the full local part is not in this set.
 */
const ROLE_LOCAL_PARTS = new Set([
  "info",
  "support",
  "hello",
  "hi",
  "hey",
  "contact",
  "contacts",
  "admin",
  "administrator",
  "sales",
  "sale",
  "selling",
  "team",
  "teams",
  "help",
  "helpdesk",
  "press",
  "media",
  "marketing",
  "market",
  "markets",
  "pr",
  "careers",
  "career",
  "jobs",
  "job",
  "hiring",
  "recruit",
  "recruiting",
  "recruiter",
  "recruiters",
  "talent",
  "hr",
  "people",
  "peopleops",
  "recops",
  "billing",
  "finance",
  "accounts",
  "account",
  "accounting",
  "legal",
  "compliance",
  "noreply",
  "no-reply",
  "no_reply",
  "donotreply",
  "do-not-reply",
  "privacy",
  "security",
  "webmaster",
  "postmaster",
  "office",
  "enquiries",
  "enquiry",
  "inquiry",
  "inquiries",
  "reception",
  "general",
  "mail",
  "email",
  "newsletter",
  "news",
  "abuse",
  "root",
  "business",
  "biz",
  "enterprise",
  "commercial",
  "partnerships",
  "partnership",
  "partners",
  "partner",
  "affiliates",
  "affiliate",
  "vendors",
  "vendor",
  "suppliers",
  "supplier",
  "clients",
  "client",
  "customer",
  "customers",
  "success",
  "cs",
  "cx",
  "service",
  "services",
  "servicedesk",
  "ops",
  "operations",
  "product",
  "products",
  "engineering",
  "eng",
  "dev",
  "devs",
  "developers",
  "developer",
  "design",
  "designer",
  "designers",
  "founders",
  "founder",
  "ceo",
  "cto",
  "coo",
  "cfo",
  "cmo",
  "head",
  "heads",
  "lead",
  "leads",
  "manager",
  "managers",
  "director",
  "directors",
  "associate",
  "associates",
  "representative",
  "representatives",
  "rep",
  "reps",
  "agent",
  "agents",
  "desk",
  "inbox",
  "mailer",
  "updates",
  "update",
  "notify",
  "notifications",
  "alerts",
  "alert",
  "feedback",
  "community",
  "hello-world",
  "test",
  "testing",
  "demo",
  "demos",
  "example",
  "samples",
  "sample",
  "working",
  "work",
  "workers",
  "inside",
  "insider",
  "discover",
  "apply",
  "applications",
  "application",
  "join",
  "joinus",
  "welcome",
  "intro",
  "outreach",
  "growth",
  "revenue",
  "deals",
  "deal",
  "pipeline",
  "enablement",
  "onboarding",
  "implementation",
  "solutions",
  "solution",
  "consulting",
  "consultant",
  "edtech",
  "fintech",
  "healthtech",
  "saas",
  "analytics",
  "research",
  "labs",
  "studio",
  "ventures",
  "venture",
  "invest",
  "investors",
  "investor",
  "comms",
  "communications",
  "brand",
  "events",
  "event",
  "rsvp",
  "booking",
  "bookings",
  "orders",
  "order",
  "shipping",
  "returns",
  "refunds",
  "payments",
  "payment",
  "invoice",
  "invoices",
  "payroll",
  "sysadmin",
  "devops",
  "status",
  "null",
  "undefined",
  "unknown",
  "everyone",
  "anybody",
  "someone",
]);

/**
 * Strong department tokens for dotted locals (sales.team). Keep short codes
 * out of here so jane.ai / mark.us style personals are not false-positived.
 */
const ROLE_STRONG_TOKENS = new Set(
  [...ROLE_LOCAL_PARTS].filter((token) => token.length >= 4),
);

export type Verdict = { ok: true } | { ok: false; reason: string };

const OK: Verdict = { ok: true };

function domainBrand(domain: string): string {
  const host = domain.toLowerCase().replace(/^www\./, "");
  const parts = host.split(".").filter(Boolean);
  if (parts.length <= 2) return parts[0] ?? "";
  // co.uk / com.au style — brand is still the left-most label for most startups.
  return parts[0] ?? "";
}

/**
 * True for role/department/generic inboxes and company-name@company addresses.
 * Used before verify credits are spent and before drafts are written.
 */
export function isRoleOrGenericEmail(email: string): boolean {
  const trimmed = email.toLowerCase().trim();
  const at = trimmed.lastIndexOf("@");
  if (at <= 0) return true;
  const local = trimmed.slice(0, at);
  const domain = trimmed.slice(at + 1);
  if (!local || !domain) return true;

  const base = (local.split("+")[0] ?? local).replace(/^"+|"+$/g, "");
  if (!base) return true;
  if (base.length <= 1) return true;

  // Exact local-part match (includes short codes like hr@, pr@).
  if (ROLE_LOCAL_PARTS.has(base)) return true;

  // sales.team / sales-representative / business_dev — strong role token → reject.
  const tokens = base.split(/[._-]+/).filter(Boolean);
  if (tokens.length >= 2 && tokens.some((token) => ROLE_STRONG_TOKENS.has(token))) {
    return true;
  }

  // company@company.com / innovaccer@innovaccer.com / aztec@aztec.network
  const brand = domainBrand(domain);
  if (brand && (base === brand || base.replace(/-/g, "") === brand.replace(/-/g, ""))) {
    return true;
  }

  // Pure digits / no letters — never a person.
  if (!/[a-z]/i.test(base)) return true;

  return false;
}

/**
 * Local part looks like a real person address (jane.doe, j.smith, jane_doe).
 * Used to decide whether a scraped mailto is worth keeping at all.
 */
export function looksLikePersonalEmail(email: string): boolean {
  if (isRoleOrGenericEmail(email)) return false;
  const local = (email.split("@")[0] ?? "").toLowerCase().split("+")[0] ?? "";
  // first.last / first_last / first-last (optionally middle)
  if (/^[a-z]{2,}[._-][a-z]{2,}([._-][a-z]{2,})?$/.test(local)) return true;
  // j.smith / j_smith
  if (/^[a-z][._-][a-z]{2,}$/.test(local)) return true;
  return false;
}

/** Checks that depend only on the recipient, run before a draft is written. */
export function screenContact(contact: ContactRow): Verdict {
  const settings = getSettings();
  const email = contact.email.toLowerCase();
  const domain = email.split("@")[1] ?? "";

  if (!/^[^@\s]+@[^@\s]+\.[a-z]{2,}$/i.test(email)) {
    return { ok: false, reason: "not a valid email address" };
  }
  if (isRoleOrGenericEmail(email)) {
    return { ok: false, reason: "role/generic inbox (support@, hello@, media@, …) — skipped" };
  }
  // Prospecting never cold-emails consumer mailboxes. Manual/imported contacts
  // are an explicit exception — the operator chose that address on purpose.
  const userSupplied =
    contact.email_source === "manual" || contact.email_source === "imported";
  if (settings.blockFreemail && FREEMAIL_DOMAINS.has(domain) && !userSupplied) {
    return { ok: false, reason: `${domain} is a consumer mailbox; this tool is B2B only` };
  }
  // Block self-company sends (e.g. you@acme.com → coworker@acme.com). Freemail
  // domains are shared by millions of people — same domain ≠ same org, so allow.
  const senderDomain = activeSenderAddresses().fromEmail.split("@")[1]?.toLowerCase() ?? "";
  if (domain && domain === senderDomain && !FREEMAIL_DOMAINS.has(domain)) {
    return { ok: false, reason: "recipient is on the sending domain" };
  }
  if (contact.country && settings.blockedCountries.includes(contact.country.toUpperCase())) {
    return {
      ok: false,
      reason: `${contact.country} is in blocked countries — skipped at draft/send`,
    };
  }
  if (suppressions.isSuppressed(email)) {
    return { ok: false, reason: "on the suppression list" };
  }
  if (contact.verify_status === "invalid") {
    return { ok: false, reason: "verification says the address is invalid" };
  }
  return OK;
}

/** Rate and schedule checks, re-run immediately before every single send. */
export function screenSendWindow(projectId: number, now = new Date()): Verdict {
  const settings = getSettings();
  const sentToday = messages.sentInLastDay(projectId);
  if (sentToday >= settings.dailySendCap) {
    return {
      ok: false,
      reason: `daily cap reached (${sentToday}/${settings.dailySendCap} in the last 24h)`,
    };
  }

  const hour = Number(
    new Intl.DateTimeFormat("en-GB", {
      hour: "numeric",
      hour12: false,
      timeZone: settings.sendWindowTimezone,
    }).format(now),
  );
  if (hour < settings.sendWindowStartHour || hour >= settings.sendWindowEndHour) {
    return {
      ok: false,
      reason: `outside the send window (${settings.sendWindowStartHour}:00-${settings.sendWindowEndHour}:00 ${settings.sendWindowTimezone}, now ${hour}:00)`,
    };
  }

  const weekday = new Intl.DateTimeFormat("en-GB", {
    weekday: "short",
    timeZone: settings.sendWindowTimezone,
  }).format(now);
  if (!settings.sendOnWeekends && (weekday === "Sat" || weekday === "Sun")) {
    return { ok: false, reason: "weekend; outbound is paused" };
  }

  const lastSent = messages.lastSentAt(projectId);
  if (lastSent) {
    // Stored as SQLite's "YYYY-MM-DD HH:MM:SS" in UTC.
    const elapsedMinutes =
      (now.getTime() - new Date(`${lastSent.replace(" ", "T")}Z`).getTime()) / 60_000;
    if (elapsedMinutes < settings.minMinutesBetweenSends) {
      return {
        ok: false,
        reason: `throttled; ${Math.ceil(settings.minMinutesBetweenSends - elapsedMinutes)} min until the next send`,
      };
    }
  }

  return OK;
}

/**
 * Configuration that must be present before anything real can be sent. Missing
 * items are shown in the UI rather than discovered as a bounce later.
 */
export function preflight(): string[] {
  const problems: string[] = [];
  if (env.sender.provider === "dry_run") return problems;

  const { fromEmail } = activeSenderAddresses();
  if (!fromEmail.includes("@")) {
    problems.push(
      env.sender.provider === "gmail"
        ? "GMAIL_FROM_EMAIL (or SENDER_FROM_EMAIL) is not a valid address."
        : "BREVO_FROM_EMAIL (or SENDER_FROM_EMAIL) is not a valid address.",
    );
  }
  if (env.sender.provider === "brevo" && !env.sender.brevo.apiKey) {
    problems.push("SENDER_PROVIDER=brevo but BREVO_API_KEY is missing.");
  }
  if (env.sender.provider === "gmail" && !env.sender.gmail.appPassword.trim()) {
    problems.push("SENDER_PROVIDER=gmail but GMAIL_APP_PASSWORD is missing.");
  }
  return problems;
}
