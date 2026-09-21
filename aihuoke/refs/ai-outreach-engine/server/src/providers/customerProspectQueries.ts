import { finalizeSearchQuery, quoteTerm, sanitizeQueryTerm } from "./searchQuerySafe";

export type ProspectQueryPlan = {
  /** Queries to run this batch (already finalized, Serper-safe). */
  queries: string[];
  /** Short labels parallel to queries, for logging/UI. */
  intents: string[];
};

type Competitor = { name: string; domain?: string };

/**
 * Deterministic customer-discovery query plan.
 *
 * LLM audience.searchQueries alone are unreliable. This layer always mixes:
 *   1) structured industry × signal × market (from Setup prefs + audience fields)
 *   2) sanitized LLM queries
 *   3) hiring + job-title employer discovery
 *   4) competitor lookalikes (find companies near competitors — not the competitors)
 *
 * That is the actual connection between Setup inputs and Serper.
 */
export function planCustomerSearchQueries(args: {
  audienceName: string;
  jobTitles: string[];
  industries: string[];
  companySizes: string[];
  searchQueries: string[];
  markets: string[];
  signals: string[];
  competitors: Competitor[];
  page: number;
  maxQueries?: number;
}): ProspectQueryPlan {
  const maxQueries = args.maxQueries ?? 8;
  const page = Math.max(1, args.page);
  const rotate = page - 1;

  const markets = args.markets.map(sanitizeQueryTerm).filter(Boolean);
  const signals = args.signals.map(sanitizeQueryTerm).filter(Boolean);
  const industries = args.industries.map(sanitizeQueryTerm).filter(Boolean);
  const titles = args.jobTitles.map(sanitizeQueryTerm).filter(Boolean);
  const sizes = args.companySizes.map(sanitizeQueryTerm).filter(Boolean);

  const pick = <T,>(items: T[], salt: number): T | undefined => {
    if (items.length === 0) return undefined;
    return items[Math.abs(salt) % items.length];
  };

  const planned: Array<{ query: string; intent: string }> = [];
  const seen = new Set<string>();

  const push = (rawParts: string[], intent: string) => {
    const query = finalizeSearchQuery(rawParts.map(sanitizeQueryTerm).filter(Boolean));
    if (!query || query.split(/\s+/).length < 2) return;
    const key = query.toLowerCase();
    if (seen.has(key)) return;
    seen.add(key);
    planned.push({ query, intent });
  };

  // ── 1. Structured prefs × audience industries (the missing link) ──
  for (let i = 0; i < 3; i++) {
    const industry = pick(industries, rotate + i) ?? "B2B SaaS";
    const signal = pick(signals, rotate + i * 3);
    const market = pick(markets, rotate + i * 5);
    // One signal + one market max — never stack the whole prefs list.
    const parts = [quoteTerm(industry), "startups"];
    if (signal) parts.push(quoteTerm(signal));
    if (market && i !== 1) parts.push(quoteTerm(market)); // vary: some queries skip geo
    if (!market && sizes[0] && i === 1) parts.push(quoteTerm(sizes[0]));
    push(parts, "structured_prefs");
  }

  // ── 2. LLM audience queries (light touch — already should be employer-finding) ──
  for (const [index, base] of args.searchQueries.slice(0, 5).entries()) {
    const cleaned = sanitizeQueryTerm(base.replace(/^["']|["']$/g, ""));
    if (!cleaned) continue;
    const market = pick(markets, rotate + index + 2);
    const signal = pick(signals, rotate + index + 7);
    // Only add a missing seasoning term if the base is short/generic.
    const parts = [cleaned];
    const lower = cleaned.toLowerCase();
    const hasSignal = signals.some((s) => lower.includes(s.toLowerCase()));
    const hasMarket = markets.some((m) => lower.includes(m.toLowerCase()));
    if (!hasSignal && !hasMarket) {
      if (index % 2 === 0 && signal) parts.push(quoteTerm(signal));
      else if (market) parts.push(quoteTerm(market));
    }
    push(parts, "audience_llm");
  }

  // ── 3. Hiring + job title → employers that hire this persona ──
  const titleA = pick(titles, rotate);
  const titleB = pick(titles, rotate + 1);
  const industryForHire = pick(industries, rotate + 1) ?? "B2B SaaS";
  const signalForHire = pick(signals, rotate + 4);
  if (titleA) {
    push(
      [
        "hiring",
        quoteTerm(titleA),
        quoteTerm(industryForHire),
        signalForHire ? quoteTerm(signalForHire) : "",
      ],
      "hiring_title",
    );
  }
  if (titleB && titleB !== titleA) {
    const market = pick(markets, rotate + 3);
    push(
      ["hiring", quoteTerm(titleB), quoteTerm(industryForHire), market ? quoteTerm(market) : "startup"],
      "hiring_title",
    );
  }

  // ── 4. Competitor lookalikes — find companies near competitors, not competitors ──
  const comps = args.competitors.filter((c) => c.name.trim()).slice(0, 6);
  for (let i = 0; i < Math.min(2, comps.length); i++) {
    const comp = comps[(rotate + i) % comps.length]!;
    const industry = pick(industries, rotate + i) ?? "B2B SaaS";
    // SERPs for "alternatives to X" / "companies like X" list peer products AND
    // often roundups that name many employers in the same buyer market.
    push(
      [`companies like`, quoteTerm(comp.name), quoteTerm(industry), "startup"],
      "competitor_lookalike",
    );
  }

  // Fallback if everything was empty (should not happen with defaults).
  if (planned.length === 0) {
    push(["B2B SaaS", "startups", pick(signals, 0) ?? "Seed", pick(markets, 0) ?? "United States"], "fallback");
  }

  const sliced = planned.slice(0, maxQueries);
  return {
    queries: sliced.map((item) => item.query),
    intents: sliced.map((item) => item.intent),
  };
}

/** Domains we should never prospect (our product + competitors). */
export function blockedProspectDomains(args: {
  ownDomains: string[];
  competitors: Competitor[];
}): string[] {
  const out = new Set<string>();
  for (const domain of args.ownDomains) {
    const clean = domain.toLowerCase().replace(/^www\./, "").trim();
    if (clean) out.add(clean);
  }
  for (const competitor of args.competitors) {
    const clean = (competitor.domain ?? "").toLowerCase().replace(/^www\./, "").trim();
    if (clean) out.add(clean);
  }
  return [...out];
}
