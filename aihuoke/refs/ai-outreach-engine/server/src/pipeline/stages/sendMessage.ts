import { contacts, events, messages } from "../../db/repo";
import { withFooter } from "../../compliance/footer";
import { preflight, screenContact, screenSendWindow } from "../../compliance/guards";
import { activeSender } from "../../providers/sender";
import { RetryLater } from "../errors";

export async function sendMessage(
  projectId: number,
  payload: { messageId: number },
): Promise<string> {
  const message = messages.get(payload.messageId);
  if (!message) throw new Error(`Message ${payload.messageId} not found`);

  if (message.status === "sent") return `Message ${message.id} was already sent.`;
  if (message.status !== "approved") {
    return `Message ${message.id} is "${message.status}", not approved; nothing sent.`;
  }

  const blockers = preflight();
  if (blockers.length > 0) {
    messages.setStatus(message.id, "failed", { error: blockers.join(" ") });
    throw new Error(`Sender is not configured: ${blockers.join(" ")}`);
  }

  const contact = contacts.get(message.contact_id);
  if (!contact) throw new Error(`Contact ${message.contact_id} not found`);

  // Re-checked here rather than trusting the draft-time result: the contact may
  // have unsubscribed or been suppressed in between.
  const contactVerdict = screenContact(contact);
  if (!contactVerdict.ok) {
    messages.setStatus(message.id, "skipped", { error: contactVerdict.reason });
    return `Skipped ${contact.email}: ${contactVerdict.reason}`;
  }

  const windowVerdict = screenSendWindow(projectId);
  if (!windowVerdict.ok) {
    // Short delay so config changes (weekend flag, window hours) take effect
    // quickly instead of sitting behind a long stale timer.
    const delaySeconds = windowVerdict.reason.startsWith("daily cap")
      ? 30 * 60
      : 2 * 60;
    throw new RetryLater(windowVerdict.reason, delaySeconds);
  }

  const textBody = withFooter(message.body, message.unsubscribe_token);

  messages.setStatus(message.id, "sending");
  const sender = activeSender();

  try {
    const result = await sender.send({
      to: contact.email,
      toName: contact.full_name,
      subject: message.subject,
      body: textBody,
      // Plain text only — no branded HTML / custom headers (Brevo rejects empty headers).
    });

    messages.setStatus(message.id, "sent", {
      provider_message_id: result.providerMessageId,
      sent_at: new Date().toISOString().replace("T", " ").slice(0, 19),
      email_format: "plain",
      sent_text_body: textBody,
      sent_html_body: "",
    });
    events.log("send", {
      projectId,
      ref: contact.email,
      data: {
        provider: result.provider,
        messageId: message.id,
        emailFormat: "plain",
      },
    });
    return `Sent message ${message.id} to ${contact.email} via ${result.provider}.`;
  } catch (error) {
    const reason = error instanceof Error ? error.message : String(error);
    messages.setStatus(message.id, "approved", { error: reason });
    throw new Error(`Send failed for ${contact.email}: ${reason}`);
  }
}
