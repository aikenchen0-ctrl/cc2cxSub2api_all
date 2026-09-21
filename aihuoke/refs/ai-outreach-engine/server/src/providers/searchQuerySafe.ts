/**
 * Serper free-tier rejects some Google advanced patterns with:
 *   400 "Query pattern not allowed for free accounts"
 *
 * Keep queries simple, and provide a strip-down retry path.
 */

const BLOCKED_HOST_MARKERS = [
  "linkedin.com",
  "facebook.com",
  "instagram.com",
  "twitter.com",
  "x.com",
  "youtube.com",
  "reddit.com",
];

/** Normalize a term for safe inclusion in a Serper query. */
export function sanitizeQueryTerm(term: string): string {
  return term
    .replace(/[“”]/g, '"')
    .replace(/[‘’]/g, "'")
    .replace(/[\/|]+/g, " ")
    .replace(/[()[\]{}]/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

export function quoteTerm(term: string): string {
  const cleaned = sanitizeQueryTerm(term);
  if (!cleaned) return "";
  if (/^".*"$/.test(cleaned)) return cleaned;
  if (/\s/.test(cleaned)) return `"${cleaned.replace(/"/g, "")}"`;
  return cleaned;
}

/** Build a final query string. Avoids multi -site: stacks that free Serper blocks. */
export function finalizeSearchQuery(parts: string[]): string {
  return parts
    .map((part) => part.trim())
    .filter(Boolean)
    .join(" ")
    .replace(/\s+/g, " ")
    .trim()
    .slice(0, 240);
}

/**
 * Strip operators that commonly trigger Serper free-tier pattern blocks.
 * Used as an automatic retry when the first attempt returns 400.
 */
export function simplifySearchQuery(query: string): string {
  let q = query
    .replace(/-site:\S+/gi, " ")
    .replace(/\bsite:\S+/gi, " ")
    .replace(/\bOR\b/g, " ")
    .replace(/\bAND\b/g, " ")
    .replace(/[“”]/g, '"')
    .replace(/[‘’]/g, "'");

  // Collapse nested / excessive quotes to bare words for the retry.
  q = q.replace(/"/g, " ");
  return finalizeSearchQuery([q]);
}

export function isSerperPatternBlocked(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error);
  return /query pattern not allowed/i.test(message) || /pattern not allowed/i.test(message);
}

/** Drop social/directory hosts after search instead of using -site: operators. */
export function isBlockedSearchHost(
  urlOrHost: string,
  opts: { allowHosts?: string[] } = {},
): boolean {
  const host = urlOrHost
    .toLowerCase()
    .replace(/^https?:\/\//, "")
    .replace(/^www\./, "")
    .split("/")[0] ?? "";
  const allowed = (opts.allowHosts ?? []).map((entry) => entry.toLowerCase());
  if (
    allowed.some(
      (allowedHost) => host === allowedHost || host.endsWith(`.${allowedHost}`),
    )
  ) {
    return false;
  }
  return BLOCKED_HOST_MARKERS.some(
    (blocked) => host === blocked || host.endsWith(`.${blocked}`),
  );
}

export function filterSearchResults<T extends { link: string }>(
  results: T[],
  opts: { allowHosts?: string[] } = {},
): T[] {
  return results.filter((result) => !isBlockedSearchHost(result.link, opts));
}
