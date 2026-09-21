import { completeJson } from "../../llm/client";
import { EXTRACT_COMPANIES_SYSTEM, extractCompaniesUser } from "../../llm/prompts";
import { webSearch } from "../search";
import type { CompanySource, SourceCompany, SourceFetchArgs } from "./types";
import { domainFromWebsite, matchesAnySignal } from "./types";

type PortfolioDef = {
  id: string;
  label: string;
  signalNeedles: string[];
  /** Always include for customer outreach when true (high-signal AI/SaaS feed). */
  always?: boolean;
  queries: string[];
};

const PORTFOLIOS: PortfolioDef[] = [
  {
    id: "techstars",
    label: "Techstars",
    signalNeedles: ["techstars"],
    queries: [
      "Techstars portfolio companies B2B SaaS",
      "Techstars portfolio AI startups",
      "site:techstars.com companies portfolio",
    ],
  },
  {
    id: "500global",
    label: "500 Global",
    signalNeedles: ["500 global", "500 startups", "500global"],
    queries: [
      "500 Global portfolio companies B2B SaaS",
      "500 Startups portfolio AI companies",
      "site:500.co companies",
    ],
  },
  {
    id: "antler",
    label: "Antler",
    signalNeedles: ["antler"],
    queries: [
      "Antler portfolio companies B2B SaaS",
      "Antler portfolio AI startups",
      "site:antler.co portfolio companies",
    ],
  },
  {
    id: "ef",
    label: "Entrepreneur First",
    signalNeedles: ["ef", "entrepreneur first", "joinef"],
    queries: [
      "Entrepreneur First portfolio companies",
      "EF portfolio startups B2B SaaS",
      "site:joinef.com companies",
    ],
  },
  {
    id: "surge",
    label: "Surge",
    signalNeedles: ["surge"],
    queries: [
      "Surge accelerator portfolio companies",
      "Sequoia Surge portfolio startups",
      "Surge portfolio B2B SaaS AI",
    ],
  },
  {
    id: "producthunt",
    label: "Product Hunt",
    signalNeedles: ["product hunt", "producthunt", "fast-growing ai"],
    always: true,
    queries: [
      "site:producthunt.com AI SaaS B2B",
      "Product Hunt AI meeting assistant startups",
      "site:producthunt.com developer tools SaaS",
    ],
  },
];

type Extracted = {
  companies: Array<{
    name: string;
    domain: string;
    description?: string;
    sizeHint?: string;
    country?: string;
    sourceUrl?: string;
  }>;
};

async function fetchPortfolioCompanies(
  def: PortfolioDef,
  args: SourceFetchArgs,
): Promise<SourceCompany[]> {
  const query = def.queries[(args.page - 1) % def.queries.length]!;
  // Season with one industry when available.
  const industry = args.industries[(args.page - 1) % Math.max(args.industries.length, 1)] ?? "";
  const q = [query, industry].filter(Boolean).join(" ").trim();

  const results = await webSearch(q, {
    limit: 20,
    page: Math.min(args.page, 5),
    projectId: args.projectId,
  });
  if (results.length === 0) return [];

  const extracted = await completeJson<Extracted>({
    system: EXTRACT_COMPANIES_SYSTEM,
    user: extractCompaniesUser({
      audienceName: args.audienceName,
      jobTitles: args.jobTitles,
      industries: args.industries,
      companySizes: [],
      targetMarkets: args.markets,
      targetSignals: args.signals,
      query: q,
      results,
    }),
    tier: "cheap",
    projectId: args.projectId,
    label: `extract_${def.id}`,
  });

  const exclude = new Set(args.excludeDomains.map((d) => d.toLowerCase()));
  const out: SourceCompany[] = [];
  const seen = new Set<string>();

  for (const company of extracted.companies ?? []) {
    const domain = domainFromWebsite(company.domain || "");
    if (!domain || exclude.has(domain) || seen.has(domain)) continue;
    // Skip the accelerator/directory hosts themselves.
    if (
      /(techstars\.com|500\.co|antler\.co|joinef\.com|producthunt\.com|ycombinator\.com)$/.test(
        domain,
      )
    ) {
      continue;
    }
    seen.add(domain);
    out.push({
      name: company.name,
      domain,
      description: company.description,
      sizeHint: company.sizeHint,
      country: company.country,
      sourceUrl: company.sourceUrl,
      source: def.id,
    });
    if (out.length >= args.limit) break;
  }
  return out;
}

export const portfolioSources: CompanySource[] = PORTFOLIOS.map((def) => ({
  id: def.id,
  label: def.label,
  matches: (signals) => def.always === true || matchesAnySignal(signals, def.signalNeedles),
  fetch: (args) => fetchPortfolioCompanies(def, args),
}));
