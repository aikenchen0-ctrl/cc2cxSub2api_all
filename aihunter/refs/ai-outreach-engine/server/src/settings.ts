import { db } from "./db/index";

/**
 * Product knobs that used to live in .env. Secrets and provider identity stay
 * in env; everything operators tune day-to-day lives here (SQLite + Settings UI).
 */
export type AppSettings = {
  blockedCountries: string[];
  blockFreemail: boolean;
  brandLogoUrl: string;
  brandProductUrl: string;
  brandPrimaryColor: string;
  dailySendCap: number;
  minMinutesBetweenSends: number;
  sendWindowStartHour: number;
  sendWindowEndHour: number;
  sendWindowTimezone: string;
  sendOnWeekends: boolean;
  includeUnsubscribeLink: boolean;
  unsubscribeBaseUrl: string;
  localizeEmails: boolean;
  targetMarkets: string[];
  targetCompanySignals: string[];
  maxContactsPerCompany: number;
  crawlMaxPages: number;
};

type SettingsRow = {
  id: number;
  blocked_countries: string;
  block_freemail: number;
  brand_logo_url: string;
  brand_product_url: string;
  brand_primary_color: string;
  daily_send_cap: number;
  min_minutes_between_sends: number;
  send_window_start_hour: number;
  send_window_end_hour: number;
  send_window_timezone: string;
  send_on_weekends: number;
  include_unsubscribe_link: number | null;
  unsubscribe_base_url: string;
  localize_emails: number;
  target_markets: string;
  target_company_signals: string;
  max_contacts_per_company: number;
  crawl_max_pages: number;
};

/** Premium English-first B2B markets that return usable cold-email targets. */
export const DEFAULT_MARKETS = [
  "United States",
  "Canada",
  "United Kingdom",
  "Ireland",
  "Australia",
  "New Zealand",
  "Germany",
  "Netherlands",
  "Switzerland",
  "Sweden",
  "Denmark",
  "Norway",
  "Finland",
  "Singapore",
  "UAE",
  "France",
  "Belgium",
  "Austria",
  "Spain",
  "Portugal",
  "Italy",
  "Latin America",
];

/**
 * Company quality / stage signals — short atomic phrases for search + LLM context.
 * Expanded from the product default brief (funded / accelerators / verticals / hiring).
 */
export const DEFAULT_SIGNALS = [
  "Recently funded",
  "Pre-Seed",
  "Seed",
  "Series A",
  "Series B",
  "YC",
  "Techstars",
  "500 Global",
  "Antler",
  "EF",
  "Surge",
  "accelerator-backed",
  "Bootstrapped",
  "Fast-growing AI",
  "B2B SaaS",
  "Developer Tools",
  "Design",
  "Sales Tech",
  "Recruiting",
  "Customer Success",
  "Product Management",
  "Security",
  "Infrastructure",
  "Hiring",
  "Remote-first",
  "Hybrid",
  "high meeting culture",
];

/** Prior seed values — rewrite once so existing local DBs pick up the new set. */
const LEGACY_DEFAULT_MARKETS = [
  "US",
  "UK",
  "EU",
  "Australia",
  "Singapore",
  "Malaysia",
  "UAE",
  "Latin America",
  "top Indian startups",
];

const LEGACY_DEFAULT_SIGNALS_V1 = [
  "YC",
  "Y Combinator",
  "Entrepreneur First",
  "Techstars",
  "Seedcamp",
  "venture-backed",
  "funded startup",
  "Series A",
  "Series B",
  "fast-growing SaaS",
];

const LEGACY_DEFAULT_SIGNALS_V2 = [
  "Pre-Seed",
  "Seed",
  "YC",
  "Techstars",
  "500 Global",
  "Antler",
  "EF",
  "Fast-growing AI",
  "B2B SaaS",
  "Developer Tools",
  "Design",
  "Sales Tech",
  "Recruiting",
  "Customer Success",
  "Product Management",
  "Series A",
  "Series B",
  "venture-backed",
];

