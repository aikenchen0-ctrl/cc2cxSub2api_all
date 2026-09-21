import { env } from "../env";
import { getSettings } from "../settings";
import { events } from "../db/repo";
import { httpJson, mapLimit } from "./http";

const FIRECRAWL = "https://api.firecrawl.dev/v2";

export type CrawledPage = { url: string; title: string; markdown: string };

/**
 * Pages that actually describe what a product is and who buys it. Crawling a
 * whole site burns credits on blog archives and changelogs for no benefit.
 */
const HIGH_VALUE_PATHS = [
  "",
  "pricing",
  "plans",
  "features",
  "product",
  "platform",
  "about",
  "use-cases",
  "usecases",
  "solutions",
  "industries",
  "customers",
  "case-studies",
  "how-it-works",
  "why",
  "compare",
  "vs",
  "for",
  "demo",
  "faq",
];

function pathScore(url: string): number {
  let path: string;
  try {
    path = new URL(url).pathname.replace(/^\/|\/$/g, "").toLowerCase();
  } catch {
    return -1;
  }
  if (path === "") return 100;
  const depth = path.split("/").length;
  const firstSegment = path.split("/")[0] ?? "";
  if (/^(blog|news|changelog|docs|help|legal|privacy|terms|careers|jobs)$/.test(firstSegment)) {
    return -1;
  }
  const index = HIGH_VALUE_PATHS.indexOf(firstSegment);
  const base = index >= 0 ? 90 - index * 3 : 30;
  return base - depth * 5;
}

function stripHtml(html: string): string {
  // Preserve anchors as markdown links so competitor/compare URLs survive the
  // plain-fetch fallback (Firecrawl already returns markdown with links).
  const withLinks = html.replace(
    /<a\b[^>]*href=["'](https?:\/\/[^"']+)["'][^>]*>([\s\S]*?)<\/a>/gi,
    (_full, href: string, label: string) => {
      const text = label.replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").trim() || href;
      return `[${text}](${href})`;
    },
  );
  return withLinks
    .replace(/<script[\s\S]*?<\/script>/gi, " ")
    .replace(/<style[\s\S]*?<\/style>/gi, " ")
    .replace(/<nav[\s\S]*?<\/nav>/gi, " ")
    .replace(/<footer[\s\S]*?<\/footer>/gi, " ")
    .replace(/<[^>]+>/g, " ")
    .replace(/&nbsp;/g, " ")
    .replace(/&amp;/g, "&")
    .replace(/&#\d+;/g, " ")
    .replace(/[ \t]+/g, " ")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

async function firecrawlMap(domain: string): Promise<string[]> {
  const payload = await httpJson<{ links?: Array<{ url: string } | string> }>(
    "firecrawl",
    `${FIRECRAWL}/map`,
    {
      method: "POST",
      headers: {
        Authorization: `Bearer ${env.crawler.firecrawlKey}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ url: `https://${domain}`, limit: 100 }),
    },
  );
  return (payload.links ?? []).map((link) => (typeof link === "string" ? link : link.url));
}

async function firecrawlScrape(url: string): Promise<CrawledPage | null> {
  const payload = await httpJson<{
    data?: { markdown?: string; metadata?: { title?: string } };
  }>("firecrawl", `${FIRECRAWL}/scrape`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${env.crawler.firecrawlKey}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ url, formats: ["markdown"], onlyMainContent: true }),
  });
  const markdown = payload.data?.markdown ?? "";
  if (!markdown.trim()) return null;
  return { url, title: payload.data?.metadata?.title ?? url, markdown };
}

async function plainFetch(url: string): Promise<CrawledPage | null> {
  const res = await fetch(url, {
    headers: { "User-Agent": "outreach-engine/0.1 (+personal research tool)" },
    signal: AbortSignal.timeout(20_000),
  });
  if (!res.ok) return null;
  const html = await res.text();
  const title = html.match(/<title[^>]*>([^<]*)<\/title>/i)?.[1]?.trim() ?? url;
  const text = stripHtml(html);
  if (text.length < 200) return null;
  return { url, title, markdown: text };
}

/**
 * Cheap homepage peek for company qualification. Uses plain fetch only — never
 * burns Firecrawl credits during prospecting.
 */
export async function peekHomepage(
  domain: string,
): Promise<{ title: string; excerpt: string } | null> {
  const clean = domain.trim().toLowerCase().replace(/^www\./, "");
  if (!clean) return null;
  try {
    const page =
      (await plainFetch(`https://${clean}`)) ?? (await plainFetch(`https://www.${clean}`));
    if (!page) return null;
    return {
      title: page.title.slice(0, 180),
      excerpt: page.markdown.slice(0, 1200),
    };
  } catch {
    return null;
  }
}

/**
 * Fetch a job listing / careers URL so contact emails can reference the real role.
 * Plain fetch only — no Firecrawl credits. Best-effort; returns null on failure.
 */
