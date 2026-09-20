function str(key: string, fallback = ""): string {
  const raw = process.env[key];
  return raw === undefined || raw === "" ? fallback : raw;
}

function int(key: string, fallback: number): number {
  const parsed = Number.parseInt(str(key), 10);
  return Number.isFinite(parsed) ? parsed : fallback;
}

/**
 * Secrets and provider identity only. Product knobs (brand, compliance, send
 * windows, targeting, crawl depth) live in Settings / SQLite — see settings.ts.
 */
export const env = {
  port: int("PORT", 8787),
  databasePath: str("DATABASE_PATH", "./data/outreach.db"),

  llm: {
    openaiKey: str("OPENAI_API_KEY"),
    reasoningModel: str("LLM_MODEL_REASONING", "gpt-5-mini"),
    cheapModel: str("LLM_MODEL_CHEAP", "gpt-5.4-nano"),
  },

  crawler: {
    // Firecrawl is the only supported crawler. Plain fetch is an automatic
    // emergency fallback when Firecrawl is unavailable or returns nothing.
    firecrawlKey: str("FIRECRAWL_API_KEY"),
  },

  search: {
    serperKey: str("SERPER_API_KEY"),
  },

  verify: {
    reoonKey: str("REOON_API_KEY"),
  },

  sender: {
    provider: str("SENDER_PROVIDER", "dry_run") as "dry_run" | "brevo" | "gmail",
    /** Display name shared across providers. */
    fromName: str("SENDER_FROM_NAME", "Rohit from Observer"),
    /**
     * Legacy shared fallbacks. Prefer BREVO_* / GMAIL_* when set.
     * Kept so older .env files keep working.
     */
    fromEmail: str("SENDER_FROM_EMAIL", "rohit@getobserver.app"),
    replyTo: str("SENDER_REPLY_TO"),
    brevo: {
      apiKey: str("BREVO_API_KEY"),
      fromEmail: str("BREVO_FROM_EMAIL"),
      replyTo: str("BREVO_REPLY_TO"),
    },
    gmail: {
      /** Google Account → App passwords (spaces optional). */
      appPassword: str("GMAIL_APP_PASSWORD"),
      fromEmail: str("GMAIL_FROM_EMAIL"),
      replyTo: str("GMAIL_REPLY_TO"),
      host: str("GMAIL_SMTP_HOST", "smtp.gmail.com"),
      port: int("GMAIL_SMTP_PORT", 465),
    },
  },
};

/** Active From / Reply-To for the configured sender provider. */
export function activeSenderAddresses(): { fromEmail: string; replyTo: string } {
  const sharedFrom = env.sender.fromEmail.trim();
  const sharedReply = env.sender.replyTo.trim();

  if (env.sender.provider === "gmail") {
    const fromEmail = env.sender.gmail.fromEmail.trim() || sharedFrom;
    const replyTo = env.sender.gmail.replyTo.trim() || sharedReply || fromEmail;
    return { fromEmail, replyTo };
  }

  if (env.sender.provider === "brevo") {
    const fromEmail = env.sender.brevo.fromEmail.trim() || sharedFrom;
    const replyTo = env.sender.brevo.replyTo.trim() || sharedReply || fromEmail;
    return { fromEmail, replyTo };
  }

  // dry_run — prefer shared, else any provider-specific that happens to be set
  const fromEmail =
    sharedFrom ||
    env.sender.brevo.fromEmail.trim() ||
    env.sender.gmail.fromEmail.trim() ||
    "dry-run@localhost";
  const replyTo =
    sharedReply ||
    env.sender.brevo.replyTo.trim() ||
    env.sender.gmail.replyTo.trim() ||
    fromEmail;
  return { fromEmail, replyTo };
}

/**
 * Which stages can run for real vs. fall back to offline stubs. Surfaced in
 * the UI so it is always obvious what is live and what is simulated.
 */
export function capabilities() {
  return {
    llm: Boolean(env.llm.openaiKey),
    crawler: Boolean(env.crawler.firecrawlKey),
    search: Boolean(env.search.serperKey),
    verify: Boolean(env.verify.reoonKey),
    sender: env.sender.provider,
    sendingLive: env.sender.provider !== "dry_run",
  };
}
