/**
 * Structured job-search brief from playbook setup (resume form + preferences).
 * Used so lane generation, Serper queries, and qualify all honor what the user typed.
 */

import { COMPANY_SIZE_BUCKETS, type CompanySizeBucket } from "../providers/companyFilters";

/** Local copy — avoid circular import with preferences.ts. */
function stringList(value: unknown): string[] {
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
      if (!cleaned || /^priority\s*\d+$/i.test(cleaned)) continue;
      const key = cleaned.toLowerCase();
      if (seen.has(key)) continue;
      seen.add(key);
      out.push(cleaned);
    }
  }
  return out;
}

export type JobSearchBrief = {
  targetRoles: string[];
  locations: string[];
  /** Markets ordered for search — preferred geos first. */
  markets: string[];
  remotePreference: string;
  seniority: string;
  notes: string;
  /** Normalized size buckets only. */
  companySizes: CompanySizeBucket[];
  /** Clean industry terms usable in Google queries. */
  industries: string[];
  /** True when the user asked to prefer AI companies. */
  preferAi: boolean;
  /** 0 = no filter. Otherwise prefer employers funded/raised at or after this year. */
  fundedAfterYear: number;
  /** Short Serper-friendly constraint phrases from notes/prefs. */
  queryHints: string[];
  /** Human-readable must-follow rules for LLM prompts. */
  hardConstraints: string[];
};

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" ? (value as Record<string, unknown>) : {};
}

function parseWorkflowData(raw: string | unknown): Record<string, unknown> {
  if (typeof raw !== "string") return asRecord(raw);
  try {
    return asRecord(JSON.parse(raw));
  } catch {
    return {};
  }
}

/** Map freeform / invalid size strings onto canonical buckets. */
export function normalizeCompanySizes(raw: unknown): CompanySizeBucket[] {
  const items = stringList(raw);
  if (items.length === 0) return [];

  const out = new Set<CompanySizeBucket>();
  for (const item of items) {
    const cleaned = item.toLowerCase().replace(/\s+/g, "");
    const asIs = COMPANY_SIZE_BUCKETS.find((bucket) => bucket === item.trim());
    if (asIs) {
      out.add(asIs);
      continue;
    }

    // "0-50", "1-50", "under 50", "small team", "<50"
    if (
      /^(0|1)-50$/.test(cleaned) ||
      /under50|less(?:than)?50|<50|upto50|smallteam|earlystage/.test(cleaned) ||
      /\bsmall\b/.test(item.toLowerCase())
    ) {
      out.add("1-10");
      out.add("11-50");
      continue;
    }

    const range = cleaned.match(/^(\d+)-(\d+)$/);
    if (range) {
      const min = Number(range[1]);
      const max = Number(range[2]);
      for (const bucket of COMPANY_SIZE_BUCKETS) {
        const [bMin, bMax] =
          bucket === "1000+"
            ? [1000, Number.POSITIVE_INFINITY]
            : bucket.split("-").map(Number);
        if (min <= (bMax ?? 0) && max >= (bMin ?? 0)) out.add(bucket);
      }
      continue;
    }

    if (/1-?10|solo|tiny/.test(cleaned)) out.add("1-10");
    else if (/11-?50|startup/.test(cleaned)) out.add("11-50");
    else if (/51-?200/.test(cleaned)) out.add("51-200");
    else if (/201-?1000/.test(cleaned)) out.add("201-1000");
    else if (/1000\+|enterprise|large/.test(cleaned)) out.add("1000+");
  }

  return COMPANY_SIZE_BUCKETS.filter((bucket) => out.has(bucket));
}

/**
 * Turn vague industry prefs into searchable terms.
 * "Any Industry with a strong preference for an AI company" → ["AI"] + preferAi.
 */
export function normalizeJobIndustries(raw: unknown): {
  industries: string[];
  preferAi: boolean;
} {
  const items = stringList(raw);
  const industries: string[] = [];
  let preferAi = false;
  const seen = new Set<string>();

  for (const item of items) {
    const lower = item.toLowerCase();
    if (/\bai\b|artificial intelligence|llm|machine learning/.test(lower)) {
      preferAi = true;
    }
    // Drop useless "any industry…" blobs; keep the AI signal only.
    if (/^any\b/.test(lower) || lower.includes("any industry")) {
      continue;
    }
    // Keep short atomic industry labels.
    const cleaned = item.replace(/\s+/g, " ").trim();
    if (!cleaned || cleaned.split(/\s+/).length > 4) {
      // Try to pull a known vertical out of a long phrase.
      const known = cleaned.match(
        /\b(AI|SaaS|fintech|healthtech|climate|devtools|developer tools|crypto|blockchain|martech|edtech|productivity|infra(?:structure)?)\b/i,
      );
      if (known?.[1]) {
        const label = known[1].replace(/^ai$/i, "AI");
        const key = label.toLowerCase();
        if (!seen.has(key)) {
          seen.add(key);
          industries.push(label === "ai" ? "AI" : label);
        }
      }
      continue;
    }
    const key = cleaned.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    industries.push(cleaned);
  }

  if (preferAi && !industries.some((item) => /\bai\b/i.test(item))) {
    industries.unshift("AI");
  }

  return { industries, preferAi };
}

