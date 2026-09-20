import { completeJson } from "../../llm/client";
import {
  EXTRACT_COMPANIES_SYSTEM,
  EXTRACT_INVESTOR_TARGETS_SYSTEM,
  EXTRACT_JOB_TARGETS_SYSTEM,
  QUALIFY_COMPANIES_SYSTEM,
  QUALIFY_INVESTOR_TARGETS_SYSTEM,
  QUALIFY_JOB_TARGETS_SYSTEM,
  extractCompaniesUser,
  qualifyCompaniesUser,
} from "../../llm/prompts";
import { normalizeSiteAnalysis } from "../../db/analysis";
import { buildCompanySearchQuery } from "../../providers/companySearchQuery";
import {
  blockedProspectDomains,
  planCustomerSearchQueries,
} from "../../providers/customerProspectQueries";
import { planJobSearchQueries } from "../../providers/jobProspectQueries";
import { peekHomepage } from "../../providers/crawler";
import { webSearch } from "../../providers/search";
import { fetchFromCompanySources, type SourceCompany } from "../../providers/sources";
import { matchesCompanySizes } from "../../providers/companyFilters";
import { verificationGate } from "../../providers/verifyGate";
import { audiences, companies, events, jobs, projects, siteProfiles } from "../../db/repo";
import { searchEnrichment } from "../../workflow/preferences";
import {
  jobBriefPromptBlock,
  jobSearchBriefFromProject,
} from "../../workflow/jobBrief";
import { throwIfCancelled } from "../context";
import { JobCancelled } from "../errors";
import { mapLimit } from "../../providers/http";

type ExtractedCompany = {
  name: string;
  domain: string;
  description?: string;
  sizeHint?: string;
  country?: string;
  sourceUrl?: string;
};

type ExtractedCompanies = { companies: ExtractedCompany[] };

type QualifyDecision = {
  domain: string;
  keep: boolean;
  score: number;
  reason: string;
};

/** Domains that show up constantly in search results but are never prospects. */
const NEVER_PROSPECT = [
  "linkedin.com",
  "twitter.com",
  "x.com",
  "facebook.com",
  "instagram.com",
  "youtube.com",
  "reddit.com",
  "medium.com",
  "substack.com",
  "github.com",
  "wikipedia.org",
  "crunchbase.com",
  "g2.com",
  "capterra.com",
  "producthunt.com",
  "glassdoor.com",
  "indeed.com",
  "ycombinator.com",
  "forbes.com",
  "techcrunch.com",
  "quora.com",
  "wellfound.com",
  "angel.co",
  "growthlist.co",
  "failory.com",
  "sacra.com",
];

const MIN_KEEP_SCORE = 55;
const RESULTS_PER_QUERY = 20;
const MAX_CANDIDATES_PER_QUERY = 15;
/**
 * Aim for ~50 contacts/batch: ~20 companies × ~2–4 people each after people search.
 * Structured sources (YC etc.) fill most of this; Serper is fallback.
 */
const TARGET_NEW_COMPANIES_CUSTOMER = 20;
const TARGET_NEW_COMPANIES_DEFAULT = 10;
const MAX_PAGES_PER_BATCH = 2;
const MAX_DOMAIN_RESOLVES_PER_QUERY = 6;
/** Serper free-tier deep pages are flaky; wrap before burning the batch. */
const MAX_SEARCH_PAGE = 10;

