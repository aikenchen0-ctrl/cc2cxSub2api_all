import type { WorkflowType } from "../db/types";
import { DEFAULT_MARKETS, DEFAULT_SIGNALS, getSettings } from "../settings";
import {
  jobSearchBriefFromProject,
  normalizeCompanySizes,
  normalizeJobIndustries,
} from "./jobBrief";

export type CustomerPreferences = {
  targetMarkets: string[];
  targetCompanySignals: string[];
  maxContactsPerCompany: number;
  crawlMaxPages: number;
  localizeEmails: boolean;
};

export type InvestorPreferences = {
  investorTypes: string[];
  thesisSignals: string[];
  maxContactsPerFirm: number;
  localizeEmails: boolean;
};

export type JobPreferences = {
  companySizes: string[];
  industries: string[];
  maxContactsPerCompany: number;
  localizeEmails: boolean;
  /**
   * When true: each contact email is LLM-written from that job listing.
   * When false (default): render the search-lane template with JD merge fills — no per-contact email LLM.
   */
  tailorEmailsToJobListing: boolean;
};

export type PlaybookPreferences =
  | ({ kind: "customer_outreach" } & CustomerPreferences)
  | ({ kind: "investor_outreach" } & InvestorPreferences)
  | ({ kind: "job_outreach" } & JobPreferences);

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" ? (value as Record<string, unknown>) : {};
}

/**
 * Parse preference lists from AI/UI. Array items may still contain newlines or
 * commas (models love "Priority 1\\nUnited States"), so flatten aggressively.
 */
export function stringList(value: unknown): string[] {
  const chunks: string[] = [];
  if (Array.isArray(value)) {
    for (const item of value) chunks.push(String(item));
  } else if (typeof value === "string") {
    chunks.push(value);
  } else {
    return [];
  }

  const out: string[] = [];
  const seen = new Set<string>();
  for (const chunk of chunks) {
    for (const part of chunk.split(/\n|;|\u2022|\||,/)) {
      const cleaned = part
        .replace(/^priority\s*\d+\s*[:.\-]?\s*/i, "")
        .replace(/\s+/g, " ")
        .trim();
      if (!cleaned) continue;
      // Drop heading-only leftovers like "Priority 1".
      if (/^priority\s*\d+$/i.test(cleaned)) continue;
      const key = cleaned.toLowerCase();
      if (seen.has(key)) continue;
      seen.add(key);
      out.push(cleaned);
    }
  }
  return out;
}

/** Keep only short, Google-usable market/signal phrases. */
export function sanitizeSearchTerms(terms: string[], opts: { maxLen?: number } = {}): string[] {
  const maxLen = opts.maxLen ?? 48;
  // Re-run stringList so embedded newlines / Priority labels are flattened.
  const flattened = stringList(terms);
  const out: string[] = [];
  const seen = new Set<string>();
  for (const term of flattened) {
    const cleaned = term
      .replace(/\s+/g, " ")
      .replace(/[.!?]+$/g, "")
      .trim();
    if (!cleaned) continue;
    if (cleaned.length > maxLen) continue;
    // Prefer short atomic phrases Google can AND cleanly.
    if (cleaned.split(/\s+/).length > 5) continue;
    if (/[()]/.test(cleaned)) continue;
    // Prose / multi-clause blobs are useless as Serper AND constraints.
    if ((cleaned.match(/[.!?]/g) ?? []).length >= 1) continue;
    if ((cleaned.match(/\band\b/gi) ?? []).length >= 2) continue;
    const key = cleaned.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(cleaned);
  }
  return out;
}

function clampInt(value: unknown, min: number, max: number, fallback: number): number {
  const n = typeof value === "number" ? value : Number(value);
  if (!Number.isFinite(n)) return fallback;
  return Math.min(max, Math.max(min, Math.round(n)));
}

function parseWorkflowData(raw: string | unknown): Record<string, unknown> {
  if (typeof raw !== "string") return asRecord(raw);
  try {
    return asRecord(JSON.parse(raw));
  } catch {
    return {};
  }
}