function fundedYearFromText(text: string): number {
  const lower = text.toLowerCase();
  // "funded in 2026", "raised in 2026", "should be funded in 2026"
  // Also tolerate common typos like "fuded".
  const explicit = lower.match(
    /(?:fund(?:ed|ing)?|fuded|raised?|raise)\s+(?:in\s+)?(20\d{2})/,
  );
  if (explicit?.[1]) return Number(explicit[1]);
  const yearOnly = lower.match(/\b(?:fund(?:ed|ing)?|fuded)[^\n.]{0,40}(20\d{2})/);
  if (yearOnly?.[1]) return Number(yearOnly[1]);
  // "Should be … in 2026" near funding language
  const near = lower.match(
    /(?:fund|fud|rais|seed|series)[^\n.]{0,48}\bin\s+(20\d{2})\b/,
  );
  if (near?.[1]) return Number(near[1]);
  return 0;
}

function europePreferred(locations: string[], notes: string): boolean {
  const blob = `${locations.join(" ")} ${notes}`.toLowerCase();
  return /european company|europe(?:an)?\s+(?:is\s+)?most preferred|prefer(?:ence)?\s+(?:for\s+)?europe|eu\b.*prefer|prefer.*\beu\b/.test(
    blob,
  );
}

function orderMarkets(locations: string[], notes: string, remote: string): string[] {
  const preferEurope = europePreferred(locations, notes);
  const europeTerms = [
    "Europe",
    "European Union",
    "EMEA",
    "Netherlands",
    "Germany",
    "Spain",
    "Italy",
    "United Kingdom",
    "France",
  ];
  const cleaned = locations
    .map((item) =>
      item
        .replace(/\.+/g, " ")
        .replace(/\bbut\b.*$/i, "")
        .replace(/\s+/g, " ")
        .trim(),
    )
    .filter(Boolean)
    // Drop prose fragments that aren't places.
    .filter((item) => !/most preferred|but |company is/i.test(item))
    .filter((item) => item.split(/\s+/).length <= 4);

  const out: string[] = [];
  const seen = new Set<string>();
  const push = (term: string) => {
    const key = term.toLowerCase();
    if (!term || seen.has(key)) return;
    seen.add(key);
    out.push(term);
  };

  if (preferEurope) {
    for (const term of europeTerms) {
      if (cleaned.some((loc) => loc.toLowerCase().includes(term.toLowerCase().slice(0, 5)))) {
        push(term);
      }
    }
    // Always bias Europe when the user said European company preferred.
    push("Europe");
    push("European Union");
  }

  for (const loc of cleaned) push(loc);

  if (remote && /remote/i.test(remote)) {
    if (/india/i.test(remote)) push("Remote from India");
    push("Remote");
  }

  return out.slice(0, 12);
}

function queryHintsFromNotes(notes: string, preferAi: boolean, fundedAfterYear: number): string[] {
  const hints: string[] = [];
  const lower = notes.toLowerCase();
  if (preferAi) hints.push("AI startup");
  if (fundedAfterYear > 0) {
    hints.push(`funded ${fundedAfterYear}`);
    hints.push(`raised ${fundedAfterYear}`);
  }
  if (/foreign currency|pays in (usd|gbp|euro|sgd|aud)|usd|gbp|eur/.test(lower)) {
    hints.push("international salary");
  }
  if (/small team|early.?stage|seed|series a/.test(lower)) {
    hints.push("seed startup");
    hints.push("early stage");
  }
  if (/recently funded/.test(lower)) hints.push("recently funded");
  return hints;
}

