import { isRoleOrGenericEmail, looksLikePersonalEmail } from "../compliance/guards";
import { domainEmailPatterns, events } from "../db/repo";
import { getSettings } from "../settings";
import { throwIfCancelled } from "../pipeline/context";
import { JobCancelled, RetryLater } from "../pipeline/errors";
import { webSearch } from "./search";
import { scrapePublicEmails } from "./crawler";
import {
  detectEmailPattern,
  emailPatterns,
  isEmailPatternKey,
  orderedEmailCandidates,
  RANKED_PATTERN_LIMIT,
  type EmailPatternKey,
} from "./emailPatterns";
import {
  CATCH_ALL_PERSON_THRESHOLD,
  domainVerifyState,
  type PersonProbeOutcome,
} from "./domainVerifyState";
import {
  markVerifyQuotaExhausted,
  verificationGate,
} from "./verifyGate";
import { isQuotaOrProviderFailure, verifyAvailable, verifyEmail } from "./verify";
import { finalizeSearchQuery, sanitizeQueryTerm } from "./searchQuerySafe";

export type FoundContact = {
  fullName: string;
  title: string;
  email: string;
  source: string;
  evidenceUrl: string;
  verifyStatus?: string;
  verifyScore?: number | null;
};

export { emailPatterns, orderedEmailCandidates, detectEmailPattern };

function rankedKeysForDomain(domain: string): EmailPatternKey[] {
  return domainEmailPatterns
    .ranked(domain, RANKED_PATTERN_LIMIT)
    .filter(isEmailPatternKey);
}

/**
 * Read "Jane Doe - Head of Sales - Acme | LinkedIn" style search result titles.
 * One search query per company is far cheaper than a people-database seat.
 */
