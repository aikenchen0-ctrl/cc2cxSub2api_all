import { db } from "../db/index";

/**
 * Persist catch-all / credit-protection memory for email verification.
 * Cross-project on purpose: Reoon's answer for a domain does not depend on
 * which campaign asked.
 *
 * Catch-all is declared only after several consecutive people had EVERY pattern
 * guess come back `risky` with no `valid` and no `invalid`. A single risky
 * address must never short-circuit the remaining patterns for that person —
 * the valid inbox may be pattern #4 or #5.
 */
type DomainVerifyRow = {
  domain: string;
  is_catch_all: number;
  /** Consecutive people whose full pattern set came back all-risky. */
  risky_streak: number;
  valid_hits: number;
  last_status: string;
  updated_at: string;
};

/** How many all-risky people in a row before we stop probing a domain. */
export const CATCH_ALL_PERSON_THRESHOLD = 3;

function ensureTable(): void {
  db.exec(`
    CREATE TABLE IF NOT EXISTS domain_verify_state (
      domain TEXT PRIMARY KEY,
      is_catch_all INTEGER NOT NULL DEFAULT 0,
      risky_streak INTEGER NOT NULL DEFAULT 0,
      valid_hits INTEGER NOT NULL DEFAULT 0,
      last_status TEXT NOT NULL DEFAULT '',
      updated_at TEXT NOT NULL DEFAULT (datetime('now'))
    );
  `);
}

ensureTable();

function normalise(domain: string): string {
  return domain.trim().toLowerCase().replace(/^www\./, "");
}

export type PersonProbeOutcome =
  | "valid"
  | "all_risky"
  | "saw_invalid"
  | "empty";

export const domainVerifyState = {
  isCatchAll(domain: string): boolean {
    const key = normalise(domain);
    if (!key) return false;
    const row = db
      .query<DomainVerifyRow, [string]>(
        `SELECT * FROM domain_verify_state WHERE domain = ?`,
      )
      .get(key);
    return Boolean(row?.is_catch_all);
  },

  allRiskyStreak(domain: string): number {
    const key = normalise(domain);
    if (!key) return 0;
    return (
      db
        .query<{ risky_streak: number }, [string]>(
          `SELECT risky_streak FROM domain_verify_state WHERE domain = ?`,
        )
        .get(key)?.risky_streak ?? 0
    );
  },

  /**
   * Record the outcome of one person's full pattern probe.
   *
   * - `valid` / `saw_invalid` → domain is not behaving like catch-all; reset streak
   * - `all_risky` → bump streak; at threshold, mark catch-all for future people
   */
  recordPersonOutcome(domain: string, outcome: PersonProbeOutcome): {
    isCatchAll: boolean;
    allRiskyStreak: number;
  } {
    const key = normalise(domain);
    if (!key || outcome === "empty") {
      return { isCatchAll: this.isCatchAll(domain), allRiskyStreak: this.allRiskyStreak(domain) };
    }

    const existing = db
      .query<DomainVerifyRow, [string]>(
        `SELECT * FROM domain_verify_state WHERE domain = ?`,
      )
      .get(key);

    let streak = existing?.risky_streak ?? 0;
    let validHits = existing?.valid_hits ?? 0;
    let isCatchAll = Boolean(existing?.is_catch_all);

    if (outcome === "valid") {
      streak = 0;
      validHits += 1;
    } else if (outcome === "saw_invalid") {
      // Invalid locals prove the domain is not catch-all.
      streak = 0;
    } else if (outcome === "all_risky") {
      streak += 1;
      if (streak >= CATCH_ALL_PERSON_THRESHOLD) isCatchAll = true;
    }

    db.run(
      `INSERT INTO domain_verify_state
         (domain, is_catch_all, risky_streak, valid_hits, last_status, updated_at)
       VALUES (?, ?, ?, ?, ?, datetime('now'))
       ON CONFLICT (domain) DO UPDATE SET
         is_catch_all = CASE
           WHEN excluded.is_catch_all = 1 OR domain_verify_state.is_catch_all = 1 THEN 1
           ELSE 0
         END,
         risky_streak = excluded.risky_streak,
         valid_hits = excluded.valid_hits,
         last_status = excluded.last_status,
         updated_at = datetime('now')`,
      [key, isCatchAll ? 1 : 0, streak, validHits, outcome],
    );

    return { isCatchAll, allRiskyStreak: streak };
  },
};

/**
 * @deprecated Project-wide risky pause was removed. Catch-all protection is
 * domain-scoped via recordPersonOutcome / isCatchAll. Kept as a no-op so older
 * imports/tests do not break mid-refactor.
 */
export function shouldPauseVerification(_projectId: number, _lookback = 12): {
  pause: boolean;
  reason?: string;
} {
  return { pause: false };
}
