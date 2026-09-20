import type { JobStage } from "../../db/types";
import { draftMessage } from "./draftMessage";
import { findCompanies } from "./findCompanies";
import { findContacts } from "./findContacts";
import { generateAudienceEmail } from "./generateAudienceEmail";
import { generateAudiences } from "./generateAudiences";
import { regenerateAudience } from "./regenerateAudience";
import { scanSite } from "./scanSite";
import { sendMessage } from "./sendMessage";
import { verifyContact } from "./verifyContact";

export type StageHandler = (projectId: number, payload: any) => Promise<string>;

export const STAGES: Record<JobStage, StageHandler> = {
  scan_site: (projectId, payload) => scanSite(projectId, payload ?? {}),
  generate_audiences: (projectId, payload) => generateAudiences(projectId, payload ?? {}),
  generate_audience_email: generateAudienceEmail,
  regenerate_audience: regenerateAudience,
  find_companies: findCompanies,
  find_contacts: findContacts,
  verify_contact: verifyContact,
  draft_message: draftMessage,
  send_message: sendMessage,
};

export const STAGE_LABELS: Record<JobStage, string> = {
  scan_site: "Build profile",
  generate_audiences: "Generate audiences",
  generate_audience_email: "Write audience email",
  regenerate_audience: "Refresh audience",
  find_companies: "Find targets",
  find_contacts: "Find contacts",
  verify_contact: "Verify email",
  draft_message: "Write email",
  send_message: "Send email",
};
