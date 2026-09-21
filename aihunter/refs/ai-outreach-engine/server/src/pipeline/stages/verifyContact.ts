import { isRoleOrGenericEmail } from "../../compliance/guards";
import { companies, contacts, domainEmailPatterns, events, jobs } from "../../db/repo";
import { detectEmailPattern } from "../../providers/emailPatterns";
import {
  markVerifyQuotaExhausted,
  verificationGate,
} from "../../providers/verifyGate";
import { isQuotaOrProviderFailure, verifyEmail } from "../../providers/verify";
import { RetryLater } from "../errors";

export async function verifyContact(
  projectId: number,
  payload: { contactId: number },
): Promise<string> {
  const contact = contacts.get(payload.contactId);
  if (!contact) throw new Error(`Contact ${payload.contactId} not found`);

  // Role inboxes are never worth a verify credit or a cold send.
  if (isRoleOrGenericEmail(contact.email)) {
    contacts.setVerification(contact.id, "invalid", null);
    return `${contact.email}: role/generic inbox, dropped without verification.`;
  }

  // Discovery contacts must be named people. Manual/imported may be exceptions.
  const userSupplied =
    contact.email_source === "manual" || contact.email_source === "imported";
  if (!userSupplied && !contact.full_name.trim()) {
    contacts.setVerification(contact.id, "invalid", null);
    return `${contact.email}: nameless discovery contact, dropped without verification.`;
  }

  // Already resolved on a prior attempt — don't spend another verify credit.
  if (contact.verify_status === "valid" || contact.verify_status === "risky") {
    jobs.enqueue(projectId, "draft_message", { contactId: contact.id });
    return `${contact.email}: already ${contact.verify_status}, queued for drafting.`;
  }
  if (contact.verify_status === "invalid") {
    return `${contact.email}: already invalid, dropped.`;
  }

  const gate = verificationGate(projectId);
  if (!gate.allow) {
    events.log("verify_skipped", {
      projectId,
      ref: contact.email,
      data: { reason: gate.reason, during: "verify_contact", kind: gate.kind },
    });
    // Do not draft guessed addresses. Resume verify after topping up Reoon.
    throw new RetryLater(
      `Verification paused (${gate.kind}). Top up Reoon, then resume verify.`,
      60 * 30,
    );
  }

  try {
    const result = await verifyEmail(contact.email, projectId);
    contacts.setVerification(contact.id, result.status, result.score);

    // Learn formats from any valid named address (pattern or scraped) so later
    // people at this domain can skip unlikely probes.
    if (result.status === "valid" && contact.full_name) {
      const company = companies.get(contact.company_id);
      const domain = company?.domain.trim().toLowerCase() ?? "";
      const patternKey = detectEmailPattern(contact.email, contact.full_name, domain || undefined);
      if (patternKey && domain) domainEmailPatterns.recordHit(domain, patternKey);
    }

    // `risky` covers catch-all domains, which are worth a send at low volume but
    // are the first thing to cut if the bounce rate climbs.
    if (result.status === "valid" || result.status === "risky") {
      jobs.enqueue(projectId, "draft_message", { contactId: contact.id });
      return `${contact.email}: ${result.status}, queued for drafting.`;
    }
    return `${contact.email}: ${result.status}, dropped.`;
  } catch (error) {
    if (isQuotaOrProviderFailure(error)) {
      const reason = error instanceof Error ? error.message : String(error);
      markVerifyQuotaExhausted(projectId, reason);
      contacts.setVerification(contact.id, "unverified", null);
      throw new RetryLater(
        `Reoon quota exhausted while verifying ${contact.email}. Top up credits, then resume verify.`,
        60 * 30,
      );
    }
    throw error;
  }
}
