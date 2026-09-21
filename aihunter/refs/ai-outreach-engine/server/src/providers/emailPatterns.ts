/**
 * Work-email local-part formats we guess against company domains.
 * Order is the default probe sequence when no domain preference is known.
 * Prefer first.last — first@ alone is rarely the corporate inbox and must not
 * be the unverified fallback people see in the contacts list.
 */
export const EMAIL_PATTERN_KEYS = [
  "first.last",
  "flast",
  "firstlast",
  "first.lastInitial",
  "first",
] as const;

export type EmailPatternKey = (typeof EMAIL_PATTERN_KEYS)[number];

export function isEmailPatternKey(value: string): value is EmailPatternKey {
  return (EMAIL_PATTERN_KEYS as readonly string[]).includes(value);
}

export function normaliseNamePart(value: string): string {
  return value
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z]/g, "");
}

export function parseNameParts(
  fullName: string,
): { first: string; last: string } | null {
  const parts = fullName.trim().split(/\s+/).filter(Boolean);
  const first = normaliseNamePart(parts[0] ?? "");
  const last = normaliseNamePart(parts.length > 1 ? parts[parts.length - 1]! : "");
  if (!first) return null;
  return { first, last };
}

export function applyEmailPattern(
  key: EmailPatternKey,
  fullName: string,
  domain: string,
): string | null {
  const parts = parseNameParts(fullName);
  if (!parts) return null;
  const { first, last } = parts;
  const host = domain.trim().toLowerCase();
  if (!host) return null;

  if (!last) {
    return key === "first" ? `${first}@${host}` : null;
  }

  switch (key) {
    case "first":
      return `${first}@${host}`;
    case "first.last":
      return `${first}.${last}@${host}`;
    case "first.lastInitial":
      return `${first}.${last[0]}@${host}`;
    case "flast":
      return `${first[0]}${last}@${host}`;
    case "firstlast":
      return `${first}${last}@${host}`;
  }
}

/** Which known format produced this address for this person, if any. */
export function detectEmailPattern(
  email: string,
  fullName: string,
  domain?: string,
): EmailPatternKey | null {
  const at = email.indexOf("@");
  if (at <= 0) return null;
  const local = email.slice(0, at).toLowerCase();
  const emailDomain = email.slice(at + 1).toLowerCase();
  if (domain && emailDomain !== domain.trim().toLowerCase()) return null;

  if (!parseNameParts(fullName)) return null;

  for (const key of EMAIL_PATTERN_KEYS) {
    const candidate = applyEmailPattern(key, fullName, emailDomain);
    if (!candidate) continue;
    if (candidate.slice(0, candidate.indexOf("@")).toLowerCase() === local) {
      return key;
    }
  }
  return null;
}

/** How many learned domain formats to probe before the default order. */
export const RANKED_PATTERN_LIMIT = 2;

/**
 * Default candidate addresses for a person.
 * Prefer `orderedEmailCandidates` when a domain preference may exist.
 */
export function emailPatterns(fullName: string, domain: string): string[] {
  return orderedEmailCandidates(fullName, domain);
}

/**
 * Candidates for verification:
 *   1. learned formats for the domain, highest hit_count first (up to 2)
 *   2. then the default probe order
 *
 * Each address appears at most once.
 */
export function orderedEmailCandidates(
  fullName: string,
  domain: string,
  rankedKeys: readonly EmailPatternKey[] = [],
): string[] {
  const seen = new Set<string>();
  const out: string[] = [];

  const push = (email: string | null) => {
    if (!email) return;
    const normalised = email.toLowerCase();
    if (seen.has(normalised)) return;
    seen.add(normalised);
    out.push(normalised);
  };

  for (const key of rankedKeys) {
    push(applyEmailPattern(key, fullName, domain));
  }
  for (const key of EMAIL_PATTERN_KEYS) {
    push(applyEmailPattern(key, fullName, domain));
  }
  return out;
}