/** Empty draft for a new campaign — blank lists invite AI fill-in. */
export function emptyPreferences(workflowType: "customer_outreach"): Extract<PlaybookPreferences, { kind: "customer_outreach" }>;
export function emptyPreferences(workflowType: "investor_outreach"): Extract<PlaybookPreferences, { kind: "investor_outreach" }>;
export function emptyPreferences(workflowType: "job_outreach"): Extract<PlaybookPreferences, { kind: "job_outreach" }>;
export function emptyPreferences(workflowType: WorkflowType): PlaybookPreferences;
export function emptyPreferences(workflowType: WorkflowType): PlaybookPreferences {
  const settings = getSettings();
  if (workflowType === "investor_outreach") {
    return {
      kind: "investor_outreach",
      investorTypes: [],
      thesisSignals: [],
      maxContactsPerFirm: settings.maxContactsPerCompany,
      localizeEmails: settings.localizeEmails,
    };
  }
  if (workflowType === "job_outreach") {
    return {
      kind: "job_outreach",
      companySizes: [],
      industries: [],
      maxContactsPerCompany: Math.min(settings.maxContactsPerCompany, 5),
      localizeEmails: settings.localizeEmails,
      tailorEmailsToJobListing: false,
    };
  }
  return {
    kind: "customer_outreach",
    // Prefill premium defaults so new campaigns don't wait on weak AI suggestions.
    targetMarkets: settings.targetMarkets.length ? settings.targetMarkets : [...DEFAULT_MARKETS],
    targetCompanySignals: settings.targetCompanySignals.length
      ? settings.targetCompanySignals
      : [...DEFAULT_SIGNALS],
    maxContactsPerCompany: settings.maxContactsPerCompany,
    crawlMaxPages: settings.crawlMaxPages,
    localizeEmails: settings.localizeEmails,
  };
}

/** Parse stored campaign preferences without inventing list values. */
export function parseStoredPreferences(
  workflowType: WorkflowType,
  raw: unknown,
): PlaybookPreferences {
  const data = asRecord(raw);

  if (workflowType === "investor_outreach") {
    const empty = emptyPreferences("investor_outreach");
    return {
      kind: "investor_outreach",
      investorTypes: stringList(data.investorTypes),
      thesisSignals: stringList(data.thesisSignals),
      maxContactsPerFirm: clampInt(
        data.maxContactsPerFirm,
        1,
        15,
        empty.maxContactsPerFirm,
      ),
      localizeEmails:
        typeof data.localizeEmails === "boolean" ? data.localizeEmails : empty.localizeEmails,
    };
  }

  if (workflowType === "job_outreach") {
    const empty = emptyPreferences("job_outreach");
    const sizes = normalizeCompanySizes(data.companySizes);
    const { industries } = normalizeJobIndustries(data.industries);
    return {
      kind: "job_outreach",
      companySizes: sizes.length ? sizes : stringList(data.companySizes),
      industries,
      maxContactsPerCompany: clampInt(
        data.maxContactsPerCompany,
        1,
        15,
        empty.maxContactsPerCompany,
      ),
      localizeEmails:
        typeof data.localizeEmails === "boolean" ? data.localizeEmails : empty.localizeEmails,
      tailorEmailsToJobListing:
        typeof data.tailorEmailsToJobListing === "boolean"
          ? data.tailorEmailsToJobListing
          : empty.tailorEmailsToJobListing,
    };
  }

  const empty = emptyPreferences("customer_outreach");
  return {
    kind: "customer_outreach",
    targetMarkets: stringList(data.targetMarkets),
    targetCompanySignals: stringList(data.targetCompanySignals),
    maxContactsPerCompany: clampInt(
      data.maxContactsPerCompany,
      1,
      15,
      empty.maxContactsPerCompany,
    ),
    crawlMaxPages: clampInt(data.crawlMaxPages, 1, 50, empty.crawlMaxPages),
    localizeEmails:
      typeof data.localizeEmails === "boolean" ? data.localizeEmails : empty.localizeEmails,
  };
}