function hardConstraintsFrom(args: {
  targetRoles: string[];
  markets: string[];
  remotePreference: string;
  seniority: string;
  notes: string;
  companySizes: string[];
  industries: string[];
  preferAi: boolean;
  fundedAfterYear: number;
  europePreferred: boolean;
}): string[] {
  const rules: string[] = [];
  if (args.targetRoles.length) {
    rules.push(
      `ONLY search lanes and employers for these candidate roles (or close synonyms): ${args.targetRoles.join(", ")}. Do not invent unrelated role tracks.`,
    );
  }
  if (args.seniority) {
    rules.push(`Seniority bar: ${args.seniority}. Prefer senior/lead/founding titles; skip junior/intern tracks.`);
  }
  if (args.europePreferred) {
    rules.push(
      "Strong geo preference: European companies (EU/UK/EMEA HQ or Europe-primary) first. Remote-from-India is OK when the employer is European or global-remote.",
    );
  } else if (args.markets.length) {
    rules.push(`Prefer employers in / hiring across: ${args.markets.slice(0, 8).join(", ")}.`);
  }
  if (args.remotePreference) {
    rules.push(`Remote policy must fit: ${args.remotePreference}.`);
  }
  if (args.companySizes.length) {
    rules.push(`Company size: ${args.companySizes.join(", ")} only (small team). Reject clear enterprise (1000+) unless unknown.`);
  }
  if (args.preferAi) {
    rules.push(
      "Industry: strong preference for AI / AI-product companies. Other industries only when the role and company still clearly fit; do not fill lanes with random Healthtech/Fintech/Blockchain just to diversify.",
    );
  } else if (args.industries.length) {
    rules.push(`Prefer industries: ${args.industries.join(", ")}.`);
  }
  if (args.fundedAfterYear > 0) {
    rules.push(
      `Prefer recently funded / raised in ${args.fundedAfterYear} (or ${args.fundedAfterYear - 1}+ if exact year unknown). Skip long-stable enterprises with no early-stage signal.`,
    );
  }
  if (args.notes.trim()) {
    rules.push(`User notes (must respect): ${args.notes.trim().slice(0, 500)}`);
  }
  return rules;
}

/** Build the brief from a job playbook's workflow_data (+ nested preferences). */
export function jobSearchBriefFromProject(project: {
  workflow_type: string;
  workflow_data: string;
}): JobSearchBrief | null {
  if (project.workflow_type !== "job_outreach") return null;
  const data = parseWorkflowData(project.workflow_data);
  const prefs = asRecord(data.preferences);

  const targetRoles = stringList(data.targetRoles);
  const locations = stringList(data.locations);
  const remotePreference =
    typeof data.remotePreference === "string" ? data.remotePreference.trim() : "";
  const seniority = typeof data.seniority === "string" ? data.seniority.trim() : "";
  const notes = typeof data.notes === "string" ? data.notes.trim() : "";

  const companySizes = normalizeCompanySizes(
    prefs.companySizes ?? data.companySizes ?? [],
  );
  const { industries, preferAi } = normalizeJobIndustries(
    prefs.industries ?? data.industries ?? [],
  );
  // Notes can also imply AI even if prefs didn't.
  const notesPreferAi =
    preferAi || /\bai\b|artificial intelligence/.test(notes.toLowerCase());
  const fundedAfterYear = fundedYearFromText(notes);
  const markets = orderMarkets(locations, notes, remotePreference);
  const europe = europePreferred(locations, notes);
  const queryHints = queryHintsFromNotes(notes, notesPreferAi, fundedAfterYear);

  return {
    targetRoles,
    locations,
    markets,
    remotePreference,
    seniority,
    notes,
    companySizes,
    industries: notesPreferAi && industries.length === 0 ? ["AI"] : industries,
    preferAi: notesPreferAi,
    fundedAfterYear,
    queryHints,
    hardConstraints: hardConstraintsFrom({
      targetRoles,
      markets,
      remotePreference,
      seniority,
      notes,
      companySizes,
      industries: notesPreferAi && industries.length === 0 ? ["AI"] : industries,
      preferAi: notesPreferAi,
      fundedAfterYear,
      europePreferred: europe,
    }),
  };
}

/** Compact prompt block for LLMs. */
export function jobBriefPromptBlock(brief: JobSearchBrief): string {
  return `MUST-FOLLOW job-search brief from the candidate's setup form (do not ignore or dilute):
${JSON.stringify(
  {
    targetRoles: brief.targetRoles,
    locations: brief.locations,
    preferredMarkets: brief.markets,
    remotePreference: brief.remotePreference,
    seniority: brief.seniority,
    companySizes: brief.companySizes,
    industries: brief.industries,
    preferAi: brief.preferAi,
    fundedAfterYear: brief.fundedAfterYear || null,
    notes: brief.notes,
    hardConstraints: brief.hardConstraints,
  },
  null,
  2,
)}`;
}