export const DEFAULT_SETTINGS: AppSettings = {
  // High-risk / hard-to-reach markets for cold B2B — ISO 3166-1 alpha-2.
  blockedCountries: [
    // Asia / Central Asia / Middle East
    "CN", // China
    "JP", // Japan
    "AF", // Afghanistan
    "TJ", // Tajikistan
    "KG", // Kyrgyzstan
    "UZ", // Uzbekistan
    "KZ", // Kazakhstan
    "TM", // Turkmenistan
    "IR", // Iran
    "IQ", // Iraq
    "OM", // Oman
    "YE", // Yemen
    "RU", // Russia
    "BN", // Brunei
    "MM", // Myanmar
    "KH", // Cambodia
    "MN", // Mongolia
    // Africa
    "BI", // Burundi
    "SO", // Somalia
    "SD", // Sudan
    "SS", // South Sudan
    "NE", // Niger
    "ML", // Mali
    "BF", // Burkina Faso
  ],
  blockFreemail: true,
  brandLogoUrl: "",
  brandProductUrl: "https://getobserver.app",
  brandPrimaryColor: "#6ea8fe",
  dailySendCap: 30,
  minMinutesBetweenSends: 1,
  sendWindowStartHour: 9,
  sendWindowEndHour: 22,
  sendWindowTimezone: "Asia/Kolkata",
  sendOnWeekends: true,
  includeUnsubscribeLink: false,
  unsubscribeBaseUrl: "http://localhost:8787",
  localizeEmails: true,
  targetMarkets: DEFAULT_MARKETS,
  targetCompanySignals: DEFAULT_SIGNALS,
  maxContactsPerCompany: 10,
  crawlMaxPages: 15,
};

function envStr(key: string, fallback: string): string {
  const raw = process.env[key];
  return raw === undefined || raw === "" ? fallback : raw;
}

