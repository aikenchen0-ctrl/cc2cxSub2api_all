import type { WorkflowType } from "../db/types";
import { sanitizeSearchTerms } from "../workflow/preferences";
import { finalizeSearchQuery, quoteTerm, sanitizeQueryTerm } from "./searchQuerySafe";

/**
 * Light enrichment for a single base query.
 *
 * Prefer planCustomerSearchQueries for customer outreach batches — that planner
 * already mixes markets/signals/industries. This helper remains for investor/job
 * paths and as a safe fallback.
 *
 * Important: do NOT append stacks of -site: operators. Serper free accounts
 * intermittently reject those patterns; we filter hosts after search instead.
 */
export function buildCompanySearchQuery(args: {
  baseQuery: string;
  markets: string[];
  signals: string[];
  industries: string[];
  companySizes: string[];
  workflowType: WorkflowType;
  page: number;
  queryIndex: number;
}): string {
  const base = sanitizeQueryTerm(args.baseQuery);
  if (!base) return "";

  const markets = sanitizeSearchTerms(args.markets, { maxLen: 40 }).map(sanitizeQueryTerm);
  const signals = sanitizeSearchTerms(args.signals, { maxLen: 40 }).map(sanitizeQueryTerm);
  const industries = sanitizeSearchTerms(args.industries, { maxLen: 40 }).map(sanitizeQueryTerm);

  const pick = (items: string[], salt: number): string => {
    if (items.length === 0) return "";
    return items[Math.abs(salt) % items.length]!.trim();
  };

  const rotate = (args.page - 1) * 5 + args.queryIndex;
  const industry = pick(industries, rotate);
  const market = pick(markets, rotate);
  const signal = pick(signals, rotate);

  const parts: string[] = [base];
  const baseLower = base.toLowerCase();

  const baseHasSignal = signals.some((term) => containsTerm(baseLower, term));
  const baseHasMarket = markets.some((term) => containsTerm(baseLower, term));

  if (!(baseHasSignal && baseHasMarket)) {
    const candidates = [signal, market, industry].filter(Boolean);
    const start = Math.abs(rotate) % Math.max(candidates.length, 1);
    for (let offset = 0; offset < candidates.length; offset++) {
      const term = candidates[(start + offset) % candidates.length]!;
      if (!term || containsTerm(baseLower, term)) continue;
      parts.push(quoteTerm(term));
      break;
    }
  }

  // workflowType kept for API compat; site exclusions are post-filtered now.
  void args.workflowType;
  void args.companySizes;

  return finalizeSearchQuery(parts);
}

function containsTerm(haystackLower: string, term: string): boolean {
  const needle = term.trim().toLowerCase();
  if (!needle) return false;
  const aliases: Record<string, string[]> = {
    us: ["us", "usa", "united states", "u.s."],
    usa: ["us", "usa", "united states"],
    "united states": ["us", "usa", "united states"],
    uk: ["uk", "united kingdom", "britain", "england"],
    "united kingdom": ["uk", "united kingdom", "britain"],
    uae: ["uae", "united arab emirates", "dubai"],
    "united arab emirates": ["uae", "united arab emirates", "dubai"],
    yc: ["yc", "y combinator", "ycombinator"],
    "y combinator": ["yc", "y combinator", "ycombinator"],
    "latin america": ["latin america", "latam", "lat am"],
    latam: ["latin america", "latam"],
    netherlands: ["netherlands", "holland", "amsterdam"],
  };
  const variants = aliases[needle] ?? [needle];
  return variants.some((variant) => haystackLower.includes(variant));
}
