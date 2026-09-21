import type { CompanySource, SourceCompany, SourceFetchArgs } from "./types";
import { domainFromWebsite, matchesAnySignal, sizeHintFromTeamSize } from "./types";
import {
  matchesCompanySizes,
  matchesFundedAfter,
  yearFromBatch,
} from "../companyFilters";

const YC_HIRING_URL = "https://yc-oss.github.io/api/companies/hiring.json";
const YC_META_URL = "https://yc-oss.github.io/api/meta.json";
const CACHE_TTL_MS = 6 * 60 * 60 * 1000;

type YcCompany = {
  name?: string;
  website?: string;
  one_liner?: string;
  long_description?: string;
  team_size?: number;
  industry?: string;
  subindustry?: string;
  industries?: string[];
  tags?: string[];
  regions?: string[];
  batch?: string;
  status?: string;
  isHiring?: boolean;
  url?: string;
};

type CacheEntry<T> = { at: number; data: T };

let hiringCache: CacheEntry<YcCompany[]> | null = null;
let metaCache: CacheEntry<Record<string, unknown>> | null = null;
const listCache = new Map<string, CacheEntry<YcCompany[]>>();

async function fetchJson<T>(url: string): Promise<T> {
  const res = await fetch(url, {
    headers: { Accept: "application/json", "User-Agent": "outreach-engine/0.1" },
    signal: AbortSignal.timeout(60_000),
  });
  if (!res.ok) throw new Error(`YC source ${res.status} for ${url}`);
  return (await res.json()) as T;
}

async function loadHiring(): Promise<YcCompany[]> {
  if (hiringCache && Date.now() - hiringCache.at < CACHE_TTL_MS) return hiringCache.data;
  const data = await fetchJson<YcCompany[]>(YC_HIRING_URL);
  hiringCache = { at: Date.now(), data };
  return data;
}

async function loadMeta(): Promise<Record<string, unknown>> {
  if (metaCache && Date.now() - metaCache.at < CACHE_TTL_MS) return metaCache.data;
  const data = await fetchJson<Record<string, unknown>>(YC_META_URL);
  metaCache = { at: Date.now(), data };
  return data;
}

async function loadList(url: string): Promise<YcCompany[]> {
  const hit = listCache.get(url);
  if (hit && Date.now() - hit.at < CACHE_TTL_MS) return hit.data;
  const data = await fetchJson<YcCompany[]>(url);
  listCache.set(url, { at: Date.now(), data });
  return data;
}

/** Map audience industry phrases onto yc-oss industry/tag slugs. */
function resolveYcSlices(industries: string[], meta: Record<string, unknown>): string[] {
  const industryIndex = (meta.industries ?? {}) as Record<string, { api?: string; name?: string }>;
  const tagIndex = (meta.tags ?? {}) as Record<string, { api?: string; name?: string }>;
  const urls = new Set<string>();

  const aliases: Record<string, string[]> = {
    "b2b saas": ["b2b", "productivity", "saas"],
    saas: ["b2b", "productivity"],
    "developer tools": ["engineering-product-and-design", "infrastructure", "developer-tools"],
    "dev tools": ["engineering-product-and-design", "infrastructure"],
    recruiting: ["recruiting-and-talent", "human-resources"],
    hr: ["human-resources", "recruiting-and-talent"],
    "sales tech": ["sales", "marketing"],
    sales: ["sales"],
    "customer success": ["operations", "office-management"],
    design: ["engineering-product-and-design"],
    "product management": ["engineering-product-and-design", "productivity"],
    ai: ["ai", "ai-assistant"],
    "fast-growing ai": ["ai", "ai-assistant"],
    security: ["security"],
    fintech: ["fintech", "finance-and-accounting"],
    productivity: ["productivity"],
  };

  for (const raw of industries) {
    const key = raw.toLowerCase().trim();
    const candidates = aliases[key] ?? [
      key.replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, ""),
      key.replace(/\s+/g, "-"),
    ];
    for (const slug of candidates) {
      const industry = industryIndex[slug];
      if (industry?.api) urls.add(industry.api);
      const tag = tagIndex[slug];
      if (tag?.api) urls.add(tag.api);
    }
  }

  // Always blend AI + B2B productivity for meeting/assistant style products when empty.
  if (urls.size === 0) {
    for (const slug of ["b2b", "productivity", "ai", "ai-assistant", "sales", "recruiting-and-talent"]) {
      const industry = industryIndex[slug];
      if (industry?.api) urls.add(industry.api);
      const tag = tagIndex[slug];
      if (tag?.api) urls.add(tag.api);
    }
  }

  return [...urls].slice(0, 6);
}

