import { env } from "../../env";
import { brevoSender } from "./brevo";
import { dryRunSender } from "./dryRun";
import { gmailSender } from "./gmail";
import type { Sender } from "./types";

export type { OutboundEmail, SendResult, Sender } from "./types";

export function activeSender(): Sender {
  switch (env.sender.provider) {
    case "brevo":
      return brevoSender;
    case "gmail":
      return gmailSender;
    default:
      return dryRunSender;
  }
}
