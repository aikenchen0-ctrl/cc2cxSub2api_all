import { isRoleOrGenericEmail } from "../../compliance/guards";
import { findContactsForCompany } from "../../providers/contacts";
import { audiences, companies, contacts, jobs, projects } from "../../db/repo";
import { maxContactsForProject } from "../../workflow/preferences";

function queueDownstream(projectId: number, contactId: number, verifyStatus: string): void {
  if (verifyStatus === "invalid") return;
  if (verifyStatus === "valid" || verifyStatus === "risky") {
    jobs.ensure(projectId, "draft_message", { contactId });
    return;
  }
  jobs.ensure(projectId, "verify_contact", { contactId });
}

/**
 * Resume without rediscovery when a previous attempt already saved contacts.
 * Retrying after a verify/credit failure must not burn search credits again.
 */
function resumeExistingContacts(
  projectId: number,
  companyName: string,
  existing: ReturnType<typeof contacts.listByCompany>,
): string {
  for (const contact of existing) {
    queueDownstream(projectId, contact.id, contact.verify_status);
  }
  return `Resumed ${existing.length} existing contact(s) at ${companyName} without re-searching.`;
}

export async function findContacts(
  projectId: number,
  payload: { companyId: number; maxContacts?: number },
): Promise<string> {
  const company = companies.get(payload.companyId);
  if (!company) throw new Error(`Company ${payload.companyId} not found`);
  const audience = audiences.get(company.audience_id);
  if (!audience) throw new Error(`Audience ${company.audience_id} not found`);
  const project = projects.get(projectId);
  if (!project) throw new Error(`Project ${projectId} not found`);

  const existing = contacts.listByCompany(company.id).filter((row) => {
    if (!isRoleOrGenericEmail(row.email)) return true;
    // Drop legacy role inboxes so retries don't keep paying to verify them.
    if (row.verify_status !== "invalid") {
      contacts.setVerification(row.id, "invalid", null);
    }
    return false;
  });
  if (existing.length > 0) {
    return resumeExistingContacts(projectId, company.name, existing);
  }

  const found = await findContactsForCompany({
    projectId,
    companyName: company.name,
    domain: company.domain,
    jobTitles: audience.jobTitles,
    maxContacts: payload.maxContacts ?? maxContactsForProject(project),
  });

  let inserted = 0;
  for (const person of found) {
    // People only — never persist nameless or role/generic inboxes.
    if (!person.fullName.trim()) continue;
    if (isRoleOrGenericEmail(person.email)) continue;
    const row = contacts.upsert({
      projectId,
      audienceId: audience.id,
      companyId: company.id,
      fullName: person.fullName,
      title: person.title,
      email: person.email,
      emailSource: person.source,
      evidenceUrl: person.evidenceUrl,
      country: company.country,
    });
    if (!row) continue;
    inserted += 1;

    // The waterfall already verified pattern-derived addresses; anything else
    // still needs a check before it is allowed near the send queue.
    if (person.verifyStatus === "valid" || person.verifyStatus === "risky") {
      contacts.setVerification(row.id, person.verifyStatus, person.verifyScore ?? null);
      jobs.ensure(projectId, "draft_message", { contactId: row.id });
    } else if (person.verifyStatus === "unverified") {
      // Catch-all / deferred verify only — never draft until Reoon (or a human) decides.
      contacts.setVerification(row.id, "unverified", null);
      jobs.ensure(projectId, "verify_contact", { contactId: row.id });
    } else if (person.verifyStatus === "invalid") {
      contacts.setVerification(row.id, "invalid", person.verifyScore ?? null);
    } else {
      jobs.ensure(projectId, "verify_contact", { contactId: row.id });
    }
  }

  return inserted > 0
    ? `Found ${inserted} contact(s) at ${company.name}.`
    : `No reachable contact found at ${company.name}.`;
}