export function parsePersonFromSerp(title: string): { fullName: string; title: string } | null {
  const cleaned = title
    .replace(/\s*[|\-–]\s*LinkedIn\s*$/i, "")
    .replace(/\s+on\s+LinkedIn\s*$/i, "")
    .trim();
  const segments = cleaned.split(/\s+[-–|]\s+/).map((part) => part.trim());
  const name = segments[0] ?? "";
  const words = name.split(/\s+/);
  if (words.length < 2 || words.length > 4) return null;
  if (!/^[A-Z][\p{L}'.-]*$/u.test(words[0]!)) return null;
  // Reject org/page titles that look like people but aren't.
  if (/^(about|home|team|careers|jobs|company|official)\b/i.test(name)) return null;
  return { fullName: name, title: segments[1] ?? "" };
}

/**
 * Serper free tier often rejects `site:linkedin.com/in … OR …` and even
 * quoted multi-word phrases with "Query pattern not allowed".
 * Use plain keywords — "linkedin" is enough for Google to rank /in/ hits.
 */
export function buildPeopleSearchQuery(companyName: string, title?: string): string {
  const company = sanitizeQueryTerm(companyName);
  const role = title ? sanitizeQueryTerm(title) : "";
  return finalizeSearchQuery([company, role, "linkedin"].filter(Boolean));
}

/**
 * True when a LinkedIn SERP title refers to the target company.
 * Title-only on purpose: snippets often echo the query company even for
 * competitor profiles ("Olivia @ Runway" in a Canva search).
 */
export function serpMentionsCompany(
  companyName: string,
  domain: string,
  title: string,
  _snippet = "",
): boolean {
  void _snippet;
  const hay = title.toLowerCase();
  const compactHay = hay.replace(/[^a-z0-9]+/g, "");
  const name = sanitizeQueryTerm(companyName).toLowerCase();
  if (name && hay.includes(name)) return true;

  const compactName = name.replace(/[^a-z0-9]+/g, "");
  if (compactName.length >= 4 && compactHay.includes(compactName)) return true;

  const brand = domain
    .trim()
    .toLowerCase()
    .replace(/^www\./, "")
    .split(".")[0] ?? "";
  if (brand.length >= 4 && (hay.includes(brand) || compactHay.includes(brand))) return true;

  return false;
}

function bestGuessEmail(
  fullName: string,
  domain: string,
  rankedKeys: EmailPatternKey[],
): string | null {
  return (
    orderedEmailCandidates(fullName, domain, rankedKeys).find(
      (email) => !isRoleOrGenericEmail(email),
    ) ?? null
  );
}

async function findPeopleViaSearch(args: {
  companyName: string;
  domain: string;
  jobTitles: string[];
  projectId: number;
  limit: number;
}): Promise<Array<{ fullName: string; title: string; evidenceUrl: string }>> {
  const people: Array<{ fullName: string; title: string; evidenceUrl: string }> = [];
  const seen = new Set<string>();
  // One title per query — OR groups trigger Serper free-tier pattern blocks.
  // Cap to 3 titles to stay under Serper's 5 req/s free-tier ceiling.
  const titles =
    args.jobTitles.length > 0
      ? args.jobTitles
          .map((title) => sanitizeQueryTerm(title))
          .filter(Boolean)
          .slice(0, 3)
      : [""];

  for (let i = 0; i < titles.length; i++) {
    const title = titles[i]!;
    if (people.length >= args.limit) break;
    throwIfCancelled();
    const query = buildPeopleSearchQuery(args.companyName, title || undefined);
    if (!query) continue;

    // Stay under Serper free-tier 5 req/s when many companies queue at once.
    if (i > 0) await Bun.sleep(250);

    const results = await webSearch(query, {
      limit: Math.min(15, Math.max(args.limit * 2, 8)),
      projectId: args.projectId,
      // Company search strips LinkedIn; people search needs /in/ profiles.
      allowHosts: ["linkedin.com"],
    });

    for (const result of results) {
      if (!/linkedin\.com\/in\//i.test(result.link)) continue;
      if (!serpMentionsCompany(args.companyName, args.domain, result.title, result.snippet)) {
        continue;
      }
      const parsed = parsePersonFromSerp(result.title);
      if (!parsed) continue;
      const key = parsed.fullName.toLowerCase();
      if (seen.has(key)) continue;
      seen.add(key);
      people.push({
        fullName: parsed.fullName,
        title: parsed.title || title,
        evidenceUrl: result.link,
      });
      if (people.length >= args.limit) return people;
    }
  }
  return people;
}

async function resolvePersonEmail(args: {
  person: { fullName: string; title: string; evidenceUrl: string };
  domain: string;
  projectId: number;
  rankedKeys: EmailPatternKey[];
  domainIsCatchAll: boolean;
  verificationPaused: boolean;
}): Promise<{
  contact: FoundContact | null;
  rankedKeys: EmailPatternKey[];
  domainIsCatchAll: boolean;
  verificationPaused: boolean;
}> {
  let { rankedKeys, domainIsCatchAll, verificationPaused } = args;
  const { person, domain, projectId } = args;

  // Verify paused / no key: do NOT invent first@domain contacts. Zero contacts
  // is honest; unverified guesses are not. Catch-all domains still keep a
  // best-guess as risky-equivalent unverified because Reoon cannot decide.
  if (verificationPaused || !verifyAvailable()) {
    return { contact: null, rankedKeys, domainIsCatchAll, verificationPaused };
  }

  if (domainIsCatchAll || domainVerifyState.isCatchAll(domain)) {
    const guess = bestGuessEmail(person.fullName, domain, rankedKeys);
    if (!guess) {
      return { contact: null, rankedKeys, domainIsCatchAll, verificationPaused };
    }
    return {
      contact: {
        fullName: person.fullName,
        title: person.title,
        email: guess,
        source: "pattern",
        evidenceUrl: person.evidenceUrl,
        verifyStatus: "unverified",
        verifyScore: null,
      },
      rankedKeys,
      domainIsCatchAll,
      verificationPaused,
    };
  }

  let resolved: FoundContact | null = null;
  try {
    const tried = new Set<string>();
    let sawRisky = false;
    let sawInvalid = false;
    let checks = 0;
    let firstRisky: FoundContact | null = null;

    // Cap probes — wrong people × many patterns burns Reoon credits fast.
    const candidates = orderedEmailCandidates(person.fullName, domain, rankedKeys).slice(0, 3);

    for (const candidate of candidates) {
      throwIfCancelled();
      if (isRoleOrGenericEmail(candidate)) continue;
      if (tried.has(candidate)) continue;
      tried.add(candidate);

      const verdict = await verifyEmail(candidate, projectId);
      checks += 1;

      if (verdict.status === "valid") {
        const patternKey = detectEmailPattern(candidate, person.fullName, domain);
        if (patternKey) {
          domainEmailPatterns.recordHit(domain, patternKey);
          rankedKeys = rankedKeysForDomain(domain);
        }
        resolved = {
          fullName: person.fullName,
          title: person.title,
          email: candidate,
          source: "pattern",
          evidenceUrl: person.evidenceUrl,
          verifyStatus: verdict.status,
          verifyScore: verdict.score,
        };
        break;
      }

      if (verdict.status === "invalid") {
        sawInvalid = true;
        continue;
      }

      if (verdict.status === "risky") {
        sawRisky = true;
        if (!firstRisky) {
          firstRisky = {
            fullName: person.fullName,
            title: person.title,
            email: candidate,
            source: "pattern",
            evidenceUrl: person.evidenceUrl,
            verifyStatus: verdict.status,
            verifyScore: verdict.score,
          };
        }
        continue;
      }
      // unknown — keep probing; do not persist as a contact
    }

    let outcome: PersonProbeOutcome = "empty";
    if (resolved?.verifyStatus === "valid") {
      outcome = "valid";
    } else if (checks === 0) {
      outcome = "empty";
    } else if (sawRisky) {
      outcome = "all_risky";
      resolved = firstRisky;
    } else if (sawInvalid) {
      outcome = "saw_invalid";
      // All probes invalid → no contact. Do not invent an unverified address.
    }

    const domainState = domainVerifyState.recordPersonOutcome(domain, outcome);
    if (domainState.isCatchAll && !domainIsCatchAll) {
      domainIsCatchAll = true;
      events.log("verify_catch_all", {
        projectId,
        ref: domain,
        data: {
          action: "mark_catch_all",
          threshold: CATCH_ALL_PERSON_THRESHOLD,
          allRiskyPeople: domainState.allRiskyStreak,
          reason: `${CATCH_ALL_PERSON_THRESHOLD} consecutive people had all pattern guesses return risky`,
        },
      });
    }

    const gate = verificationGate(projectId);
    if (!gate.allow && !verificationPaused) {
      verificationPaused = true;
      events.log("verify_circuit_open", {
        projectId,
        ref: domain,
        data: { reason: gate.reason, during: "pattern_guess", kind: gate.kind },
      });
    }
  } catch (error) {
    if (error instanceof JobCancelled) throw error;
    if (!isQuotaOrProviderFailure(error)) throw error;
    const reason = error instanceof Error ? error.message : String(error);
    markVerifyQuotaExhausted(projectId, reason);
    verificationPaused = true;
    events.log("verify_skipped", {
      projectId,
      ref: `${person.fullName} @ ${domain}`,
      data: {
        reason,
        during: "pattern_guess",
        action: "pause_verify_stop_guessing",
      },
    });
    // Credits died mid-person — do not invent an unverified inbox.
    resolved = null;
  }

  return { contact: resolved, rankedKeys, domainIsCatchAll, verificationPaused };
}

/* ------------------------------------------------------------ waterfall */

/**
 * Find work emails for one company — people only:
 *
 *   1. LinkedIn people search for named humans in target roles
 *   2. Pattern-guess personal inboxes + verify (never role/generic)
 *   3. Optional scrape of mailto: ONLY when it looks like first.last
 *
 * Role inboxes (careers@, hello@, sales.team@, company@company) are never
 * returned, verified, or allowed to fill the contact quota.
 */
export async function findContactsForCompany(args: {
  projectId: number;
  companyName: string;
  domain: string;
  jobTitles: string[];
  maxContacts?: number;
}): Promise<FoundContact[]> {
  const settings = getSettings();
  const maxContacts = Math.min(args.maxContacts ?? settings.maxContactsPerCompany, 15);
  const found: FoundContact[] = [];
  const seenEmails = new Set<string>();
  const domain = args.domain.trim().toLowerCase();
  let rankedKeys = rankedKeysForDomain(domain);
  let domainIsCatchAll = domainVerifyState.isCatchAll(domain);
  let gate = verificationGate(args.projectId);
  let verificationPaused = !gate.allow;

  if (verificationPaused) {
    events.log("verify_skipped", {
      projectId: args.projectId,
      ref: domain,
      data: { reason: gate.reason, during: "find_contacts", kind: gate.kind },
    });
  }

  // Step 1: real people first — never let mailto scrapes consume the quota.
  const people = await findPeopleViaSearch({
    companyName: args.companyName,
    domain,
    jobTitles: args.jobTitles,
    projectId: args.projectId,
    limit: Math.max(maxContacts * 3, 12),
  }).catch((error) => {
    if (error instanceof JobCancelled) throw error;
    const message = error instanceof Error ? error.message : String(error);
    events.log("search_failed", {
      projectId: args.projectId,
      ref: args.companyName,
      data: {
        during: "find_people",
        domain,
        error: message,
      },
    });
    // Soft-empty would mark the job done with 0 contacts and never retry.
    if (/rate limit exceeded|\b429\b/i.test(message)) {
      throw new RetryLater(`Serper rate limited while finding people at ${args.companyName}`, 15);
    }
    return [];
  });

  for (const person of people) {
    if (found.length >= maxContacts) break;
    throwIfCancelled();

    const resolved = await resolvePersonEmail({
      person,
      domain,
      projectId: args.projectId,
      rankedKeys,
      domainIsCatchAll,
      verificationPaused,
    });
    rankedKeys = resolved.rankedKeys;
    domainIsCatchAll = resolved.domainIsCatchAll;
    verificationPaused = resolved.verificationPaused;

    const contact = resolved.contact;
    if (!contact) continue;
    if (!contact.fullName.trim()) continue;
    if (isRoleOrGenericEmail(contact.email)) continue;
    const key = contact.email.toLowerCase();
    if (seenEmails.has(key)) continue;
    seenEmails.add(key);
    found.push(contact);
  }

  // Step 2: scrape is a backfill only — personal-looking mailto with a name shape.
  if (found.length < maxContacts) {
    const scraped = await scrapePublicEmails(domain).catch(() => []);
    for (const hit of scraped) {
      if (found.length >= maxContacts) break;
      if (isRoleOrGenericEmail(hit.email)) continue;
      if (!looksLikePersonalEmail(hit.email)) continue;
      const key = hit.email.toLowerCase();
      if (seenEmails.has(key)) continue;
      seenEmails.add(key);

      // Derive a display name from jane.doe → "Jane Doe" so drafts aren't anonymous.
      const local = (hit.email.split("@")[0] ?? "").split("+")[0] ?? "";
      const fullName = local
        .split(/[._-]+/)
        .filter((part) => part.length > 1)
        .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
        .join(" ");
      if (!fullName || fullName.split(/\s+/).length < 2) continue;

      found.push({
        fullName,
        title: "",
        email: hit.email,
        source: "scraped",
        evidenceUrl: hit.url,
      });
    }
  }

  return found.filter(
    (contact) =>
      contact.fullName.trim().length > 0 &&
      !isRoleOrGenericEmail(contact.email),
  );
}