/** Runtime preferences with sensible fallbacks when lists were left blank. */
export function effectivePreferences(preferences: PlaybookPreferences): PlaybookPreferences {
  const settings = getSettings();
  if (preferences.kind === "investor_outreach") {
    return {
      ...preferences,
      investorTypes: preferences.investorTypes.length
        ? preferences.investorTypes
        : ["seed funds", "angels", "micro-VCs", "accelerators"],
      thesisSignals: preferences.thesisSignals,
    };
  }
  if (preferences.kind === "job_outreach") {
    const sizes = normalizeCompanySizes(preferences.companySizes);
    return {
      ...preferences,
      companySizes: sizes.length
        ? sizes
        : preferences.companySizes.length
          ? preferences.companySizes
          : ["1-10", "11-50"],
      industries: preferences.industries,
    };
  }
  return {
    ...preferences,
    targetMarkets: preferences.targetMarkets.length
      ? preferences.targetMarkets
      : settings.targetMarkets,
    targetCompanySignals: preferences.targetCompanySignals.length
      ? preferences.targetCompanySignals
      : settings.targetCompanySignals,
  };
}

export function preferencesFromProject(project: {
  workflow_type: WorkflowType;
  workflow_data: string;
}): PlaybookPreferences {
  const data = parseWorkflowData(project.workflow_data);
  return parseStoredPreferences(project.workflow_type, data.preferences);
}

export function mergePreferencesIntoWorkflowData(
  workflowType: WorkflowType,
  workflowData: unknown,
  preferences: unknown,
): Record<string, unknown> {
  const data = asRecord(workflowData);
  const current = parseStoredPreferences(workflowType, data.preferences);
  const incomingRaw = asRecord(preferences);
  const incoming = parseStoredPreferences(workflowType, preferences);

  // Preserve job email mode when Setup saves prefs without that field.
  if (
    workflowType === "job_outreach" &&
    current.kind === "job_outreach" &&
    incoming.kind === "job_outreach" &&
    typeof incomingRaw.tailorEmailsToJobListing !== "boolean"
  ) {
    return {
      ...data,
      preferences: {
        ...incoming,
        tailorEmailsToJobListing: current.tailorEmailsToJobListing,
      },
    };
  }

  return {
    ...data,
    preferences: incoming,
  };
}

/** Jobs: true = LLM email from JD; false = render lane template (default). */
export function tailorJobEmailsEnabled(project: {
  workflow_type: WorkflowType;
  workflow_data: string;
}): boolean {
  const prefs = preferencesFromProject(project);
  return prefs.kind === "job_outreach" && prefs.tailorEmailsToJobListing;
}

export function preferencesNeedAiFill(preferences: PlaybookPreferences): boolean {
  if (preferences.kind === "customer_outreach") {
    return preferences.targetMarkets.length === 0 || preferences.targetCompanySignals.length === 0;
  }
  if (preferences.kind === "investor_outreach") {
    return preferences.investorTypes.length === 0 || preferences.thesisSignals.length === 0;
  }
  return preferences.companySizes.length === 0 || preferences.industries.length === 0;
}

export function mergeAiPreferences(
  current: PlaybookPreferences,
  suggested: unknown,
): PlaybookPreferences {
  const incoming = parseStoredPreferences(current.kind, suggested);
  if (current.kind === "customer_outreach" && incoming.kind === "customer_outreach") {
    return {
      ...current,
      targetMarkets: current.targetMarkets.length
        ? current.targetMarkets
        : incoming.targetMarkets,
      targetCompanySignals: current.targetCompanySignals.length
        ? current.targetCompanySignals
        : incoming.targetCompanySignals,
    };
  }
  if (current.kind === "investor_outreach" && incoming.kind === "investor_outreach") {
    return {
      ...current,
      investorTypes: current.investorTypes.length ? current.investorTypes : incoming.investorTypes,
      thesisSignals: current.thesisSignals.length ? current.thesisSignals : incoming.thesisSignals,
    };
  }
  if (current.kind === "job_outreach" && incoming.kind === "job_outreach") {
    return {
      ...current,
      companySizes: current.companySizes.length ? current.companySizes : incoming.companySizes,
      industries: current.industries.length ? current.industries : incoming.industries,
      // Never let AI fill overwrite the user's email-mode choice.
      tailorEmailsToJobListing: current.tailorEmailsToJobListing,
    };
  }
  return current;
}

