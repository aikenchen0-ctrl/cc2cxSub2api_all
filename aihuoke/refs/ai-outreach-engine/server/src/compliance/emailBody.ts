/**
 * Normalize cold-email bodies for storage, preview, and send.
 *
 * - Preserve intentional single newlines (e.g. signature lines).
 * - Keep blank lines as paragraph breaks.
 * - Only auto-split when the model returns a true one-liner (no newlines).
 */

function unescapeLiteralNewlines(text: string): string {
  if (text.includes("\n")) return text;
  if (!/\\n/.test(text)) return text;
  return text.replace(/\\n/g, "\n").replace(/\\t/g, " ");
}

function sentences(text: string): string[] {
  const matches = text.match(/[^.!?]+[.!?]+(?:["')\]]+)?|[^.!?]+$/g);
  if (!matches) return text.trim() ? [text.trim()] : [];
  return matches.map((part) => part.trim()).filter(Boolean);
}

function groupSentences(parts: string[]): string[] {
  const paragraphs: string[] = [];
  let buffer: string[] = [];

  const flush = () => {
    if (buffer.length === 0) return;
    paragraphs.push(buffer.join(" "));
    buffer = [];
  };

  for (const sentence of parts) {
    const words = sentence.split(/\s+/).length;
    const isAsk = /\?["')\]]*$/.test(sentence);
    // Cold email: long beats and questions stand alone.
    if (words > 18 || isAsk) {
      flush();
      paragraphs.push(sentence);
      continue;
    }
    buffer.push(sentence);
    if (buffer.length >= 2) flush();
  }
  flush();
  return paragraphs;
}

function splitWallOfText(text: string): string {
  let rest = text.trim();
  const blocks: string[] = [];

  const greeting = rest.match(/^(hi|hey|hello)\s+[^,]{1,40},\s*/i);
  if (greeting) {
    blocks.push(greeting[0].trim());
    rest = rest.slice(greeting[0].length).trim();
  }

  let signature = "";
  const sig = rest.match(/\s+([A-Za-z][A-Za-z.'-]{0,24})\s*$/);
  if (sig && sig.index !== undefined && sig.index > 40) {
    const candidate = sig[1] ?? "";
    // Avoid treating the last word of a sentence as a signature.
    const before = rest.slice(0, sig.index).trimEnd();
    if (/[.!?]$/.test(before) || before.endsWith("?")) {
      signature = candidate;
      rest = before;
    }
  }

  blocks.push(...groupSentences(sentences(rest)));
  if (signature) blocks.push(signature);
  return blocks.filter(Boolean).join("\n\n");
}

/** Normalize spacing for storage, preview, and send. */
export function normalizeEmailBody(body: string): string {
  let text = unescapeLiteralNewlines(body).replace(/\r\n/g, "\n");
  if (!text.trim()) return "";

  // True one-liner from the model → invent paragraph breaks.
  if (!text.includes("\n")) {
    return splitWallOfText(text.trim()).replace(/\n{3,}/g, "\n\n").trim();
  }

  // Keep single newlines (signature stacks) and blank lines (paragraphs).
  // Only trim per-line whitespace; do not join soft wraps into one line.
  text = text
    .split("\n")
    .map((line) => line.trimEnd())
    .join("\n")
    .replace(/\n{3,}/g, "\n\n")
    .replace(/^\n+/, "")
    .replace(/\n+$/, "");

  return text.trimEnd();
}
