export type SourceCompany = {
  name: string;
  domain: string;
  description?: string;
  sizeHint?: string;
  country?: string;
  sourceUrl?: string;
  /** Adapter id: yc | techstars | 500global | antler | ef | surge | producthunt */
  source: string;
  batch?: string;
  tags?: string[];
};

export type SourceFetchArgs = {
  projectId: number;
  audienceName: string;
  industries: string[];
  jobTitles: string[];
  markets: string[];
  signals: string[];
  /** Hard filter when set — e.g. 1-10, 11-50, 51-200 */
  companySizes?: string[];
  /** Keep batch/launch year >= this when known; 0 = off */
  fundedAfterYear?: number;
  /** 1-based page / batch cursor */
  page: number;
  limit: number;
  /** Domains already saved or blocked */
  excludeDomains: string[];
};

export type CompanySource = {
  id: string;
  label: string;
  /** Run when campaign signals / defaults match */
  matches: (signals: string[]) => boolean;
  fetch: (args: SourceFetchArgs) => Promise<SourceCompany[]>;
};

export function domainFromWebsite(website: string): string {
  const raw = website.trim();
  if (!raw) return "";
  try {
    const withProto = /^https?:\/\//i.test(raw) ? raw : `https://${raw}`;
    return new URL(withProto).hostname.toLowerCase().replace(/^www\./, "");
  } catch {
    return raw
      .toLowerCase()
      .replace(/^https?:\/\//, "")
      .replace(/^www\./, "")
      .replace(/\/.*$/, "")
      .replace(/[^\w.-]/g, "");
  }
}

export { sizeHintFromTeamSize } from "../companyFilters";

export function matchesAnySignal(signals: string[], needles: string[]): boolean {
  if (signals.length === 0) return false;
  const normalized = signals.map((s) => s.toLowerCase().trim());
  return needles.some((needle) => {
    const n = needle.toLowerCase().trim();
    if (!n) return false;
    // Short codes (yc, ef): exact or standalone word — never substring of "pre-seed".
    if (n.length <= 3) {
      return normalized.some(
        (signal) => signal === n || new RegExp(`(^|[^a-z0-9])${n}([^a-z0-9]|$)`).test(signal),
      );
    }
    return normalized.some((signal) => signal === n || signal.includes(n));
  });
}
