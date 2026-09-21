export type OutboundEmail = {
  to: string;
  toName: string;
  subject: string;
  body: string;
  /** When omitted or empty, senders deliver plain text only (no HTML part). */
  htmlBody?: string;
  /** Optional custom SMTP headers. Omit entirely for Brevo (empty object is rejected). */
  headers?: Record<string, string>;
};

export type SendResult = { provider: string; providerMessageId: string };

export type Sender = {
  name: string;
  send(email: OutboundEmail): Promise<SendResult>;
};
