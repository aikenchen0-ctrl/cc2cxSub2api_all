import { activeSenderAddresses, env } from "../../env";
import { httpJson } from "../http";
import type { Sender } from "./types";

/**
 * Brevo sender for the user's chosen simple setup.
 *
 * We still send one approved message at a time from our app, rather than
 * creating a Brevo campaign, because we need local approval, local sent-state,
 * suppression checks and per-contact audit history.
 *
 * From / Reply-To: BREVO_FROM_EMAIL + BREVO_REPLY_TO (fallback: SENDER_*).
 */
export const brevoSender: Sender = {
  name: "brevo",
  async send(email) {
    const { fromEmail, replyTo } = activeSenderAddresses();
    // Build payload without a `headers` key. Brevo returns
    // missing_parameter "headers is blank" for `"headers": {}`.
    const customHeaders = email.headers
      ? Object.fromEntries(
          Object.entries(email.headers).filter(([, value]) => Boolean(value?.trim())),
        )
      : {};

    const body: Record<string, unknown> = {
      sender: { name: env.sender.fromName, email: fromEmail },
      to: [{ email: email.to, name: email.toName || undefined }],
      replyTo: { email: replyTo || fromEmail, name: env.sender.fromName },
      subject: email.subject,
      textContent: email.body,
    };
    if (email.htmlBody?.trim()) {
      body.htmlContent = email.htmlBody;
    }
    if (Object.keys(customHeaders).length > 0) {
      body.headers = customHeaders;
    }

    const payload = await httpJson<{ messageId?: string }>(
      "brevo",
      "https://api.brevo.com/v3/smtp/email",
      {
        method: "POST",
        headers: {
          "api-key": env.sender.brevo.apiKey,
          "Content-Type": "application/json",
        },
        body: JSON.stringify(body),
      },
    );

    return { provider: "brevo", providerMessageId: payload.messageId ?? "" };
  },
};
