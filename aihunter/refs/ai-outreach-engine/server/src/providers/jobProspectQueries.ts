import { finalizeSearchQuery, quoteTerm, sanitizeQueryTerm } from "./searchQuerySafe";
import type { ProspectQueryPlan } from "./customerProspectQueries";

/**
 * Deterministic job-search query plan.
 *
 * Jobs are NOT customer ICPs. Each "audience" is a search lane:
 *   - targetRoles = roles the candidate wants (search for these openings)
 *   - jobTitles   = hiring contacts to email later (NOT used as search roles)
 *
 * Mixes:
 *   1) open-role / hiring queries for targetRoles × industry × location
 *   2) careers-page / "we're hiring" employer discovery
 *   3) sanitized LLM searchQueries from the lane
 *   4) job-board style discovery (Wellfound / YC jobs / LinkedIn Jobs SERPs)
 *      — we extract the EMPLOYER domain from those results, never prospect the board
 */
export function planJobSearchQueries(args: {
  audienceName: string;
  /** Roles the candidate wants — primary search seed. */
  targetRoles: string[];
  industries: string[];
  companySizes: string[];
  searchQueries: string[];
  /** Locations / remote prefs from campaign. */
  markets: string[];
  /** Soft signals like "Seed hiring", industry hiring. */
  signals: string[];
  /** Extra constraint phrases from setup notes (funded year, AI, Europe…). */
  queryHints?: string[];
  page: number;
  maxQueries?: number;
}): ProspectQueryPlan {
  const maxQueries = args.maxQueries ?? 8;
  const page = Math.max(1, args.page);
  const rotate = page - 1;

  const markets = args.markets.map(sanitizeQueryTerm).filter(Boolean);
  const signals = args.signals.map(sanitizeQueryTerm).filter(Boolean);
  const industries = args.industries.map(sanitizeQueryTerm).filter(Boolean);
  const roles = args.targetRoles.map(sanitizeQueryTerm).filter(Boolean);
  const sizes = args.companySizes.map(sanitizeQueryTerm).filter(Boolean);
  const hints = (args.queryHints ?? []).map(sanitizeQueryTerm).filter(Boolean);

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

  const roleA =
    pick(roles, rotate) ?? sanitizeQueryTerm(args.audienceName) ?? "engineer";
  const roleB = pick(roles, rotate + 1);
  const industry = pick(industries, rotate) ?? (hints.some((h) => /\bai\b/i.test(h)) ? "AI" : "startup");
  const market = pick(markets, rotate);
  const marketAlt = pick(markets, rotate + 2);
  const signal = pick(signals, rotate);
  const size = pick(sizes, rotate);
  const hint = pick(hints, rotate);
  const hintAlt = pick(hints, rotate + 1);

  // ── 1. Open role / hiring for the candidate's target roles ──
  push(
    [
      "hiring",
      quoteTerm(roleA),
      quoteTerm(industry),
      market ? quoteTerm(market) : "",
      hint ? quoteTerm(hint) : "",
    ],
    "open_role",
  );
  push(
    [quoteTerm(roleA), "job", quoteTerm(industry), signal ? quoteTerm(signal) : "hiring"],
    "open_role",
  );
  if (roleB && roleB.toLowerCase() !== roleA.toLowerCase()) {
    push(
      [
        "hiring",
        quoteTerm(roleB),
        quoteTerm(industry),
        marketAlt ? quoteTerm(marketAlt) : "",
        hintAlt ? quoteTerm(hintAlt) : "",
      ],
      "open_role",
    );
  }

  // ── 2. Careers / we're-hiring employer discovery ──
  push(
    [quoteTerm(industry), "careers", quoteTerm(roleA), market ? quoteTerm(market) : ""],
    "careers_page",
  );
  push(
    [quoteTerm(industry), "startup", "we're hiring", quoteTerm(roleA), market ? quoteTerm(market) : ""],
    "careers_page",
  );
  if (size) {
    push(
      [quoteTerm(roleA), "hiring", quoteTerm(size), "employees", quoteTerm(industry)],
      "size_hiring",
    );
  }

  // ── 3. Job-board style SERPs (extract employer from snippets, never email the board) ──
  const boards = ["Wellfound", "YC jobs", "LinkedIn Jobs"];
  const board = boards[Math.abs(rotate) % boards.length]!;
  push(
    [
      quoteTerm(roleA),
      quoteTerm(industry),
      board,
      market ? quoteTerm(market) : "Remote",
      hint ? quoteTerm(hint) : "",
    ],
    "job_board",
  );

  // ── 4. Setup-brief constraint query (funded year / AI / Europe…) ──
  if (hint) {
    push(
      ["hiring", quoteTerm(roleA), quoteTerm(hint), market ? quoteTerm(market) : quoteTerm(industry)],
      "setup_brief",
    );
  }

  // ── 5. LLM lane queries (already should be hiring/employer-finding) ──
  for (const [index, base] of args.searchQueries.slice(0, 4).entries()) {
    const cleaned = sanitizeQueryTerm(base.replace(/^["']|["']$/g, ""));
    if (!cleaned) continue;
    const parts = [cleaned];
    const lower = cleaned.toLowerCase();
    const hasRole = roles.some((r) => lower.includes(r.toLowerCase()));
    const hasMarket = markets.some((m) => lower.includes(m.toLowerCase()));
    if (!hasRole && roleA && index % 2 === 0) parts.push(quoteTerm(roleA));
    else if (!hasMarket && market) parts.push(quoteTerm(market));
    else if (hint && !lower.includes(hint.toLowerCase()) && index % 3 === 0) {
      parts.push(quoteTerm(hint));
    }
    push(parts, "audience_llm");
  }

  if (planned.length === 0) {
    push(["hiring", "engineer", "startup", pick(markets, 0) ?? "Remote"], "fallback");
  }

  const sliced = planned.slice(0, maxQueries);
  return {
    queries: sliced.map((item) => item.query),
    intents: sliced.map((item) => item.intent),
  };
}
