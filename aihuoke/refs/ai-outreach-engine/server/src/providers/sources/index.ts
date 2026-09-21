import { portfolioSources } from "./portfolios";
import type { CompanySource, SourceCompany, SourceFetchArgs } from "./types";
import { ycSource } from "./yc";
import { matchesCompanySizes, matchesFundedAfter, yearFromBatch } from "../companyFilters";

export type { SourceCompany, SourceFetchArgs, CompanySource } from "./types";
export { SOURCE_ADAPTER_OPTIONS, DEFAULT_SOURCE_ADAPTERS } from "./catalog";

const ALL_SOURCES: CompanySource[] = [ycSource, ...portfolioSources];

/**
 * Pick adapters from explicit audience selection, or fall back to campaign signals.
 * YC preferred for venture-backed customer campaigns when auto-selecting.
 */
export function selectCompanySources(
  signals: string[],
  sourceIds?: string[],
): CompanySource[] {
  const explicit = (sourceIds ?? []).filter((id) => id && id !== "web");
  if (explicit.length > 0) {
    const wanted = new Set(explicit);
    const matched = ALL_SOURCES.filter((source) => wanted.has(source.id));
    const order = ["yc", "techstars", "500global", "antler", "ef", "surge", "producthunt"];
    return matched.sort((a, b) => order.indexOf(a.id) - order.indexOf(b.id));
  }

  const matched = ALL_SOURCES.filter((source) => source.matches(signals));
  const hay = signals.map((s) => s.toLowerCase()).join(" ");
  const venture =
    /seed|series a|series b|venture|pre-seed|yc|techstars|antler|500|entrepreneur/.test(hay) ||
    signals.length === 0;
  if (venture && !matched.some((s) => s.id === "yc")) {
    matched.unshift(ycSource);
  }
  const order = ["yc", "techstars", "500global", "antler", "ef", "surge", "producthunt"];
  return matched.sort((a, b) => order.indexOf(a.id) - order.indexOf(b.id));
}

function passesAudienceFilters(
  company: SourceCompany,
  args: SourceFetchArgs,
): boolean {
  if (
    !matchesCompanySizes(company.sizeHint, undefined, args.companySizes ?? [])
  ) {
    return false;
  }
  const year = yearFromBatch(company.batch);
  if (!matchesFundedAfter(year, args.fundedAfterYear ?? 0)) return false;
  return true;
}

/**
 * Pull companies from structured / portfolio sources until `limit` is reached.
 * Failures in one adapter do not abort the others.
 */
export async function fetchFromCompanySources(
  args: SourceFetchArgs & { signals: string[]; sourceIds?: string[] },
): Promise<{ companies: SourceCompany[]; sourceCounts: Record<string, number> }> {
  const sources = selectCompanySources(args.signals, args.sourceIds);
  const companies: SourceCompany[] = [];
  const seen = new Set(args.excludeDomains.map((d) => d.toLowerCase()));
  const sourceCounts: Record<string, number> = {};

  if (sources.length === 0) {
    return { companies, sourceCounts };
  }

  // Budget per source so one adapter cannot eat the whole batch.
  const perSource = Math.max(6, Math.ceil(args.limit / Math.max(sources.length, 1)));

  for (const source of sources) {
    if (companies.length >= args.limit) break;
    try {
      const batch = await source.fetch({
        ...args,
        limit: Math.min(perSource, args.limit - companies.length),
        excludeDomains: [...seen],
      });
      let added = 0;
      for (const company of batch) {
        const domain = company.domain.toLowerCase();
        if (!domain || seen.has(domain)) continue;
        if (!passesAudienceFilters(company, args)) continue;
        seen.add(domain);
        companies.push(company);
        added += 1;
        if (companies.length >= args.limit) break;
      }
      sourceCounts[source.id] = (sourceCounts[source.id] ?? 0) + added;
    } catch (error) {
      console.warn(
        `[sources] ${source.id} failed:`,
        error instanceof Error ? error.message : error,
      );
      sourceCounts[source.id] = sourceCounts[source.id] ?? 0;
    }
  }

  return { companies, sourceCounts };
}
