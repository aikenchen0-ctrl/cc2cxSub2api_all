import { Hono } from "hono";
import { contacts, events, messages, suppressions } from "../db/repo";

export const unsubscribe = new Hono();

function page(title: string, detail: string): string {
  return `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>${title}</title>
<style>
  body{margin:0;min-height:100vh;display:grid;place-items:center;background:#0b0d10;color:#e8eaed;
       font:16px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
  main{max-width:34rem;padding:2.5rem;text-align:center}
  h1{font-size:1.5rem;margin:0 0 .75rem;font-weight:600}
  p{margin:0;color:#9aa0a6}
</style></head>
<body><main><h1>${title}</h1><p>${detail}</p></main></body></html>`;
}

/**
 * Honours the opt-out immediately and permanently. CAN-SPAM allows 10 business
 * days and GDPR expects 24-48 hours; doing it synchronously is simpler than
 * tracking a deadline, and the suppression list is checked before every send.
 */
function optOut(token: string): { ok: boolean; email?: string } {
  const message = messages.getByToken(token);
  if (!message) return { ok: false };

  const contact = contacts.get(message.contact_id);
  if (contact) {
    suppressions.add(contact.email, null, "unsubscribed", message.project_id);
    events.log("unsubscribe", {
      projectId: message.project_id,
      ref: contact.email,
      data: { messageId: message.id },
    });
    return { ok: true, email: contact.email };
  }
  return { ok: false };
}

// RFC 8058 one-click: mailbox providers POST here without any user interaction.
unsubscribe.post("/u/:token", (c) => {
  const result = optOut(c.req.param("token"));
  return c.text(result.ok ? "unsubscribed" : "unknown token", result.ok ? 200 : 404);
});

unsubscribe.get("/u/:token", (c) => {
  const result = optOut(c.req.param("token"));
  if (!result.ok) {
    return c.html(page("Link not recognised", "This unsubscribe link is not valid."), 404);
  }
  return c.html(
    page(
      "You are unsubscribed",
      `${result.email} has been added to the do-not-contact list. You will not receive further email from us.`,
    ),
  );
});
