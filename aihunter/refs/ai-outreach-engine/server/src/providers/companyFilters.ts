/** Shared company-size / launch-year filters for adapters + Serper path. */

export const COMPANY_SIZE_BUCKETS = [
  "1-10",
  "11-50",
  "51-200",
  "201-1000",
  "1000+",
] as const;

export type CompanySizeBucket = (typeof COMPANY_SIZE_BUCKETS)[number];

const BUCKET_MAX: Record<string, number> = {
  "1-10": 10,
  "11-50": 50,
  "51-200": 200,
  "201-1000": 1000,
  "1000+": Number.POSITIVE_INFINITY,
};

/** Parse "11-50", "51-200", "1000+" into a numeric range. */
export function parseSizeBucket(raw: string): { min: number; max: number } | null {
  const value = raw.trim().toLowerCase().replace(/\s+/g, "");
  if (!value) return null;
  if (value.endsWith("+")) {
    const min = Number(value.slice(0, -1));
    if (!Number.isFinite(min)) return null;
    return { min, max: Number.POSITIVE_INFINITY };
  }
  const match = value.match(/^(\d+)-(\d+)$/);
  if (!match) return null;
  const min = Number(match[1]);
  const max = Number(match[2]);
  if (!Number.isFinite(min) || !Number.isFinite(max) || min > max) return null;
  return { min, max };
}

export function sizeHintFromTeamSize(teamSize: unknown): string {
  const n = typeof teamSize === "number" ? teamSize : Number(teamSize);
  if (!Number.isFinite(n) || n <= 0) return "";
  if (n <= 10) return "1-10";
  if (n <= 50) return "11-50";
  if (n <= 200) return "51-200";
  if (n <= 1000) return "201-1000";
  return "1000+";
}

/**
 * True when size is unknown OR falls in an allowed bucket.
 * Empty allowed list = no size filter.
 */
export function matchesCompanySizes(
  sizeHint: string | undefined,
  teamSize: unknown,
  allowed: string[],
): boolean {
  if (allowed.length === 0) return true;

  const ranges = allowed
    .map(parseSizeBucket)
    .filter((range): range is { min: number; max: number } => Boolean(range));
  if (ranges.length === 0) return true;

  const n = typeof teamSize === "number" ? teamSize : Number(teamSize);
  if (Number.isFinite(n) && n > 0) {
    return ranges.some((range) => n >= range.min && n <= range.max);
  }

  const hint = (sizeHint ?? "").trim();
  if (!hint) return true; // unknown — keep; qualifier / adapters may refine
  if (allowed.includes(hint)) return true;

  const hintRange = parseSizeBucket(hint);
  if (!hintRange) return true;
  const hintMax = BUCKET_MAX[hint] ?? hintRange.max;
  const hintMin = hintRange.min;
  return ranges.some(
    (range) => hintMin <= range.max && hintMax >= range.min,
  );
}

/** Batch strings like "W24", "S22", "W2024", "Winter 2023". */
export function yearFromBatch(batch: string | undefined): number {
  if (!batch) return 0;
  const full = batch.match(/(20\d{2})/)?.[1];
  if (full) {
    const year = Number(full);
    return Number.isFinite(year) ? year : 0;
  }
  // YC short form: W24 / S23 / X25 → 2024 / 2023 / 2025
  const short = batch.match(/\b[A-Za-z]?(\d{2})\b/)?.[1];
  if (short) {
    const year = 2000 + Number(short);
    if (year >= 2005 && year <= 2099) return year;
  }
  return 0;
}

/**
 * Keep when fundedAfterYear is 0, year unknown, or year >= fundedAfterYear.
 */
export function matchesFundedAfter(year: number, fundedAfterYear: number): boolean {
  if (!fundedAfterYear || fundedAfterYear <= 0) return true;
  if (!year || year <= 0) return true;
  return year >= fundedAfterYear;
}

export const DEFAULT_STARTUP_SIZES = ["1-10", "11-50", "51-200"];
export const DEFAULT_FUNDED_AFTER_YEAR = 2018;