function envInt(key: string, fallback: number): number {
  const parsed = Number.parseInt(envStr(key, ""), 10);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function envBool(key: string, fallback: boolean): boolean {
  const raw = envStr(key, "").toLowerCase();
  if (raw === "") return fallback;
  return raw === "true" || raw === "1" || raw === "yes";
}

function envList(key: string, fallback: string[], upper = false): string[] {
  const raw = envStr(key, "");
  if (raw === "") return fallback;
  return raw
    .split(",")
    .map((part) => (upper ? part.trim().toUpperCase() : part.trim()))
    .filter(Boolean);
}

function splitCsv(raw: string, upper = false): string[] {
  if (!raw.trim()) return [];
  return raw
    .split(",")
    .map((part) => (upper ? part.trim().toUpperCase() : part.trim()))
    .filter(Boolean);
}

function joinCsv(values: string[]): string {
  return values.map((value) => value.trim()).filter(Boolean).join(",");
}

function clampInt(value: number, min: number, max: number, fallback: number): number {
  if (!Number.isFinite(value)) return fallback;
  return Math.min(max, Math.max(min, Math.trunc(value)));
}

function rowToSettings(row: SettingsRow): AppSettings {
  return {
    blockedCountries: splitCsv(row.blocked_countries, true),
    blockFreemail: row.block_freemail === 1,
    brandLogoUrl: row.brand_logo_url,
    brandProductUrl: row.brand_product_url || DEFAULT_SETTINGS.brandProductUrl,
    brandPrimaryColor: row.brand_primary_color || DEFAULT_SETTINGS.brandPrimaryColor,
    dailySendCap: row.daily_send_cap,
    minMinutesBetweenSends: row.min_minutes_between_sends,
    sendWindowStartHour: row.send_window_start_hour,
    sendWindowEndHour: row.send_window_end_hour,
    sendWindowTimezone: row.send_window_timezone || DEFAULT_SETTINGS.sendWindowTimezone,
    sendOnWeekends: row.send_on_weekends === 1,
    includeUnsubscribeLink: false,
    unsubscribeBaseUrl: row.unsubscribe_base_url || DEFAULT_SETTINGS.unsubscribeBaseUrl,
    localizeEmails: row.localize_emails === 1,
    targetMarkets: splitCsv(row.target_markets),
    targetCompanySignals: splitCsv(row.target_company_signals),
    maxContactsPerCompany: row.max_contacts_per_company,
    crawlMaxPages: row.crawl_max_pages,
  };
}

function sameCsvSet(stored: string, expected: string[]): boolean {
  const a = splitCsv(stored)
    .map((item) => item.toLowerCase())
    .sort();
  const b = expected.map((item) => item.toLowerCase()).sort();
  if (a.length !== b.length) return false;
  return a.every((item, index) => item === b[index]);
}

/** Seed once from process.env so existing .env values are not lost on upgrade. */
export function ensureSettingsSeeded(): void {
  const existing = db.query<{ id: number }, []>(`SELECT id FROM app_settings WHERE id = 1`).get();
  if (existing) {
    // One-time upgrade: replace the old built-in targeting defaults with the
    // stronger English-first market + signal set. Custom operator edits are kept.
    const row = db
      .query<{ target_markets: string; target_company_signals: string }, []>(
        `SELECT target_markets, target_company_signals FROM app_settings WHERE id = 1`,
      )
      .get();
    if (row) {
      const markets = sameCsvSet(row.target_markets, LEGACY_DEFAULT_MARKETS)
        ? joinCsv(DEFAULT_MARKETS)
        : row.target_markets;
      const signals =
        sameCsvSet(row.target_company_signals, LEGACY_DEFAULT_SIGNALS_V1) ||
        sameCsvSet(row.target_company_signals, LEGACY_DEFAULT_SIGNALS_V2)
          ? joinCsv(DEFAULT_SIGNALS)
          : row.target_company_signals;
      if (markets !== row.target_markets || signals !== row.target_company_signals) {
        db.run(
          `UPDATE app_settings SET target_markets = ?, target_company_signals = ?, updated_at = datetime('now') WHERE id = 1`,
          [markets, signals],
        );
      }
    }
    return;
  }

  const seed: AppSettings = {
    blockedCountries: envList("BLOCKED_COUNTRIES", DEFAULT_SETTINGS.blockedCountries, true),
    blockFreemail: envBool("BLOCK_FREEMAIL", DEFAULT_SETTINGS.blockFreemail),
    brandLogoUrl: envStr("BRAND_LOGO_URL", DEFAULT_SETTINGS.brandLogoUrl),
    brandProductUrl: envStr("BRAND_PRODUCT_URL", DEFAULT_SETTINGS.brandProductUrl),
    brandPrimaryColor: envStr("BRAND_PRIMARY_COLOR", DEFAULT_SETTINGS.brandPrimaryColor),
    dailySendCap: envInt("DAILY_SEND_CAP", DEFAULT_SETTINGS.dailySendCap),
    minMinutesBetweenSends: envInt(
      "MIN_MINUTES_BETWEEN_SENDS",
      DEFAULT_SETTINGS.minMinutesBetweenSends,
    ),
    sendWindowStartHour: envInt("SEND_WINDOW_START_HOUR", DEFAULT_SETTINGS.sendWindowStartHour),
    sendWindowEndHour: envInt("SEND_WINDOW_END_HOUR", DEFAULT_SETTINGS.sendWindowEndHour),
    sendWindowTimezone: envStr("SEND_WINDOW_TIMEZONE", DEFAULT_SETTINGS.sendWindowTimezone),
    sendOnWeekends: envBool("SEND_ON_WEEKENDS", DEFAULT_SETTINGS.sendOnWeekends),
    includeUnsubscribeLink: envBool(
      "INCLUDE_UNSUBSCRIBE_LINK",
      DEFAULT_SETTINGS.includeUnsubscribeLink,
    ),
    unsubscribeBaseUrl: envStr("UNSUBSCRIBE_BASE_URL", DEFAULT_SETTINGS.unsubscribeBaseUrl),
    localizeEmails: envBool("LOCALIZE_EMAILS", DEFAULT_SETTINGS.localizeEmails),
    targetMarkets: envList("TARGET_MARKETS", DEFAULT_SETTINGS.targetMarkets),
    targetCompanySignals: envList("TARGET_COMPANY_SIGNALS", DEFAULT_SETTINGS.targetCompanySignals),
    maxContactsPerCompany: envInt(
      "MAX_CONTACTS_PER_COMPANY",
      DEFAULT_SETTINGS.maxContactsPerCompany,
    ),
    crawlMaxPages: envInt("CRAWL_MAX_PAGES", DEFAULT_SETTINGS.crawlMaxPages),
  };

  db.run(
    `INSERT INTO app_settings (
      id, blocked_countries, block_freemail, brand_logo_url, brand_product_url,
      brand_primary_color, daily_send_cap, min_minutes_between_sends,
      send_window_start_hour, send_window_end_hour, send_window_timezone,
      send_on_weekends, include_unsubscribe_link, unsubscribe_base_url, localize_emails,
      target_markets, target_company_signals, max_contacts_per_company, crawl_max_pages
    ) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    [
      joinCsv(seed.blockedCountries),
      seed.blockFreemail ? 1 : 0,
      seed.brandLogoUrl,
      seed.brandProductUrl,
      seed.brandPrimaryColor,
      seed.dailySendCap,
      seed.minMinutesBetweenSends,
      seed.sendWindowStartHour,
      seed.sendWindowEndHour,
      seed.sendWindowTimezone,
      seed.sendOnWeekends ? 1 : 0,
      seed.includeUnsubscribeLink ? 1 : 0,
      seed.unsubscribeBaseUrl,
      seed.localizeEmails ? 1 : 0,
      joinCsv(seed.targetMarkets),
      joinCsv(seed.targetCompanySignals),
      seed.maxContactsPerCompany,
      seed.crawlMaxPages,
    ],
  );
}

export function getSettings(): AppSettings {
  ensureSettingsSeeded();
  const row = db
    .query<SettingsRow, []>(`SELECT * FROM app_settings WHERE id = 1`)
    .get();
  return row ? rowToSettings(row) : { ...DEFAULT_SETTINGS };
}

export type SettingsPatch = Partial<{
  blockedCountries: string[];
  blockFreemail: boolean;
  brandLogoUrl: string;
  brandProductUrl: string;
  brandPrimaryColor: string;
  dailySendCap: number;
  minMinutesBetweenSends: number;
  sendWindowStartHour: number;
  sendWindowEndHour: number;
  sendWindowTimezone: string;
  sendOnWeekends: boolean;
  includeUnsubscribeLink: boolean;
  unsubscribeBaseUrl: string;
  localizeEmails: boolean;
  targetMarkets: string[];
  targetCompanySignals: string[];
  maxContactsPerCompany: number;
  crawlMaxPages: number;
}>;

export function updateSettings(patch: SettingsPatch): AppSettings {
  const current = getSettings();
  const next: AppSettings = {
    blockedCountries: patch.blockedCountries ?? current.blockedCountries,
    blockFreemail: patch.blockFreemail ?? current.blockFreemail,
    brandLogoUrl: patch.brandLogoUrl ?? current.brandLogoUrl,
    brandProductUrl: (patch.brandProductUrl ?? current.brandProductUrl).trim() ||
      DEFAULT_SETTINGS.brandProductUrl,
    brandPrimaryColor: (patch.brandPrimaryColor ?? current.brandPrimaryColor).trim() ||
      DEFAULT_SETTINGS.brandPrimaryColor,
    dailySendCap: clampInt(
      patch.dailySendCap ?? current.dailySendCap,
      1,
      500,
      DEFAULT_SETTINGS.dailySendCap,
    ),
    minMinutesBetweenSends: clampInt(
      patch.minMinutesBetweenSends ?? current.minMinutesBetweenSends,
      0,
      120,
      DEFAULT_SETTINGS.minMinutesBetweenSends,
    ),
    sendWindowStartHour: clampInt(
      patch.sendWindowStartHour ?? current.sendWindowStartHour,
      0,
      23,
      DEFAULT_SETTINGS.sendWindowStartHour,
    ),
    sendWindowEndHour: clampInt(
      patch.sendWindowEndHour ?? current.sendWindowEndHour,
      1,
      24,
      DEFAULT_SETTINGS.sendWindowEndHour,
    ),
    sendWindowTimezone: (patch.sendWindowTimezone ?? current.sendWindowTimezone).trim() ||
      DEFAULT_SETTINGS.sendWindowTimezone,
    sendOnWeekends: patch.sendOnWeekends ?? current.sendOnWeekends,
    // Cold reach-out: no unsubscribe links or compliance footers.
    includeUnsubscribeLink: false,
    unsubscribeBaseUrl: (patch.unsubscribeBaseUrl ?? current.unsubscribeBaseUrl).trim() ||
      DEFAULT_SETTINGS.unsubscribeBaseUrl,
    localizeEmails: patch.localizeEmails ?? current.localizeEmails,
    targetMarkets: patch.targetMarkets ?? current.targetMarkets,
    targetCompanySignals: patch.targetCompanySignals ?? current.targetCompanySignals,
    maxContactsPerCompany: clampInt(
      patch.maxContactsPerCompany ?? current.maxContactsPerCompany,
      1,
      15,
      DEFAULT_SETTINGS.maxContactsPerCompany,
    ),
    crawlMaxPages: clampInt(
      patch.crawlMaxPages ?? current.crawlMaxPages,
      1,
      50,
      DEFAULT_SETTINGS.crawlMaxPages,
    ),
  };

  db.run(
    `UPDATE app_settings SET
      blocked_countries = ?,
      block_freemail = ?,
      brand_logo_url = ?,
      brand_product_url = ?,
      brand_primary_color = ?,
      daily_send_cap = ?,
      min_minutes_between_sends = ?,
      send_window_start_hour = ?,
      send_window_end_hour = ?,
      send_window_timezone = ?,
      send_on_weekends = ?,
      include_unsubscribe_link = ?,
      unsubscribe_base_url = ?,
      localize_emails = ?,
      target_markets = ?,
      target_company_signals = ?,
      max_contacts_per_company = ?,
      crawl_max_pages = ?,
      updated_at = datetime('now')
     WHERE id = 1`,
    [
      joinCsv(next.blockedCountries.map((code) => code.toUpperCase())),
      next.blockFreemail ? 1 : 0,
      next.brandLogoUrl.trim(),
      next.brandProductUrl,
      next.brandPrimaryColor,
      next.dailySendCap,
      next.minMinutesBetweenSends,
      next.sendWindowStartHour,
      next.sendWindowEndHour,
      next.sendWindowTimezone,
      next.sendOnWeekends ? 1 : 0,
      next.includeUnsubscribeLink ? 1 : 0,
      next.unsubscribeBaseUrl.replace(/\/$/, ""),
      next.localizeEmails ? 1 : 0,
      joinCsv(next.targetMarkets),
      joinCsv(next.targetCompanySignals),
      next.maxContactsPerCompany,
      next.crawlMaxPages,
    ],
  );

  return getSettings();
}

/** Human-readable send window in the configured local timezone. */
export function sendWindowLabel(settings: AppSettings = getSettings()): string {
  const zone = settings.sendWindowTimezone;
  const start = String(settings.sendWindowStartHour).padStart(2, "0");
  const end = String(settings.sendWindowEndHour).padStart(2, "0");
  let shortZone = zone;
  try {
    shortZone =
      new Intl.DateTimeFormat("en-GB", {
        timeZone: zone,
        timeZoneName: "short",
      })
        .formatToParts(new Date())
        .find((part) => part.type === "timeZoneName")?.value ?? zone;
  } catch {
    // keep the configured IANA zone name
  }
  return `${start}:00-${end}:00 ${shortZone}`;
}
