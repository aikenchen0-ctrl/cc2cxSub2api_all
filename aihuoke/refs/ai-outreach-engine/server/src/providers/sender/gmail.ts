import nodemailer from "nodemailer";
import { activeSenderAddresses, env } from "../../env";
import type { Sender } from "./types";

/**
 * Gmail SMTP via an App Password.
 *
 * Setup:
 * 1. Google Account → Security → 2-Step Verification ON
 * 2. App passwords → create one for "Mail"
 * 3. SENDER_PROVIDER=gmail
 *    GMAIL_FROM_EMAIL=you@gmail.com  (must match the Google account)
 *    GMAIL_REPLY_TO=you@gmail.com    (optional; defaults to from)
 *    GMAIL_APP_PASSWORD=xxxx xxxx xxxx xxxx
 *
 * Note: Google Workspace custom domains also work if SMTP is allowed and the
 * From address belongs to that account. Free @gmail.com daily send limits apply
 * (~100–500/day depending on account age/reputation) — keep the app daily cap low.
 */
export const gmailSender: Sender = {
  name: "gmail",
  async send(email) {
    const { fromEmail, replyTo } = activeSenderAddresses();
    const user = fromEmail.trim();
    const pass = env.sender.gmail.appPassword.replace(/\s+/g, "");
    if (!user || !pass) {
      throw new Error("Gmail SMTP needs GMAIL_FROM_EMAIL (or SENDER_FROM_EMAIL) and GMAIL_APP_PASSWORD.");
    }

    const transporter = nodemailer.createTransport({
      host: env.sender.gmail.host,
      port: env.sender.gmail.port,
      secure: env.sender.gmail.port === 465,
      auth: { user, pass },
    });

    const info = await transporter.sendMail({
      from: {
        name: env.sender.fromName,
        address: user,
      },
      to: email.toName ? `"${email.toName.replace(/"/g, "")}" <${email.to}>` : email.to,
      replyTo: replyTo || user,
      subject: email.subject,
      text: email.body,
      ...(email.htmlBody?.trim() ? { html: email.htmlBody } : {}),
      headers: email.headers,
    });

    return {
      provider: "gmail",
      providerMessageId: String(info.messageId ?? ""),
    };
  },
};
