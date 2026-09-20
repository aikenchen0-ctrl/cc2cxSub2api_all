import { completeJson } from "../../llm/client";
import {
  ANALYSE_RESUME_SYSTEM,
  ANALYSE_SITE_SYSTEM,
  EXTRACT_COMPETITORS_SYSTEM,
  SUGGEST_PREFERENCES_SYSTEM,
  analyseResumeUser,
  analyseSiteUser,
  extractCompetitorsUser,
  suggestPreferencesUser,
} from "../../llm/prompts";
import { crawlSite, type CrawledPage } from "../../providers/crawler";
import { searchAvailable, webSearch } from "../../providers/search";
import {
  cleanDomain,
  mergeCompetitors,
  normalizeSiteAnalysis,
  summarizePricingPlans,
} from "../../db/analysis";
import { audiences, jobs, projects, siteProfiles } from "../../db/repo";
import type { Competitor, SiteAnalysis } from "../../db/types";
import {
  crawlMaxPagesForProject,
  mergeAiPreferences,
  mergePreferencesIntoWorkflowData,
  preferencesFromProject,
  preferencesNeedAiFill,
} from "../../workflow/preferences";

export type ScanSitePayload = {
  /** Explicitly queue audience regeneration after scan (default: only if none exist). */
  regenerateAudiences?: boolean;
  /** Passed through to generate_audiences when it runs (default true). */
  preserveProspecting?: boolean;
};

type JobWorkflowData = {
  resumeText?: string;
  targetRoles?: string[];
  locations?: string[];
  remotePreference?: string;
  seniority?: string;
  notes?: string;
};