export function preferencesForPrompt(preferences: PlaybookPreferences): Record<string, unknown> {
  const effective = effectivePreferences(preferences);
  if (effective.kind === "investor_outreach") {
    return {
      investorTypes: effective.investorTypes,
      thesisSignals: effective.thesisSignals,
      maxContactsPerFirm: effective.maxContactsPerFirm,
      localizeNonEnglishMarkets: effective.localizeEmails,
    };
  }
  if (effective.kind === "job_outreach") {
    return {
      companySizes: effective.companySizes,
      industries: effective.industries,
      maxContactsPerCompany: effective.maxContactsPerCompany,
      localizeNonEnglishMarkets: effective.localizeEmails,
      tailorEmailsToJobListing: effective.tailorEmailsToJobListing,
    };
  }
  return {
    targetMarkets: effective.targetMarkets,
    targetCompanySignals: effective.targetCompanySignals,
    maxContactsPerCompany: effective.maxContactsPerCompany,
    crawlMaxPages: effective.crawlMaxPages,
    localizeNonEnglishMarkets: effective.localizeEmails,
  };
}

export function maxContactsForProject(project: {
  workflow_type: WorkflowType;
  workflow_data: string;
}): number {
  const prefs = effectivePreferences(preferencesFromProject(project));
  if (prefs.kind === "investor_outreach") return prefs.maxContactsPerFirm;
  return prefs.maxContactsPerCompany;
}

export function crawlMaxPagesForProject(project: {
  workflow_type: WorkflowType;
  workflow_data: string;
}): number {
  const prefs = effectivePreferences(preferencesFromProject(project));
  if (prefs.kind === "customer_outreach") return prefs.crawlMaxPages;
  return getSettings().crawlMaxPages;
}

export function localizeEmailsForProject(project: {
  workflow_type: WorkflowType;
  workflow_data: string;
}): boolean {
  return preferencesFromProject(project).localizeEmails;
}

export function searchEnrichment(project: {
  workflow_type: WorkflowType;
  workflow_data: string;
}): { markets: string[]; signals: string[] } {
  const data = parseWorkflowData(project.workflow_data);
  const prefs = effectivePreferences(preferencesFromProject(project));

  if (prefs.kind === "investor_outreach") {
    const geography = typeof data.geography === "string" ? stringList(data.geography) : [];
    const sectors = stringList(data.sectors);
    return {
      markets: sanitizeSearchTerms(
        geography.length ? geography : ["US", "UK", "EU", "India", "Southeast Asia"],
        { maxLen: 40 },
      ),
      signals: sanitizeSearchTerms(
        [
          ...prefs.investorTypes,
          ...prefs.thesisSignals,
          ...sectors.map((sector) => `${sector} investor`),
        ].filter(Boolean),
        { maxLen: 48 },
      ),
    };
  }

  if (prefs.kind === "job_outreach") {
    const brief = jobSearchBriefFromProject(project);
    const markets = brief?.markets.length
      ? brief.markets
      : (() => {
          const locations = stringList(data.locations);
          const remote =
            typeof data.remotePreference === "string" ? data.remotePreference.trim() : "";
          return [
            ...locations,
            ...(remote && /remote/i.test(remote) ? ["Remote"] : []),
          ];
        })();
    const roles = brief?.targetRoles.length
      ? brief.targetRoles
      : stringList(data.targetRoles);
    const industries = brief?.industries.length ? brief.industries : prefs.industries;
    return {
      markets: sanitizeSearchTerms(
        markets.length ? markets : ["Europe", "Remote", "United Kingdom", "Germany", "India"],
        { maxLen: 40 },
      ),
      signals: sanitizeSearchTerms(
        [
          ...industries.map((industry) => `${industry} hiring`),
          ...(brief?.queryHints ?? []),
          ...roles.slice(0, 6).map((role) => `${role} job`),
          "we're hiring",
          "careers",
          "open roles",
        ],
        { maxLen: 48 },
      ),
    };
  }

  return {
    markets: sanitizeSearchTerms(prefs.targetMarkets, { maxLen: 40 }),
    signals: sanitizeSearchTerms(prefs.targetCompanySignals, { maxLen: 40 }),
  };
}
