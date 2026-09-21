import type { Sender } from "./types";

/**
 * Default sender. Logs the fully rendered message and returns a fake id, so the
 * whole pipeline including the compliance gates can be exercised before a
 * single real email leaves the machine.
 */
export const dryRunSender: Sender = {
  name: "dry_run",
  async send(email) {
    console.log(
      [
        "",
        "──────── DRY RUN, nothing was sent ────────",
        `To:      ${email.toName ? `${email.toName} <${email.to}>` : email.to}`,
        `Subject: ${email.subject}`,
        ...Object.entries(email.headers ?? {}).map(([key, value]) => `${key}: ${value}`),
        "",
        email.body,
        "",
        ...(email.htmlBody?.trim()
          ? ["──────── HTML VERSION ────────", email.htmlBody]
          : ["──────── PLAIN TEXT ONLY (no HTML) ────────"]),
        "───────────────────────────────────────────",
        "",
      ].join("\n"),
    );
    return {
      provider: "dry_run",
      providerMessageId: `dry-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    };
  },
};
