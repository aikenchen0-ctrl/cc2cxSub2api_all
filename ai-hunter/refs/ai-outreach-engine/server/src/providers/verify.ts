import { env } from "../env";
import { events } from "../db/repo";
import { httpJson, ProviderError } from "./http";
import { domainVerifyState } from "./domainVerifyState";

/**
 * `valid`     - safe to send.
 * `risky`     - catch-all or role address; deliverable but bounce risk is real.
 * `invalid`   - do not send, would count against the bounce rate.
 * `unknown`   - provider could not decide.
 * `unverified`- never checked.
 */
export type VerifyStatus = "valid" | "risky" | "invalid" | "unknown" | "unverified";

export type VerifyResult = { status: VerifyStatus; score: number | null; provider: string };

const REOON_COST = 0.001;

type ReoonPayload = {
  status?: string;
  is_safe_to_send?: boolean;
  overall_score?: number;
  is_catch_all?: boolean;
  is_role_account?: boolean;
  is_deliverable?: boolean;
};

/**
 * Reoon Power mode uses `safe` / `role` / `catch_all` / … (not `valid`).
 * Quick mode historically used `valid`. Map both.
 * @see https://www.reoon.com/articles/api-documentation-of-reoon-email-verifier/
 */
export function mapReoon(
  status: string,
  flags: Pick<ReoonPayload, "is_safe_to_send" | "is_catch_all" | "is_role_account"> = {},
): VerifyStatus {
  const normalized = status.toLowerCase().trim();
  switch (normalized) {
    case "safe":
    case "valid":
      return "valid";
    case "catch_all":
    case "role":
    case "role_account":
    case "inbox_full":
    case "spamtrap":
      return "risky";
    case "invalid":
    case "disabled":
    case "disposable":
      return "invalid";
    case "unknown":
      // Credits refunded by Reoon; do not override with stale flags.
      return "unknown";
    default:
      // Prefer Reoon's sendability flag over silently discarding the result.
      if (flags.is_safe_to_send) return "valid";
      if (flags.is_catch_all || flags.is_role_account) return "risky";
      return "unknown";
  }
}

function domainOf(email: string): string {
  const at = email.lastIndexOf("@");
  return at >= 0 ? email.slice(at + 1).toLowerCase() : "";
}

export function isQuotaOrProviderFailure(error: unknown): boolean {
  if (!(error instanceof ProviderError)) {
    return /credit|quota|limit|balance|payment|insufficient|timeout/i.test(
      error instanceof Error ? error.message : String(error),
    );
  }
  if (error.status === 402 || error.status === 429 || error.status === 403) return true;
  if (error.status >= 500) return true;
  return /credit|quota|limit|balance|payment|insufficient|timeout/i.test(error.message);
}

async function reoon(email: string): Promise<VerifyResult & { rawStatus: string }> {
  const url = new URL("https://emailverifier.reoon.com/api/v1/verify");
  url.searchParams.set("email", email);
  url.searchParams.set("key", env.verify.reoonKey);
  url.searchParams.set("mode", "power");
  const payload = await httpJson<ReoonPayload>("reoon", url.toString(), { timeoutMs: 60_000 });
  const rawStatus = payload.status ?? "unknown";
  const status = mapReoon(rawStatus, {
    is_safe_to_send: payload.is_safe_to_send,
    is_catch_all: payload.is_catch_all,
    is_role_account: payload.is_role_account,
  });
  const score =
    typeof payload.overall_score === "number"
      ? payload.overall_score
      : payload.is_safe_to_send
        ? 100
        : null;
  return { status, score, provider: "reoon", rawStatus };
}

export function verifyAvailable(): boolean {
  return Boolean(env.verify.reoonKey);
}

/** Verify with Reoon. */
export async function verifyEmail(
  email: string,
  projectId?: number,
): Promise<VerifyResult> {
  if (!verifyAvailable()) {
    return { status: "unverified", score: null, provider: "none" };
  }

  const domain = domainOf(email);
  // Only skip when the domain was proven catch-all across multiple people.
  // A single risky result must never short-circuit remaining pattern probes.
  if (domain && domainVerifyState.isCatchAll(domain)) {
    events.log("verify_skipped", {
      projectId,
      ref: email,
      data: { reason: "known_catch_all_domain", domain, status: "risky" },
    });
    return { status: "risky", score: null, provider: "reoon-cached" };
  }

  const result = await reoon(email);

  events.log("verify", {
    projectId,
    ref: email,
    data: {
      provider: result.provider,
      status: result.status,
      reoonStatus: result.rawStatus,
      score: result.score,
    },
    costUsd: REOON_COST,
  });
  return { status: result.status, score: result.score, provider: result.provider };
}
