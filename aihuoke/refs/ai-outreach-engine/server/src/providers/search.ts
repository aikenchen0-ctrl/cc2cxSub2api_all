import { env } from "../env";
import { events } from "../db/repo";
import { httpJson } from "./http";
import {
  filterSearchResults,
  isSerperPatternBlocked,
  simplifySearchQuery,
} from "./searchQuerySafe";

export type SearchResult = { title: string; link: string; snippet: string };

/** Serper bills roughly $1 per 1,000 queries after the free allowance. */
const SERPER_COST_PER_QUERY = 0.001;

/** Free-tier Serper gets unreliable past deep pages; keep batches useful. */
const MAX_SERPER_PAGE = 10;

function isSerperRateLimited(error: unknown): boolean {
  if (!error || typeof error !== "object") return false;
  const status = "status" in error ? Number((error as { status?: number }).status) : 0;
  const message = error instanceof Error ? error.message : String(error);
  return status === 429 || /rate limit exceeded/i.test(message);
}

async function serper(query: string, limit: number, page: number): Promise<SearchResult[]> {
  const run = () =>
    httpJson<{
      organic?: Array<{ title?: string; link?: string; snippet?: string }>;
    }>("serper", "https://google.serper.dev/search", {
      method: "POST",
      headers: { "X-API-KEY": env.search.serperKey, "Content-Type": "application/json" },
      body: JSON.stringify({ q: query, num: Math.min(limit, 20), page }),
    });

  let payload: Awaited<ReturnType<typeof run>>;
  try {
    payload = await run();
  } catch (error) {
    // Free tier: 5 req/s. Hot-reload / bursty jobs trip this; back off twice.
    if (!isSerperRateLimited(error)) throw error;
    await Bun.sleep(1500);
    try {
      payload = await run();
    } catch (retryError) {
      if (!isSerperRateLimited(retryError)) throw retryError;
      await Bun.sleep(3000);
      payload = await run();
    }
  }

  return (payload.organic ?? [])
    .filter((entry) => entry.link)
    .map((entry) => ({
      title: entry.title ?? "",
      link: entry.link!,
      snippet: entry.snippet ?? "",
    }));
}

export function searchAvailable(): boolean {
  return Boolean(env.search.serperKey);
}

/**
 * Company / people discovery search via Serper.
 * Retries once with a simplified query when free-tier pattern rules reject the first.
 *
 * By default, social/directory hosts are stripped (company discovery).
 * Pass `allowHosts` (e.g. ["linkedin.com"]) when the caller needs those results
 * — people search requires LinkedIn /in/ profiles.
 */
export async function webSearch(
  query: string,
  opts: {
    limit?: number;
    page?: number;
    projectId?: number;
    /** Host suffixes that survive the post-search social filter. */
    allowHosts?: string[];
  } = {},
): Promise<SearchResult[]> {
  const limit = opts.limit ?? 10;
  const page = Math.min(Math.max(opts.page ?? 1, 1), MAX_SERPER_PAGE);
  if (!searchAvailable()) {
    throw new Error("No search key configured. Set SERPER_API_KEY.");
  }

  const cleaned = query.trim();
  if (!cleaned) return [];

  let usedQuery = cleaned;
  let results: SearchResult[];
  try {
    results = await serper(cleaned, limit, page);
  } catch (error) {
    if (!isSerperPatternBlocked(error)) {
      events.log("search_failed", {
        projectId: opts.projectId,
        ref: cleaned.slice(0, 180),
        data: {
          provider: "serper",
          page,
          error: error instanceof Error ? error.message : String(error),
        },
      });
      throw error;
    }

    const simplified = simplifySearchQuery(cleaned);
    if (!simplified || simplified === cleaned) {
      events.log("search_failed", {
        projectId: opts.projectId,
        ref: cleaned.slice(0, 180),
        data: {
          provider: "serper",
          page,
          error: error instanceof Error ? error.message : String(error),
          patternBlocked: true,
        },
      });
      throw error;
    }

    usedQuery = simplified;
    results = await serper(simplified, limit, page);
    events.log("search_retried", {
      projectId: opts.projectId,
      ref: simplified.slice(0, 180),
      data: {
        provider: "serper",
        page,
        original: cleaned.slice(0, 180),
        reason: "pattern_not_allowed",
      },
      costUsd: SERPER_COST_PER_QUERY,
    });
  }

  const filtered = filterSearchResults(results, { allowHosts: opts.allowHosts });

  events.log("search", {
    projectId: opts.projectId,
    ref: usedQuery.slice(0, 180),
    data: {
      provider: "serper",
      page,
      results: filtered.length,
      rawResults: results.length,
      allowHosts: opts.allowHosts ?? [],
    },
    costUsd: SERPER_COST_PER_QUERY,
  });

  return filtered;
}
