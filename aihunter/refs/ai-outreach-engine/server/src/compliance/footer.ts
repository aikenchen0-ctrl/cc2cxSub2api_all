import { randomBytes } from "node:crypto";
import { getSettings } from "../settings";
import { normalizeEmailBody } from "./emailBody";

/** Kept for message rows / legacy /u/:token links; not appended to send bodies. */
export function newUnsubscribeToken(): string {
  return randomBytes(16).toString("hex");
}

/** Sent body is the draft as-is (normalized). No opt-out or postal footer. */
export function withFooter(body: string, _token?: string): string {
  return normalizeEmailBody(body);
}

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function paragraphs(body: string): string {
  const blocks = normalizeEmailBody(body).split(/\n{2,}/);
  return blocks
    .map((part, index) => {
      const margin = index === blocks.length - 1 ? "0" : "0 0 16px";
      return `<p style="margin:${margin};font:15px/1.55 -apple-system,BlinkMacSystemFont,'Segoe UI',Arial,sans-serif;color:#1f2937">${escapeHtml(
        part,
      ).replace(/\n/g, "<br>")}</p>`;
    })
    .join("\n");
}

/** Legacy branded HTML wrapper (unused by send path; kept for tests). */
export function withHtmlFooter(body: string, _token?: string): string {
  const settings = getSettings();
  const productUrl = settings.brandProductUrl || "https://getobserver.app";
  const primaryColor = settings.brandPrimaryColor || "#6ea8fe";
  const logo = settings.brandLogoUrl
    ? `<a href="${escapeHtml(productUrl)}" style="display:inline-block;margin:0 0 20px"><img src="${escapeHtml(
        settings.brandLogoUrl,
      )}" alt="Observer" width="96" style="display:block;border:0;max-width:96px;height:auto"></a>`
    : `<a href="${escapeHtml(
        productUrl,
      )}" style="display:inline-block;margin:0 0 20px;color:${escapeHtml(
        primaryColor,
      )};font:600 18px -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;text-decoration:none">Observer</a>`;

  return `<!DOCTYPE html>
<html>
<body style="margin:0;padding:0;background:#f3f4f6">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f3f4f6">
    <tr>
      <td align="center" style="padding:24px 12px">
        <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;background:#ffffff;border-radius:12px">
          <tr>
            <td style="padding:28px">
              ${logo}
              ${paragraphs(body)}
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`;
}

/** No List-Unsubscribe headers. */
export function unsubscribeHeaders(_token: string): Record<string, string> {
  return {};
}