export async function peekJobListing(
  url: string,
): Promise<{ title: string; excerpt: string; url: string } | null> {
  const clean = url.trim();
  if (!/^https?:\/\//i.test(clean)) return null;
  // Skip non-listing junk we sometimes store as source_url.
  if (/^(manual|imported|import)$/i.test(clean)) return null;
  try {
    const page = await plainFetch(clean);
    if (!page) return null;
    const excerpt = page.markdown
      .replace(/\s+/g, " ")
      .trim()
      .slice(0, 3500);
    if (excerpt.length < 80) return null;
    return {
      title: page.title.slice(0, 200),
      excerpt,
      url: clean,
    };
  } catch {
    return null;
  }
}

/** Extract same-origin links from raw HTML, for the keyless fallback crawler. */
async function discoverLinks(domain: string): Promise<string[]> {
  const origin = `https://${domain}`;
  try {
    const res = await fetch(origin, {
      headers: { "User-Agent": "outreach-engine/0.1" },
      signal: AbortSignal.timeout(20_000),
    });
    if (!res.ok) return [origin];
    const html = await res.text();
    const found = new Set<string>([origin]);
    for (const match of html.matchAll(/href="([^"#?]+)"/gi)) {
      const href = match[1]!;
      try {
        const resolved = new URL(href, origin);
        if (resolved.hostname.replace(/^www\./, "") === domain.replace(/^www\./, "")) {
          found.add(`${resolved.origin}${resolved.pathname}`.replace(/\/$/, "") || origin);
        }
      } catch {
        // ignore unparseable hrefs
      }
    }
    return [...found];
  } catch {
    return [origin];
  }
}

/**
 * Fetch the pages of a site that explain what it sells. Ranks candidate URLs so
 * a fixed page budget is spent on pricing/features/about rather than blog posts.
 * Firecrawl is the supported crawler; plain fetch is only an automatic fallback
 * when Firecrawl has no key, fails, or returns nothing.
 */
export async function crawlSite(
  domain: string,
  projectId: number,
  opts: { maxPages?: number } = {},
): Promise<{ pages: CrawledPage[]; provider: string }> {
  const clean = domain.replace(/^https?:\/\//, "").replace(/\/.*$/, "").replace(/^www\./, "");
  let provider: "firecrawl" | "fetch" = env.crawler.firecrawlKey ? "firecrawl" : "fetch";
  const maxPages = Math.min(Math.max(opts.maxPages ?? getSettings().crawlMaxPages, 1), 50);

  async function readWith(selectedProvider: "firecrawl" | "fetch") {
    const candidates =
      selectedProvider === "firecrawl" ? await firecrawlMap(clean) : await discoverLinks(clean);
    const ranked = [...new Set(candidates)]
      .map((url) => ({ url, score: pathScore(url) }))
      .filter((entry) => entry.score > 0)
      .sort((a, b) => b.score - a.score)
      .slice(0, maxPages)
      .map((entry) => entry.url);

    const targets = ranked.length > 0 ? ranked : [`https://${clean}`];
    const scraped = await mapLimit(targets, 2, (url) =>
      selectedProvider === "firecrawl" ? firecrawlScrape(url) : plainFetch(url),
    );
    return {
      requested: targets.length,
      pages: scraped.filter((page): page is CrawledPage => page !== null),
    };
  }

  let requested = 0;
  let pages: CrawledPage[] = [];
  try {
    const result = await readWith(provider);
    requested = result.requested;
    pages = result.pages;
    if (provider === "firecrawl" && pages.length === 0) {
      throw new Error("Firecrawl returned no readable pages");
    }
  } catch (error) {
    if (provider !== "firecrawl") throw error;
    events.log("crawl_fallback", {
      projectId,
      ref: clean,
      data: {
        from: "firecrawl",
        to: "fetch",
        reason: error instanceof Error ? error.message : String(error),
      },
    });
    provider = "fetch";
    const result = await readWith("fetch");
    requested = result.requested;
    pages = result.pages;
  }

  events.log("crawl", {
    projectId,
    ref: clean,
    data: { provider, requested, retrieved: pages.length },
    // Firecrawl bills 1 credit per page mapped or scraped.
    costUsd: provider === "firecrawl" ? (requested + 1) * 0.0 : 0,
  });

  if (pages.length === 0) {
    throw new Error(
      `Crawled ${clean} but got no readable content. Set FIRECRAWL_API_KEY (free tier: ~1,000 credits/month).`,
    );
  }
  return { pages, provider };
}

/**
 * Pull mailto: addresses and nearby names off a company's public team pages.
 * This is the only free contact source, so it runs before any paid finder.
 */
export async function scrapePublicEmails(
  domain: string,
): Promise<Array<{ email: string; context: string; url: string }>> {
  const paths = ["", "contact", "about", "team", "about-us", "contact-us", "company"];
  const urls = paths.map((path) => `https://${domain}${path ? `/${path}` : ""}`);
  const fetched = await mapLimit(urls, 3, async (url) => {
    const res = await fetch(url, {
      headers: { "User-Agent": "outreach-engine/0.1" },
      signal: AbortSignal.timeout(12_000),
    }).catch(() => null);
    if (!res || !res.ok) return null;
    return { url, html: await res.text() };
  });

  const seen = new Map<string, { email: string; context: string; url: string }>();
  const emailPattern = new RegExp(
    `[A-Za-z0-9._%+-]+@${domain.replace(/\./g, "\\.")}`,
    "gi",
  );

  for (const page of fetched) {
    if (!page) continue;
    for (const match of page.html.matchAll(emailPattern)) {
      const email = match[0].toLowerCase();
      if (seen.has(email)) continue;
      const start = Math.max(0, (match.index ?? 0) - 300);
      seen.set(email, {
        email,
        context: stripHtml(page.html.slice(start, (match.index ?? 0) + 200)).slice(-260),
        url: page.url,
      });
    }
  }
  return [...seen.values()];
}