function haystack(company: YcCompany): string {
  return [
    company.name,
    company.one_liner,
    company.long_description,
    company.industry,
    company.subindustry,
    ...(company.industries ?? []),
    ...(company.tags ?? []),
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
}

function marketMatch(company: YcCompany, markets: string[]): boolean {
  if (markets.length === 0) return true;
  const regions = (company.regions ?? []).join(" ").toLowerCase();
  const locations = regions;
  if (!locations.trim()) return true; // unknown geo — keep
  const aliases: Record<string, string[]> = {
    "united states": ["united states", "usa", "america / canada", "us"],
    us: ["united states", "usa", "america / canada"],
    canada: ["canada", "america / canada"],
    "united kingdom": ["united kingdom", "uk", "europe"],
    uk: ["united kingdom", "uk", "europe"],
    germany: ["germany", "europe"],
    netherlands: ["netherlands", "europe"],
    singapore: ["singapore", "southeast asia", "asia"],
    australia: ["australia"],
    "latin america": ["latin america", "latam", "south america"],
    europe: ["europe"],
    uae: ["uae", "middle east", "dubai"],
  };
  return markets.some((market) => {
    const key = market.toLowerCase();
    const needles = aliases[key] ?? [key];
    return needles.some((n) => locations.includes(n));
  });
}

function relevanceScore(company: YcCompany, industries: string[], audienceName: string): number {
  const text = haystack(company);
  let score = 0;
  if (company.isHiring) score += 2;
  if (company.status?.toLowerCase() === "active") score += 1;
  for (const industry of industries) {
    const token = industry.toLowerCase();
    if (token && text.includes(token)) score += 3;
    for (const part of token.split(/[^a-z0-9]+/).filter((p) => p.length > 2)) {
      if (text.includes(part)) score += 1;
    }
  }
  for (const part of audienceName.toLowerCase().split(/[^a-z0-9]+/).filter((p) => p.length > 3)) {
    if (text.includes(part)) score += 1;
  }
  // Prefer recent-ish batches lightly via Winter/Summer year in batch string.
  const year = Number((company.batch ?? "").match(/(20\d{2})/)?.[1] ?? 0);
  if (year >= 2022) score += 2;
  else if (year >= 2018) score += 1;
  return score;
}

function toSourceCompany(company: YcCompany): SourceCompany | null {
  const domain = domainFromWebsite(company.website ?? "");
  if (!domain || !company.name) return null;
  return {
    name: company.name,
    domain,
    description: company.one_liner || company.long_description?.slice(0, 240) || "",
    sizeHint: sizeHintFromTeamSize(company.team_size),
    country: "",
    sourceUrl: company.url || company.website,
    source: "yc",
    batch: company.batch,
    tags: company.tags,
  };
}

export const ycSource: CompanySource = {
  id: "yc",
  label: "Y Combinator",
  matches: (signals) =>
    matchesAnySignal(signals, ["yc", "y combinator", "ycombinator"]) || signals.length === 0,
  async fetch(args: SourceFetchArgs): Promise<SourceCompany[]> {
    const [hiring, meta] = await Promise.all([loadHiring(), loadMeta()]);
    const sliceUrls = resolveYcSlices(args.industries, meta);
    const sliced = (
      await Promise.all(sliceUrls.map((url) => loadList(url).catch(() => [] as YcCompany[])))
    ).flat();

    const byWebsite = new Map<string, YcCompany>();
    for (const company of [...hiring, ...sliced]) {
      const domain = domainFromWebsite(company.website ?? "");
      if (!domain) continue;
      if (!byWebsite.has(domain)) byWebsite.set(domain, company);
    }

    const exclude = new Set(args.excludeDomains.map((d) => d.toLowerCase()));
    const sizes = args.companySizes ?? [];
    const fundedAfter = args.fundedAfterYear ?? 0;
    const ranked = [...byWebsite.values()]
      .filter((company) => {
        const domain = domainFromWebsite(company.website ?? "");
        if (!domain || exclude.has(domain)) return false;
        if ((company.status ?? "").toLowerCase() === "inactive") return false;
        if (!marketMatch(company, args.markets)) return false;
        const hint = sizeHintFromTeamSize(company.team_size);
        if (!matchesCompanySizes(hint, company.team_size, sizes)) return false;
        if (!matchesFundedAfter(yearFromBatch(company.batch), fundedAfter)) return false;
        return true;
      })
      .map((company) => ({
        company,
        score: relevanceScore(company, args.industries, args.audienceName),
      }))
      .filter((row) => row.score >= 2)
      .sort((a, b) => b.score - a.score);

    const offset = Math.max(0, (args.page - 1) * args.limit);
    return ranked
      .slice(offset, offset + args.limit)
      .map((row) => toSourceCompany(row.company))
      .filter((row): row is SourceCompany => Boolean(row));
  },
};