function parseWorkflowData(raw: string): Record<string, unknown> {
  try {
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === "object" ? (parsed as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

function stringList(value: unknown): string[] {
  return Array.isArray(value) ? value.map(String).map((item) => item.trim()).filter(Boolean) : [];
}

function jobWorkflowData(project: { workflow_data: string }): JobWorkflowData {
  const data = parseWorkflowData(project.workflow_data);
  return {
    resumeText: typeof data.resumeText === "string" ? data.resumeText : "",
    targetRoles: stringList(data.targetRoles),
    locations: stringList(data.locations),
    remotePreference: typeof data.remotePreference === "string" ? data.remotePreference : "",
    seniority: typeof data.seniority === "string" ? data.seniority : "",
    notes: typeof data.notes === "string" ? data.notes : "",
  };
}

function productDomain(project: { domain: string; workflow_type: string; workflow_data: string }): string {
  if (project.workflow_type !== "investor_outreach") return project.domain;
  const data = parseWorkflowData(project.workflow_data);
  const domain = typeof data.productDomain === "string" ? data.productDomain.trim() : "";
  return domain || project.domain;
}

async function maybeFillPreferences(
  project: {
    id: number;
    workflow_type: "customer_outreach" | "investor_outreach" | "job_outreach";
    workflow_data: string;
  },
  analysis: SiteAnalysis,
): Promise<boolean> {
  const current = preferencesFromProject(project);
  if (!preferencesNeedAiFill(current)) return false;

  try {
    const suggested = await completeJson<Record<string, unknown>>({
      system: SUGGEST_PREFERENCES_SYSTEM,
      user: suggestPreferencesUser({
        workflowType: project.workflow_type,
        analysis,
        workflowData: parseWorkflowData(project.workflow_data),
        currentPreferences: current,
      }),
      tier: "cheap",
      projectId: project.id,
      label: "suggest_preferences",
    });
    const merged = mergeAiPreferences(current, suggested);
    const nextData = mergePreferencesIntoWorkflowData(
      project.workflow_type,
      parseWorkflowData(project.workflow_data),
      merged,
    );
    projects.setWorkflowData(project.id, nextData);
    return true;
  } catch (error) {
    console.warn(
      "[scan_site] preference fill skipped:",
      error instanceof Error ? error.message : error,
    );
    return false;
  }
}

function reusableIncompleteProfile(projectId: number): { id: number; pages: CrawledPage[] } | null {
  const profile = siteProfiles.latest(projectId);
  if (!profile || profile.analysis) return null;

  try {
    const pages = JSON.parse(profile.pages_json) as CrawledPage[];
    return pages.length > 0 ? { id: profile.id, pages } : null;
  } catch {
    return null;
  }
}

function domainFromSearchLink(link: string): string {
  try {
    return cleanDomain(new URL(link).hostname);
  } catch {
    return cleanDomain(link);
  }
}

const DIRECTORY_HOST =
  /(g2\.com|capterra\.com|producthunt\.com|wikipedia\.org|linkedin\.com|youtube\.com|reddit\.com|medium\.com|crunchbase\.com|techcrunch\.com|forbes\.com|alternativeto\.net|saasworthy\.com|getapp\.com)$/;

/** Pull competitor-shaped markdown links from crawled pages (compare/vs/etc.). */
export function extractCompetitorLinksFromPages(
  pages: CrawledPage[],
  ownDomain: string,
): Competitor[] {
  const own = cleanDomain(ownDomain);
  const found = new Map<string, Competitor>();
  const linkRe = /\[([^\]]{2,60})\]\((https?:\/\/[^)\s]+)\)/gi;

  for (const page of pages) {
    const path = (() => {
      try {
        return new URL(page.url).pathname.toLowerCase();
      } catch {
        return "";
      }
    })();
    const comparePage = /compare|vs|alternativ|competitor/.test(path);
    let match: RegExpExecArray | null;
    const markdown = page.markdown ?? "";
    while ((match = linkRe.exec(markdown)) !== null) {
      const name = match[1]!.replace(/\s+/g, " ").trim();
      const domain = domainFromSearchLink(match[2]!);
      if (!domain || domain === own || DIRECTORY_HOST.test(domain)) continue;
      // Skip self-links and obvious non-product pages.
      if (domain.endsWith(own) || own.endsWith(domain)) continue;
      if (!comparePage && !/vs|alternative|competitor|compare/i.test(name + " " + match[0])) {
        // On non-compare pages only keep links that look like product names (short, Title Case-ish).
        if (name.split(/\s+/).length > 4) continue;
      }
      const key = name.toLowerCase().replace(/[^a-z0-9]/g, "");
      if (key.length < 3) continue;
      if (!found.has(key) || !found.get(key)!.domain) {
        found.set(key, { name, domain });
      }
    }
  }
  return [...found.values()].slice(0, 12);
}

/** Fill empty competitor domains from search hits when the name clearly matches. */
function fillDomainsFromResults(
  competitors: Competitor[],
  results: Array<{ title: string; link: string; snippet: string }>,
  ownDomain: string,
): Competitor[] {
  const own = cleanDomain(ownDomain);
  return competitors.map((competitor) => {
    if (competitor.domain) return competitor;
    const needle = competitor.name.toLowerCase().replace(/[^a-z0-9]/g, "");
    if (needle.length < 3) return competitor;

    for (const result of results) {
      const hay = `${result.title} ${result.link} ${result.snippet}`.toLowerCase();
      const compact = hay.replace(/[^a-z0-9]/g, "");
      if (!compact.includes(needle)) continue;
      const domain = domainFromSearchLink(result.link);
      if (!domain || domain === own || DIRECTORY_HOST.test(domain)) continue;
      return { ...competitor, domain };
    }
    return competitor;
  });
}

async function resolveCompetitorDomains(
  competitors: Competitor[],
  ownDomain: string,
  projectId: number,
): Promise<Competitor[]> {
  const missing = competitors.filter((item) => !item.domain).slice(0, 6);
  if (missing.length === 0) return competitors;

  let updated = competitors;
  for (const item of missing) {
    const results = await webSearch(`${item.name} official website`, {
      limit: 5,
      projectId,
    }).catch(() => []);
    if (results.length === 0) continue;
    updated = fillDomainsFromResults(updated, results, ownDomain);
  }
  return updated;
}

/**
 * Competitors = site-mentioned + markdown links + market discovery.
 * Domains are required for dashboard links and for lookalike prospecting.
 */
async function discoverCompetitors(
  analysis: SiteAnalysis,
  ownDomain: string,
  projectId: number,
  pages: CrawledPage[],
): Promise<Competitor[]> {
  const fromLinks = extractCompetitorLinksFromPages(pages, ownDomain);
  let merged = mergeCompetitors(analysis.competitors, fromLinks, ownDomain);

  if (!searchAvailable()) {
    return resolveCompetitorDomains(merged, ownDomain, projectId).catch(() => merged);
  }

  try {
    // Keep queries simple — free Serper rejects heavy OR / -site patterns.
    const product = analysis.productName || ownDomain;
    const queries = [
      `${product} alternatives competitors`,
      `${product} vs meeting assistant software`,
    ];
    const allResults: Array<{ title: string; link: string; snippet: string }> = [];
    for (const query of queries) {
      const results = await webSearch(query, { limit: 10, projectId }).catch(() => []);
      allResults.push(...results);
    }

    if (allResults.length > 0) {
      const extracted = await completeJson<{ competitors: Competitor[] }>({
        system: EXTRACT_COMPETITORS_SYSTEM,
        user: extractCompetitorsUser({
          productName: analysis.productName,
          oneLiner: analysis.oneLiner,
          ownDomain,
          namedOnSite: merged,
          results: allResults.slice(0, 20),
        }),
        tier: "cheap",
        projectId,
        label: "extract_competitors",
      });

      const online = normalizeSiteAnalysis({ competitors: extracted.competitors }).competitors;
      merged = mergeCompetitors(online, merged, ownDomain);
      merged = fillDomainsFromResults(merged, allResults, ownDomain);
    }

    merged = await resolveCompetitorDomains(merged, ownDomain, projectId);
    return merged.slice(0, 12);
  } catch (error) {
    console.warn(
      "[scan_site] competitor discovery skipped:",
      error instanceof Error ? error.message : error,
    );
    return resolveCompetitorDomains(merged, ownDomain, projectId).catch(() => merged);
  }
}

export async function scanSite(
  projectId: number,
  payload: ScanSitePayload = {},
): Promise<string> {
  const project = projects.get(projectId);
  if (!project) throw new Error(`Project ${projectId} not found`);

  if (project.workflow_type === "job_outreach") {
    const data = jobWorkflowData(project);
    if (!data.resumeText?.trim()) {
      throw new Error("Job outreach needs resume/profile text before it can build a strategy.");
    }

    const profile = siteProfiles.create(projectId, [
      {
        url: "resume://local",
        title: project.name,
        markdown: data.resumeText.slice(0, 20_000),
      },
    ]);
    const raw = await completeJson<SiteAnalysis>({
      system: ANALYSE_RESUME_SYSTEM,
      user: analyseResumeUser({
        resumeText: data.resumeText,
        preferences: {
          targetRoles: data.targetRoles,
          locations: data.locations,
          remotePreference: data.remotePreference,
          seniority: data.seniority,
          notes: data.notes,
        },
      }),
      tier: "reasoning",
      projectId,
      label: "analyse_resume",
    });

    const analysis = normalizeSiteAnalysis(raw);
    siteProfiles.setAnalysis(profile.id, analysis);
    const filledPrefs = await maybeFillPreferences(project, analysis);

    const existingAudiences = audiences.list(projectId).length;
    const shouldGenerate =
      existingAudiences === 0 || payload.regenerateAudiences === true;
    if (shouldGenerate) {
      jobs.enqueue(projectId, "generate_audiences", {
        preserveProspecting: payload.preserveProspecting !== false,
      });
    }

    return `Analysed candidate profile for "${analysis.productName}".${
      filledPrefs ? " Filled blank playbook preferences with AI suggestions." : ""
    }${
      shouldGenerate
        ? " Queued job-search lane generation (roles you want → employers hiring → hiring contacts)."
        : " Left existing search lanes/contacts untouched."
    }`;
  }

  // Re-scan always builds a new site_profiles row (latest wins). Incomplete
  // profiles without analysis are the only rows reused — never old analysis.
  const reusable = reusableIncompleteProfile(projectId);
  const { pages, provider, profileId } = reusable
    ? { pages: reusable.pages, provider: "saved crawl", profileId: reusable.id }
    : await (async () => {
        const domain = productDomain(project);
        const crawl = await crawlSite(domain, projectId, {
          maxPages: crawlMaxPagesForProject(project),
        });
        const profile = siteProfiles.create(projectId, crawl.pages);
        return { pages: crawl.pages, provider: crawl.provider, profileId: profile.id };
      })();
  const domain = productDomain(project);

  const raw = await completeJson<SiteAnalysis>({
    system: ANALYSE_SITE_SYSTEM,
    user: analyseSiteUser(domain, pages),
    tier: "reasoning",
    projectId,
    label: "analyse_site",
  });

  let analysis = normalizeSiteAnalysis(raw);
  analysis = {
    ...analysis,
    competitors: await discoverCompetitors(analysis, domain, projectId, pages),
    pricing: analysis.pricing || summarizePricingPlans(analysis.pricingPlans),
  };

  siteProfiles.setAnalysis(profileId, analysis);
  const filledPrefs = await maybeFillPreferences(project, analysis);

  // First scan (no audiences yet) always builds ICPs. Re-scan does not — that
  // used to silently cascade-delete contacts/messages via generate_audiences.
  const existingAudiences = audiences.list(projectId).length;
  const shouldGenerate =
    existingAudiences === 0 || payload.regenerateAudiences === true;
  if (shouldGenerate) {
    jobs.enqueue(projectId, "generate_audiences", {
      preserveProspecting: payload.preserveProspecting !== false,
    });
  }

  const audienceNote = shouldGenerate
    ? existingAudiences === 0
      ? " Queued audience generation."
      : " Queued audience regeneration."
    : " Left existing audiences/contacts untouched.";

  return `Read ${pages.length} pages via ${provider}; identified "${analysis.productName}" with ${analysis.competitors.length} competitor(s) and ${analysis.useCases.length} use case(s).${
    filledPrefs ? " Filled blank playbook preferences with AI suggestions." : ""
  }${audienceNote}`;
}