function parseWorkflowData(raw: string): Record<string, unknown> {
  try {
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === "object" ? (parsed as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

function blockedOwnDomains(project: { domain: string; workflow_data: string }): string[] {
  const data = parseWorkflowData(project.workflow_data);
  return [
    project.domain,
    typeof data.productDomain === "string" ? data.productDomain : "",
  ]
    .map((domain) => domain.toLowerCase().replace(/^www\./, ""))
    .filter(Boolean);
}

function loadCompetitors(projectId: number): Array<{ name: string; domain: string }> {
  const profile = siteProfiles.latest(projectId);
  if (!profile?.analysis) return [];
  try {
    return normalizeSiteAnalysis(JSON.parse(profile.analysis)).competitors;
  } catch {
    return [];
  }
}

function isProspectable(domain: string, blocked: string[]): boolean {
  const clean = domain.toLowerCase().replace(/^www\./, "");
  if (!/^[a-z0-9-]+(\.[a-z0-9-]+)+$/.test(clean)) return false;
  if (blocked.includes(clean)) return false;
  return !NEVER_PROSPECT.some((item) => clean === item || clean.endsWith(`.${item}`));
}

function targetLabel(workflowType: string, count: number): string {
  if (workflowType === "investor_outreach") return count === 1 ? "investor target" : "investor targets";
  if (workflowType === "job_outreach") return count === 1 ? "hiring target" : "hiring targets";
  return count === 1 ? "company" : "companies";
}

function domainFromUrl(url: string): string {
  try {
    return new URL(url).hostname.toLowerCase().replace(/^www\./, "");
  } catch {
    return "";
  }
}

function queriesForBatch(args: {
  project: { workflow_type: string; domain: string; workflow_data: string };
  audience: {
    name: string;
    targetRoles: string[];
    jobTitles: string[];
    industries: string[];
    companySizes: string[];
    searchQueries: string[];
  };
  markets: string[];
  signals: string[];
  competitors: Array<{ name: string; domain: string }>;
  page: number;
}): string[] {
  if (args.project.workflow_type === "customer_outreach") {
    return planCustomerSearchQueries({
      audienceName: args.audience.name,
      jobTitles: args.audience.jobTitles,
      industries: args.audience.industries,
      companySizes: args.audience.companySizes,
      searchQueries: args.audience.searchQueries,
      markets: args.markets,
      signals: args.signals,
      competitors: args.competitors,
      page: args.page,
      maxQueries: 8,
    }).queries;
  }

  if (args.project.workflow_type === "job_outreach") {
    const brief = jobSearchBriefFromProject(
      args.project as {
        workflow_type: "job_outreach";
        workflow_data: string;
      },
    );
    const roles =
      args.audience.targetRoles.length > 0
        ? args.audience.targetRoles
        : brief?.targetRoles ?? [];
    const industries =
      args.audience.industries.length > 0
        ? args.audience.industries
        : brief?.industries ?? [];
    const sizes =
      args.audience.companySizes.length > 0
        ? args.audience.companySizes
        : brief?.companySizes ?? [];
    return planJobSearchQueries({
      audienceName: args.audience.name,
      targetRoles: roles,
      industries,
      companySizes: sizes,
      searchQueries: args.audience.searchQueries,
      markets: brief?.markets.length ? brief.markets : args.markets,
      signals: args.signals,
      queryHints: brief?.queryHints ?? [],
      page: args.page,
      maxQueries: 8,
    }).queries;
  }

  // Investor: light enrichment path.
  return args.audience.searchQueries.slice(0, 5).map((base, index) =>
    buildCompanySearchQuery({
      baseQuery: base,
      markets: args.markets,
      signals: args.signals,
      industries: args.audience.industries,
      companySizes: args.audience.companySizes,
      workflowType: "investor_outreach",
      page: args.page,
      queryIndex: index,
    }),
  );
}

/**
 * Structured portfolios (YC especially) are already high-signal — skip the heavy
 * LLM qualifier. Drop only parked/dead homepages.
 */
async function acceptStructuredCompanies(
  candidates: SourceCompany[],
): Promise<SourceCompany[]> {
  if (candidates.length === 0) return [];
  const peeks = await mapLimit(candidates, 6, async (candidate) => {
    const peek = await peekHomepage(candidate.domain).catch(() => null);
    return { domain: candidate.domain, peek };
  });
  const peekByDomain = new Map(
    peeks
      .filter((entry): entry is { domain: string; peek: Awaited<ReturnType<typeof peekHomepage>> } =>
        Boolean(entry),
      )
      .map((entry) => [entry.domain, entry.peek] as const),
  );

  const kept: SourceCompany[] = [];
  for (const candidate of candidates) {
    const peek = peekByDomain.get(candidate.domain);
    const blob = `${peek?.title ?? ""} ${peek?.excerpt ?? ""}`;
    if (/domain for sale|buy this domain|parked free|coming soon/i.test(blob)) continue;
    // Trust YC even when peek fails (some sites block bots); other sources need a peek.
    if (!peek && candidate.source !== "yc") continue;
    kept.push(candidate);
  }
  return kept;
}

async function resolveMissingDomains(args: {
  projectId: number;
  blocked: string[];
  candidates: ExtractedCompany[];
}): Promise<ExtractedCompany[]> {
  const resolved: ExtractedCompany[] = [];
  let resolves = 0;

  for (const candidate of args.candidates) {
    const domain = (candidate.domain ?? "").toLowerCase().replace(/^www\./, "");
    if (domain && isProspectable(domain, args.blocked)) {
      resolved.push({ ...candidate, domain });
      continue;
    }
    if (!candidate.name?.trim() || resolves >= MAX_DOMAIN_RESOLVES_PER_QUERY) continue;

    throwIfCancelled();
    resolves += 1;
    // One name per query — OR-chains are a common Serper free-tier reject.
    const results = await webSearch(`${candidate.name.trim()} official website`, {
      limit: 5,
      projectId: args.projectId,
    }).catch(() => []);

    let found = "";
    for (const result of results) {
      const host = domainFromUrl(result.link);
      if (host && isProspectable(host, args.blocked)) {
        found = host;
        break;
      }
    }
    if (!found) continue;
    resolved.push({
      ...candidate,
      domain: found,
      sourceUrl: candidate.sourceUrl || results[0]?.link || "",
    });
  }

  return resolved;
}

async function qualifyCandidates(args: {
  projectId: number;
  workflowType: string;
  audienceName: string;
  jobTitles: string[];
  targetRoles?: string[];
  industries: string[];
  companySizes: string[];
  markets: string[];
  signals: string[];
  jobBriefBlock?: string;
  candidates: ExtractedCompany[];
}): Promise<ExtractedCompany[]> {
  if (args.candidates.length === 0) return [];

  const peeks = await mapLimit(args.candidates, 4, async (candidate) => {
    const peek = await peekHomepage(candidate.domain).catch(() => null);
    return { domain: candidate.domain, peek };
  });
  const peekByDomain = new Map(
    peeks
      .filter((entry): entry is { domain: string; peek: Awaited<ReturnType<typeof peekHomepage>> } =>
        Boolean(entry),
      )
      .map((entry) => [entry.domain, entry.peek] as const),
  );

  const qualifySystem =
    args.workflowType === "investor_outreach"
      ? QUALIFY_INVESTOR_TARGETS_SYSTEM
      : args.workflowType === "job_outreach"
        ? QUALIFY_JOB_TARGETS_SYSTEM
        : QUALIFY_COMPANIES_SYSTEM;

  const qualified = await completeJson<{ decisions: QualifyDecision[] }>({
    system: qualifySystem,
    user: qualifyCompaniesUser({
      audienceName: args.audienceName,
      jobTitles: args.jobTitles,
      targetRoles: args.targetRoles,
      industries: args.industries,
      companySizes: args.companySizes,
      targetMarkets: args.markets,
      targetSignals: args.signals,
      workflowType: args.workflowType,
      jobBriefBlock: args.jobBriefBlock,
      candidates: args.candidates.map((candidate) => {
        const peek = peekByDomain.get(candidate.domain);
        return {
          ...candidate,
          homepageTitle: peek?.title,
          homepageExcerpt: peek?.excerpt,
        };
      }),
    }),
    tier: "reasoning",
    projectId: args.projectId,
    label: "qualify_companies",
  });

  const decisions = new Map(
    (qualified.decisions ?? []).map((decision) => [
      decision.domain.toLowerCase().replace(/^www\./, ""),
      decision,
    ]),
  );

  const kept: ExtractedCompany[] = [];
  for (const candidate of args.candidates) {
    const domain = candidate.domain.toLowerCase().replace(/^www\./, "");
    const decision = decisions.get(domain);
    const keep = Boolean(decision?.keep) && (decision?.score ?? 0) >= MIN_KEEP_SCORE;
    events.log(keep ? "company_qualified" : "company_rejected", {
      projectId: args.projectId,
      ref: domain,
      data: {
        name: candidate.name,
        score: decision?.score ?? 0,
        reason: decision?.reason ?? "no decision from qualifier",
      },
    });
    if (keep) kept.push(candidate);
  }
  return kept;
}

function insertCompanies(
  projectId: number,
  audienceId: number,
  candidates: Array<{
    name: string;
    domain: string;
    description?: string;
    sizeHint?: string;
    country?: string;
    sourceUrl?: string;
  }>,
  target: number,
  discovered: number,
): number {
  let count = discovered;
  for (const candidate of candidates) {
    if (count >= target) break;
    const inserted = companies.upsert({
      projectId,
      audienceId,
      name: candidate.name,
      domain: candidate.domain,
      description: candidate.description,
      sizeHint: candidate.sizeHint,
      country: candidate.country,
      sourceUrl: candidate.sourceUrl,
    });
    if (inserted) {
      count += 1;
      jobs.enqueue(projectId, "find_contacts", { companyId: inserted.id });
    }
  }
  return count;
}

export async function findCompanies(
  projectId: number,
  payload: { audienceId: number; maxPerQuery?: number },
): Promise<string> {
  const project = projects.get(projectId);
  const audience = audiences.get(payload.audienceId);
  if (!project) throw new Error(`Project ${projectId} not found`);
  if (!audience) throw new Error(`Audience ${payload.audienceId} not found`);
  if (!audience.enabled) return `Audience "${audience.name}" is disabled; skipped.`;

  const targetCompanies =
    project.workflow_type === "customer_outreach"
      ? TARGET_NEW_COMPANIES_CUSTOMER
      : TARGET_NEW_COMPANIES_DEFAULT;

  let discovered = 0;
  let extractedTotal = 0;
  let rejectedTotal = 0;
  let structuredKept = 0;
  const sourceCounts: Record<string, number> = {};
  const errors: string[] = [];
  let startPage = audience.nextSearchPage;
  if (startPage > MAX_SEARCH_PAGE) startPage = 1;
  let page = startPage;
  let pagesRun = 0;
  const competitors = loadCompetitors(projectId);
  const ownDomains = blockedOwnDomains(project);
  const blocked = blockedProspectDomains({ ownDomains, competitors });
  // Same playbook across campaigns: never re-search a domain already saved.
  const playbookDomains = companies.domainsInWorkflow(project.workflow_type);
  const excludeDomains = [
    ...new Set([...blocked, ...playbookDomains].map((d) => d.toLowerCase())),
  ];
  const enrichment = searchEnrichment(
    project as {
      workflow_type: "customer_outreach" | "investor_outreach" | "job_outreach";
      workflow_data: string;
    },
  );
  const jobBrief = jobSearchBriefFromProject(project);
  const jobBriefBlock = jobBrief ? jobBriefPromptBlock(jobBrief) : undefined;
  const verifyStatus = verificationGate(projectId);
  const maxPerQuery = payload.maxPerQuery ?? MAX_CANDIDATES_PER_QUERY;
  const sourceAdapters = audience.sourceAdapters ?? [];
  const useStructured =
    sourceAdapters.length === 0 || sourceAdapters.some((id) => id !== "web");
  const useWeb = sourceAdapters.length === 0 || sourceAdapters.includes("web");

  // ── 1) Structured sources first (YC / Techstars / 500 / Antler / EF / PH) ──
  if (project.workflow_type === "customer_outreach" && useStructured) {
    throwIfCancelled();
    try {
      const { companies: sourced, sourceCounts: counts } = await fetchFromCompanySources({
        projectId,
        audienceName: audience.name,
        industries: audience.industries,
        jobTitles: audience.jobTitles,
        markets: enrichment.markets,
        signals: enrichment.signals,
        companySizes: audience.companySizes,
        fundedAfterYear: audience.fundedAfterYear,
        sourceIds: sourceAdapters,
        page,
        limit: targetCompanies,
        excludeDomains,
      });
      Object.assign(sourceCounts, counts);

      const fresh = sourced.filter(
        (candidate) =>
          isProspectable(candidate.domain, blocked) &&
          !companies.existsByDomainInWorkflow(project.workflow_type, candidate.domain) &&
          matchesCompanySizes(candidate.sizeHint, undefined, audience.companySizes),
      );
      extractedTotal += fresh.length;
      const accepted = await acceptStructuredCompanies(fresh);
      rejectedTotal += fresh.length - accepted.length;
      const before = discovered;
      discovered = insertCompanies(
        projectId,
        audience.id,
        accepted,
        targetCompanies,
        discovered,
      );
      structuredKept = discovered - before;
    } catch (error) {
      if (error instanceof JobCancelled) throw error;
      errors.push(error instanceof Error ? error.message : String(error));
    }
  }

  // ── 2) Serper / open-web until we hit the company target (if enabled) ──
  while (useWeb && pagesRun < MAX_PAGES_PER_BATCH && discovered < targetCompanies) {
    throwIfCancelled();
    pagesRun += 1;

    const queries = queriesForBatch({
      project,
      audience,
      markets: enrichment.markets,
      signals: enrichment.signals,
      competitors,
      page,
    }).filter(Boolean);

    if (queries.length === 0 && discovered === 0) {
      throw new Error(
        `Audience "${audience.name}" has no usable search queries. Edit the audience or regenerate.`,
      );
    }

    for (const searchQuery of queries) {
      if (discovered >= targetCompanies) break;
      throwIfCancelled();
      try {
        const results = await webSearch(searchQuery, {
          limit: RESULTS_PER_QUERY,
          page,
          projectId,
        });
        if (results.length === 0) continue;

        throwIfCancelled();

        const extractSystem =
          project.workflow_type === "investor_outreach"
            ? EXTRACT_INVESTOR_TARGETS_SYSTEM
            : project.workflow_type === "job_outreach"
              ? EXTRACT_JOB_TARGETS_SYSTEM
              : EXTRACT_COMPANIES_SYSTEM;
        const extracted = await completeJson<ExtractedCompanies>({
          system: extractSystem,
          user: extractCompaniesUser({
            audienceName: audience.name,
            jobTitles: audience.jobTitles,
            targetRoles:
              audience.targetRoles.length > 0
                ? audience.targetRoles
                : jobBrief?.targetRoles,
            industries:
              audience.industries.length > 0
                ? audience.industries
                : jobBrief?.industries ?? [],
            companySizes:
              audience.companySizes.length > 0
                ? audience.companySizes
                : jobBrief?.companySizes ?? [],
            targetMarkets: enrichment.markets,
            targetSignals: enrichment.signals,
            workflowType: project.workflow_type,
            jobBriefBlock,
            query: searchQuery,
            results,
          }),
          tier: "cheap",
          projectId,
          label: "extract_companies",
        });

        const named = (extracted.companies ?? []).slice(0, maxPerQuery).map((candidate) => {
          // Prefer an explicit listing URL; else first SERP link that mentions the company.
          if (candidate.sourceUrl?.trim()) return candidate;
          const needle = (candidate.name ?? "").toLowerCase();
          const hit = results.find(
            (result) =>
              needle &&
              (result.title.toLowerCase().includes(needle) ||
                result.snippet.toLowerCase().includes(needle) ||
                result.link.toLowerCase().includes(needle.replace(/\s+/g, ""))),
          );
          return { ...candidate, sourceUrl: hit?.link || results[0]?.link || "" };
        });
        const withDomains = await resolveMissingDomains({
          projectId,
          blocked,
          candidates: named,
        });

        const rawCandidates = withDomains
          .map((candidate) => ({
            ...candidate,
            domain: (candidate.domain ?? "").toLowerCase().replace(/^www\./, ""),
          }))
          .filter(
            (candidate) =>
              candidate.domain &&
              isProspectable(candidate.domain, blocked) &&
              !companies.existsByDomainInWorkflow(project.workflow_type, candidate.domain) &&
              matchesCompanySizes(candidate.sizeHint, undefined, audience.companySizes),
          );

        const seen = new Set<string>();
        const uniqueCandidates = rawCandidates.filter((candidate) => {
          if (seen.has(candidate.domain)) return false;
          seen.add(candidate.domain);
          return true;
        });

        extractedTotal += uniqueCandidates.length;
        if (uniqueCandidates.length === 0) continue;

        throwIfCancelled();
        const kept = await qualifyCandidates({
          projectId,
          workflowType: project.workflow_type,
          audienceName: audience.name,
          jobTitles: audience.jobTitles,
          targetRoles:
            audience.targetRoles.length > 0
              ? audience.targetRoles
              : jobBrief?.targetRoles,
          industries:
            audience.industries.length > 0
              ? audience.industries
              : jobBrief?.industries ?? [],
          companySizes:
            audience.companySizes.length > 0
              ? audience.companySizes
              : jobBrief?.companySizes ?? [],
          markets: enrichment.markets,
          signals: enrichment.signals,
          jobBriefBlock,
          candidates: uniqueCandidates,
        });
        rejectedTotal += uniqueCandidates.length - kept.length;
        discovered = insertCompanies(
          projectId,
          audience.id,
          kept,
          targetCompanies,
          discovered,
        );
      } catch (error) {
        if (error instanceof JobCancelled) throw error;
        errors.push(error instanceof Error ? error.message : String(error));
      }
    }

    audiences.advanceSearchPage(audience.id);
    page += 1;
    if (page > MAX_SEARCH_PAGE) page = 1;
  }

  // Advance once even if only structured sources ran (keeps YC offset moving).
  if (pagesRun === 0) {
    audiences.advanceSearchPage(audience.id);
    page = startPage + 1;
  }

  if (discovered === 0 && errors.length > 0) {
    throw new Error(`Company search failed: ${errors[0]}`);
  }

  const label = targetLabel(project.workflow_type, discovered);
  const verifyNote = verifyStatus.allow
    ? ""
    : ` Verify paused (${verifyStatus.kind}): company search still ran.`;
  const sourcesNote = Object.entries(sourceCounts)
    .filter(([, n]) => n > 0)
    .map(([id, n]) => `${id}:${n}`)
    .join(", ");
  return (
    `Batch for "${audience.name}": kept ${discovered} new ${label}` +
    ` (structured ${structuredKept}${sourcesNote ? ` [${sourcesNote}]` : ""};` +
    ` extracted ${extractedTotal}, rejected ${rejectedTotal}).` +
    ` Contact jobs queued — target ~50 people as find_contacts runs.` +
    ` Next batch cursor page ${page}.` +
    (errors.length ? ` (${errors.length} query/queries failed)` : "") +
    verifyNote
  );
}
