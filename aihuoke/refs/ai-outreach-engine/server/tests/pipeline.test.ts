import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

// env.ts snapshots process.env at import time, so configure before importing.
// Settings knobs are seeded into SQLite from these env vars on first getSettings().
const workDir = mkdtempSync(join(tmpdir(), "outreach-engine-test-"));
process.env.DATABASE_PATH = join(workDir, "test.db");
process.env.SENDER_PROVIDER = "dry_run";
process.env.SENDER_FROM_EMAIL = "hello@tryobserver.app";
process.env.SENDER_FROM_NAME = "Rohit from Observer";
process.env.SEND_WINDOW_START_HOUR = "0";
process.env.SEND_WINDOW_END_HOUR = "24";
process.env.SEND_ON_WEEKENDS = "true";
process.env.MIN_MINUTES_BETWEEN_SENDS = "0";
process.env.DAILY_SEND_CAP = "2";
process.env.BLOCK_FREEMAIL = "true";
process.env.BLOCKED_COUNTRIES = "CA,DE";
process.env.UNSUBSCRIBE_BASE_URL = "http://localhost:8787";

const {
  audiences,
  companies,
  contacts,
  domainEmailPatterns,
  jobs,
  messages,
  projects,
  siteProfiles,
  suppressions,
} = await import("../src/db/repo");
const { db } = await import("../src/db/index");
const { updateSettings } = await import("../src/settings");
const { screenContact, screenSendWindow, preflight } = await import(
  "../src/compliance/guards"
);
const { withFooter, withHtmlFooter, unsubscribeHeaders, newUnsubscribeToken } = await import(
  "../src/compliance/footer"
);
const {
  detectEmailPattern,
  emailPatterns,
  orderedEmailCandidates,
} = await import("../src/providers/emailPatterns");
const { verifyAvailable } = await import("../src/providers/verify");
const { sendMessage } = await import("../src/pipeline/stages/sendMessage");
const { env } = await import("../src/env");
const { contextFromParts, renderTemplate } = await import("../src/pipeline/template");

// Ensure product settings match the test env (also covers re-runs against a warm DB).
updateSettings({
  sendWindowStartHour: 0,
  sendWindowEndHour: 24,
  sendOnWeekends: true,
  minMinutesBetweenSends: 0,
  dailySendCap: 2,
  blockFreemail: true,
  blockedCountries: ["CA", "DE"],
  unsubscribeBaseUrl: "http://localhost:8787",
});

afterAll(() => rmSync(workDir, { recursive: true, force: true }));

let projectId: number;
let audienceId: number;
let companyId: number;

beforeAll(() => {
  const project = projects.create("Observer", "getobserver.app");
  projectId = project.id;

  const profile = siteProfiles.create(projectId, [
    { url: "https://getobserver.app", title: "Observer", markdown: "AI meeting assistant" },
  ]);
  siteProfiles.setAnalysis(profile.id, {
    productName: "Observer",
    oneLiner: "macOS AI meeting assistant with live answers and private transcripts.",
    features: ["live transcript", "screen context"],
    painPointsSolved: ["forgetting what was said in calls"],
    pricing: "Pro $25/mo",
    pricingPlans: [
      {
        name: "Pro",
        price: "$25/mo",
        note: "Billed monthly",
        trial: "14-day free trial",
        highlights: ["Live answers", "Private transcripts"],
      },
    ],
    competitors: [{ name: "Otter", domain: "otter.ai" }],
    platforms: ["macOS"],
    useCases: [
      {
        title: "Live sales calls",
        description: "Surface answers while talking to a prospect",
        audienceHint: "Account executives",
      },
    ],
  });

  audienceId = audiences.create(projectId, {
    name: "Technical recruiters",
    jobTitles: ["Technical Recruiter"],
    searchQueries: ["technical recruiting agencies hiring"],
    painPoints: ["losing candidate detail between screens"],
    valueProp: "Never lose a candidate detail from a screening call.",
    origin: "generated",
  }).id;

  companyId = companies.upsert({
    projectId,
    audienceId,
    name: "Acme Talent",
    domain: "acmetalent.example",
    description: "Boutique technical recruiting firm",
  })!.id;
});

function makeContact(overrides: Partial<Parameters<typeof contacts.upsert>[0]> = {}) {
  return contacts.upsert({
    projectId,
    audienceId,
    companyId,
    fullName: "Jane Doe",
    title: "Head of Talent",
    email: `jane${Math.random().toString(36).slice(2, 8)}@acmetalent.example`,
    emailSource: "pattern",
    evidenceUrl: "https://linkedin.com/in/janedoe",
    ...overrides,
  })!;
}

function makeMessage(contactId: number) {
  return messages.create({
    projectId,
    contactId,
    audienceId,
    subject: "candidate notes after screens",
    body: "Saw Acme Talent runs a lot of first-round screens.\n\nRohit",
    model: "gpt-5-mini",
    personalisation: "Company description mentions technical recruiting.",
    unsubscribeToken: newUnsubscribeToken(),
  });
}

describe("site analysis normalization", () => {
  test("upgrades legacy string competitors and freeform pricing", async () => {
    const { mergeCompetitors, normalizeSiteAnalysis } = await import("../src/db/analysis");
    const analysis = normalizeSiteAnalysis({
      productName: "Observer",
      oneLiner: "Meeting notes",
      features: ["live answers"],
      painPointsSolved: ["forgetting context"],
      platforms: ["macOS"],
      pricing: "Pro $25/mo, Max $50/mo",
      competitors: ["Otter", { name: "Fireflies", domain: "https://www.fireflies.ai/pricing" }],
    });

    expect(analysis.competitors).toEqual([
      { name: "Otter", domain: "" },
      { name: "Fireflies", domain: "fireflies.ai" },
    ]);
    expect(analysis.pricingPlans.length).toBeGreaterThanOrEqual(1);
    expect(analysis.pricingPlans.some((plan) => plan.price.includes("25"))).toBe(true);
    expect(analysis.useCases).toEqual([]);

    const merged = mergeCompetitors(
      [{ name: "Cluely", domain: "" }, { name: "Parakeet", domain: "" }],
      [
        { name: "Cluely", domain: "cluely.com" },
        { name: "Granola", domain: "granola.ai" },
      ],
      "getobserver.app",
    );
    expect(merged.find((item) => item.name === "Cluely")?.domain).toBe("cluely.com");
    expect(merged.some((item) => item.name === "Parakeet")).toBe(true);
    expect(merged.some((item) => item.name === "Granola")).toBe(true);
  });
});

describe("campaign workflows", () => {
  test("keeps existing projects on customer outreach by default", () => {
    const project = projects.create("Default Workflow", "default-workflow.example");
    expect(project.workflow_type).toBe("customer_outreach");
    expect(JSON.parse(project.workflow_data)).toEqual({});
  });

  test("persists investor and job workflow metadata", () => {
    const investor = projects.create(
      "Investor Campaign",
      "investor-campaign.example",
      "plain",
      "investor_outreach",
      { raiseStage: "seed", sectors: ["AI", "productivity"] },
    );
    expect(investor.workflow_type).toBe("investor_outreach");
    expect(investor.email_format).toBe("plain");
    expect(JSON.parse(investor.workflow_data)).toEqual({
      raiseStage: "seed",
      sectors: ["AI", "productivity"],
    });

    const job = projects.create(
      "Job Campaign",
      "job-campaign.local",
      "plain",
      "job_outreach",
      { targetRoles: ["Founding Engineer"], locations: ["Remote"] },
    );
    expect(job.workflow_type).toBe("job_outreach");
    expect(job.email_format).toBe("plain");
    expect(JSON.parse(job.workflow_data)).toEqual({
      targetRoles: ["Founding Engineer"],
      locations: ["Remote"],
    });
  });

  test("persists job-search lane targetRoles separately from contact jobTitles", () => {
    const job = projects.create(
      "Job Lane Roles",
      `job-lane-roles-${Date.now()}.local`,
      "plain",
      "job_outreach",
      { targetRoles: ["Founding Engineer"], locations: ["Remote"] },
    );
    const lane = audiences.create(job.id, {
      name: "Founding eng at Seed AI startups",
      targetRoles: ["Founding Engineer", "Senior Full-Stack Engineer"],
      jobTitles: ["Hiring Manager", "CTO", "Recruiter"],
      industries: ["AI"],
      companySizes: ["1-10", "11-50"],
      searchQueries: ["hiring Founding Engineer AI Seed"],
      origin: "generated",
    });
    expect(lane.targetRoles).toEqual(["Founding Engineer", "Senior Full-Stack Engineer"]);
    expect(lane.jobTitles).toEqual(["Hiring Manager", "CTO", "Recruiter"]);

    const updated = audiences.update(lane.id, {
      targetRoles: ["Staff Engineer"],
      jobTitles: ["Head of Engineering", "Founder"],
    });
    expect(updated?.targetRoles).toEqual(["Staff Engineer"]);
    expect(updated?.jobTitles).toEqual(["Head of Engineering", "Founder"]);
  });

  test("stores empty playbook preferences for AI fill and effective fallbacks", async () => {
    const {
      emptyPreferences,
      effectivePreferences,
      preferencesNeedAiFill,
      mergeAiPreferences,
      searchEnrichment,
    } = await import("../src/workflow/preferences");

    const investorEmpty = emptyPreferences("investor_outreach");
    expect(preferencesNeedAiFill(investorEmpty)).toBe(true);
    const investorFilled = mergeAiPreferences(investorEmpty, {
      investorTypes: ["seed funds", "angels"],
      thesisSignals: ["B2B SaaS seed"],
    });
    expect(investorFilled.kind).toBe("investor_outreach");
    if (investorFilled.kind === "investor_outreach") {
      expect(investorFilled.investorTypes).toEqual(["seed funds", "angels"]);
      expect(investorFilled.thesisSignals).toEqual(["B2B SaaS seed"]);
    }

    const jobEffective = effectivePreferences(emptyPreferences("job_outreach"));
    expect(jobEffective.kind).toBe("job_outreach");
    if (jobEffective.kind === "job_outreach") {
      expect(jobEffective.companySizes.length).toBeGreaterThan(0);
    }

    const customerProject = {
      workflow_type: "customer_outreach" as const,
      workflow_data: JSON.stringify({
        preferences: {
          kind: "customer_outreach",
          targetMarkets: ["US"],
          targetCompanySignals: ["Series A"],
          maxContactsPerCompany: 3,
          crawlMaxPages: 10,
          localizeEmails: true,
        },
      }),
    };
    const enrichment = searchEnrichment(customerProject);
    expect(enrichment.markets).toContain("US");
    expect(enrichment.signals).toContain("Series A");
  });

  test("exports contacts for a playbook scope with campaign columns", () => {
    const investor = projects.create(
      "Export Investor Campaign",
      `export-investor-${Date.now()}.local`,
      "html",
      "investor_outreach",
      { raiseStage: "seed" },
    );
    const audience = audiences.create(investor.id, {
      name: "Export funds",
      searchQueries: ["seed funds"],
      origin: "generated",
    });
    const company = companies.upsert({
      projectId: investor.id,
      audienceId: audience.id,
      name: "Export Capital",
      domain: "exportcapital.example",
    })!;
    contacts.upsert({
      projectId: investor.id,
      audienceId: audience.id,
      companyId: company.id,
      fullName: "Export Partner",
      title: "Partner",
      email: "partner@exportcapital.example",
      emailSource: "scraped",
    });

    const rows = contacts.listForExport({
      workflowType: "investor_outreach",
      query: "export capital",
    });
    expect(rows.length).toBeGreaterThan(0);
    expect(rows[0]?.project_name).toBe("Export Investor Campaign");
    expect(rows[0]?.workflow_type).toBe("investor_outreach");
  });

  test("pages contacts across campaigns with workflow metadata", () => {
    const investor = projects.create(
      "Ledger Investor Campaign",
      `ledger-investor-${Date.now()}.local`,
      "html",
      "investor_outreach",
      { raiseStage: "seed" },
    );
    const investorAudience = audiences.create(investor.id, {
      name: "Seed funds",
      searchQueries: ["seed funds AI"],
      origin: "generated",
    });
    const investorCompany = companies.upsert({
      projectId: investor.id,
      audienceId: investorAudience.id,
      name: "Acme Capital",
      domain: "acmecapital.example",
    })!;
    const investorContact = contacts.upsert({
      projectId: investor.id,
      audienceId: investorAudience.id,
      companyId: investorCompany.id,
      fullName: "Alex Investor",
      title: "Partner",
      email: "alex@acmecapital.example",
      emailSource: "scraped",
    })!;

    const page = contacts.pageAll({ workflowType: "investor_outreach", query: "acme capital" });
    expect(page.contacts.some((contact) => contact.id === investorContact.id)).toBe(true);
    const row = page.contacts.find((contact) => contact.id === investorContact.id)!;
    expect(row.project_name).toBe("Ledger Investor Campaign");
    expect(row.workflow_type).toBe("investor_outreach");
    expect(row.audience_name).toBe("Seed funds");
  });

  test("deleting a campaign cascades campaign-scoped contacts and messages", () => {
    const deletedProject = projects.create(
      "Delete Me Campaign",
      `delete-me-${Date.now()}.local`,
      "html",
      "job_outreach",
      { targetRoles: ["Frontend Engineer"] },
    );
    const deletedAudience = audiences.create(deletedProject.id, {
      name: "Hiring teams",
      searchQueries: ["frontend engineer hiring"],
      origin: "generated",
    });
    const deletedCompany = companies.upsert({
      projectId: deletedProject.id,
      audienceId: deletedAudience.id,
      name: "Delete Co",
      domain: "deleteco.example",
    })!;
    const deletedContact = contacts.upsert({
      projectId: deletedProject.id,
      audienceId: deletedAudience.id,
      companyId: deletedCompany.id,
      fullName: "Casey Hiring",
      title: "CTO",
      email: "casey@deleteco.example",
      emailSource: "manual",
    })!;
    const deletedMessage = messages.create({
      projectId: deletedProject.id,
      contactId: deletedContact.id,
      audienceId: deletedAudience.id,
      subject: "frontend role",
      body: "Hello",
      model: "test",
      personalisation: "test",
      unsubscribeToken: newUnsubscribeToken(),
    });

    expect(projects.remove(deletedProject.id)).toBe(true);
    expect(projects.get(deletedProject.id)).toBeNull();
    expect(contacts.get(deletedContact.id)).toBeNull();
    expect(messages.get(deletedMessage.id)).toBeNull();
    expect(contacts.pageAll({ projectId: deletedProject.id }).total).toBe(0);
  });
});

describe("email pattern generation", () => {
  test("uses the fixed default probe order preferring first.last over first@", () => {
    expect(emailPatterns("Rohit Malhotra", "acme.com")).toEqual([
      "rohit.malhotra@acme.com",
      "rmalhotra@acme.com",
      "rohitmalhotra@acme.com",
      "rohit.m@acme.com",
      "rohit@acme.com",
    ]);
  });

  test("probes ranked learned formats before the default order without duplicates", () => {
    expect(
      orderedEmailCandidates("Rohit Malhotra", "acme.com", ["flast", "first.last"]),
    ).toEqual([
      "rmalhotra@acme.com",
      "rohit.malhotra@acme.com",
      "rohitmalhotra@acme.com",
      "rohit.m@acme.com",
      "rohit@acme.com",
    ]);
  });

  test("detects which pattern produced an address", () => {
    expect(detectEmailPattern("rohit.m@acme.com", "Rohit Malhotra", "acme.com")).toBe(
      "first.lastInitial",
    );
    expect(detectEmailPattern("rmalhotra@acme.com", "Rohit Malhotra")).toBe("flast");
  });

  test("handles single names and accented characters", () => {
    expect(emailPatterns("Cher", "acme.com")).toEqual(["cher@acme.com"]);
    expect(emailPatterns("Renée Müller", "acme.com")[0]).toBe("renee.muller@acme.com");
  });

  test("returns nothing without a usable name", () => {
    expect(emailPatterns("", "acme.com")).toEqual([]);
  });
});

describe("domain email pattern memory", () => {
  test("ranks learned patterns by hit_count for fallback probing", () => {
    domainEmailPatterns.recordHit("learned.example", "first");
    domainEmailPatterns.recordHit("learned.example", "first.last");
    domainEmailPatterns.recordHit("learned.example", "first.last");
    domainEmailPatterns.recordHit("learned.example", "first.last");
    domainEmailPatterns.recordHit("learned.example", "flast");
    domainEmailPatterns.recordHit("learned.example", "flast");
    domainEmailPatterns.recordHit("learned.example", "flast");
    domainEmailPatterns.recordHit("learned.example", "flast");
    // flast=4, first.last=3, first=1 → top 2 are flast then first.last
    expect(domainEmailPatterns.ranked("learned.example", 2)).toEqual([
      "flast",
      "first.last",
    ]);
    expect(domainEmailPatterns.listForDomain("LEARNED.EXAMPLE")[0]?.hit_count).toBe(4);
  });
});

describe("verification providers", () => {
  test("recognizes reoon when an API key is configured", () => {
    const previousReoon = env.verify.reoonKey;
    env.verify.reoonKey = "";
    try {
      expect(verifyAvailable()).toBe(false);
      env.verify.reoonKey = "test";
      expect(verifyAvailable()).toBe(true);
    } finally {
      env.verify.reoonKey = previousReoon;
    }
  });

  test("maps Reoon power-mode statuses (safe ≠ unknown)", async () => {
    const { mapReoon } = await import("../src/providers/verify");
    expect(mapReoon("safe")).toBe("valid");
    expect(mapReoon("valid")).toBe("valid");
    expect(mapReoon("role")).toBe("risky");
    expect(mapReoon("role_account")).toBe("risky");
    expect(mapReoon("catch_all")).toBe("risky");
    expect(mapReoon("invalid")).toBe("invalid");
    expect(mapReoon("unknown")).toBe("unknown");
    expect(mapReoon("weird_new_status", { is_safe_to_send: true })).toBe("valid");
  });
});

describe("audience templates", () => {
  test("renders category template merge fields deterministically", () => {
    const context = contextFromParts({
      fullName: "Jane Doe",
      title: "Head of Talent",
      company: "Acme Talent",
      companyDomain: "acmetalent.example",
      companyDescription: "Boutique recruiting firm",
      painPoint: "losing candidate details",
      valueProp: "keeps live notes during calls",
    });

    expect(renderTemplate("{{company}} notes", context)).toBe("Acme Talent notes");
    expect(renderTemplate("Hi {{firstName}}, {{valueProp}}.", context)).toBe(
      "Hi Jane, keeps live notes during calls.",
    );
  });
});

describe("contact screening", () => {
  test("accepts a normal business address", () => {
    expect(screenContact(makeContact()).ok).toBe(true);
  });

  test("rejects consumer mailboxes", () => {
    const verdict = screenContact(makeContact({ email: "jane.doe@gmail.com" }));
    expect(verdict.ok).toBe(false);
    expect(verdict.ok === false && verdict.reason).toContain("consumer mailbox");
  });

  test("rejects contacts in blocked countries", () => {
    const verdict = screenContact(makeContact({ country: "DE" }));
    expect(verdict.ok).toBe(false);
    expect(verdict.ok === false && verdict.reason).toContain("blocked countries");
  });

  test("rejects addresses on the sending domain", () => {
    const verdict = screenContact(makeContact({ email: "someone@tryobserver.app" }));
    expect(verdict.ok).toBe(false);
  });

  test("allows freemail recipients when sender is also freemail (manual)", () => {
    const previousProvider = env.sender.provider;
    const previousGmailFrom = env.sender.gmail.fromEmail;
    const previousSharedFrom = env.sender.fromEmail;
    env.sender.provider = "gmail";
    env.sender.gmail.fromEmail = "me@gmail.com";
    env.sender.fromEmail = "me@gmail.com";
    try {
      const verdict = screenContact(
        makeContact({ email: "jane.doe@gmail.com", emailSource: "manual" }),
      );
      expect(verdict.ok).toBe(true);
    } finally {
      env.sender.provider = previousProvider;
      env.sender.gmail.fromEmail = previousGmailFrom;
      env.sender.fromEmail = previousSharedFrom;
    }
  });

  test("rejects suppressed addresses", () => {
    const contact = makeContact();
    suppressions.add(contact.email, null, "unsubscribed", projectId);
    expect(screenContact(contact).ok).toBe(false);
  });

  test("rejects addresses verification called invalid", () => {
    const contact = makeContact();
    contacts.setVerification(contact.id, "invalid", null);
    expect(screenContact(contacts.get(contact.id)!).ok).toBe(false);
  });

  test("rejects role and generic inboxes without needing verification", () => {
    for (const email of [
      "support@acmetalent.example",
      "hello@acmetalent.example",
      "media@acmetalent.example",
      "press@acmetalent.example",
      "contact@acmetalent.example",
      "careers@acmetalent.example",
      "jobs@acmetalent.example",
      "business@acmetalent.example",
      "associate@acmetalent.example",
      "market@acmetalent.example",
      "account@acmetalent.example",
      "sales.team@acmetalent.example",
      "sales.representative@acmetalent.example",
      "work@acmetalent.example",
      "working@acmetalent.example",
      "acmetalent@acmetalent.example",
      "head@acmetalent.example",
      "enterprise@acmetalent.example",
    ]) {
      const verdict = screenContact(makeContact({ email }));
      expect(verdict.ok).toBe(false);
      expect(verdict.ok === false && verdict.reason).toContain("role/generic");
    }
  });

  test("keeps personal-looking addresses", async () => {
    const { isRoleOrGenericEmail, looksLikePersonalEmail } = await import(
      "../src/compliance/guards"
    );
    expect(isRoleOrGenericEmail("jane.doe@acmetalent.example")).toBe(false);
    expect(looksLikePersonalEmail("jane.doe@acmetalent.example")).toBe(true);
    expect(looksLikePersonalEmail("sales.team@acmetalent.example")).toBe(false);
    expect(isRoleOrGenericEmail("innovaccer@innovaccer.com")).toBe(true);
    expect(isRoleOrGenericEmail("aztec@aztec.network")).toBe(true);
  });
});

describe("email body spacing", () => {
  test("splits a one-line cold email into paragraphs", async () => {
    const { normalizeEmailBody } = await import("../src/compliance/emailBody");
    const normalized = normalizeEmailBody(
      "Hi Alex, Saw Northstar runs weekly investor updates. A few founders keep private notes on their laptop for those calls. Observer does that without a meeting bot. Worth a look? Rohit",
    );
    expect(normalized).toContain("\n\n");
    expect(normalized.split("\n\n").length).toBeGreaterThanOrEqual(3);
  });

  test("preserves single newlines in a signature stack", async () => {
    const { normalizeEmailBody } = await import("../src/compliance/emailBody");
    const normalized = normalizeEmailBody(
      "Hi {{firstName}},\n\nObserver is a sales tool.\n\nThanks,\nRohit Malhotra\nFounder, Observer\n\nhttps://getobserver.app",
    );
    expect(normalized).toContain("Thanks,\nRohit Malhotra\nFounder, Observer");
    expect(normalized).not.toContain("Thanks, Rohit Malhotra");
  });
});

describe("footer and headers", () => {
  test("sends body as-is with no opt-out or postal footer", () => {
    const body = withFooter("Hello there.\n\nRohit", "token123");
    expect(body).toBe("Hello there.\n\nRohit");
    expect(body).not.toContain("/u/");
    expect(body).not.toContain("Just reply and say so");
    expect(body).not.toContain("Bengaluru");
  });

  test("does not set List-Unsubscribe headers", () => {
    expect(unsubscribeHeaders("token123")).toEqual({});
  });

  test("renders legacy HTML without opt-out or postal footer", () => {
    const html = withHtmlFooter("Hello there.\n\nRohit", "token123");
    expect(html).toContain("<html>");
    expect(html).toContain("Observer");
    expect(html).not.toContain("/u/");
    expect(html).not.toContain("Just reply and say so");
    expect(html).not.toContain("Bengaluru");
  });
});

describe("send stage", () => {
  test("refuses to send anything that is not approved", async () => {
    const message = makeMessage(makeContact().id);
    const result = await sendMessage(projectId, { messageId: message.id });
    expect(result).toContain("not approved");
    expect(messages.get(message.id)!.status).toBe("draft");
  });

  test("sends an approved message and records the provider id", async () => {
    const message = makeMessage(makeContact().id);
    messages.setStatus(message.id, "approved", { approved_at: new Date().toISOString() });

    const result = await sendMessage(projectId, { messageId: message.id });
    expect(result).toContain("via dry_run");

    const sent = messages.get(message.id)!;
    expect(sent.status).toBe("sent");
    expect(sent.provider_message_id).toStartWith("dry-");
    expect(sent.sent_at).toBeTruthy();
    expect(sent.email_format).toBe("plain");
    expect(sent.sent_text_body).toContain("Saw Acme Talent");
    expect(sent.sent_text_body).not.toContain("Not relevant?");
    expect(sent.sent_html_body).toBe("");
  });

  test("skips a recipient who unsubscribed after the draft was written", async () => {
    const contact = makeContact();
    const message = makeMessage(contact.id);
    messages.setStatus(message.id, "approved", { approved_at: new Date().toISOString() });
    suppressions.add(contact.email, null, "unsubscribed", projectId);

    const result = await sendMessage(projectId, { messageId: message.id });
    expect(result).toContain("suppression list");
    expect(messages.get(message.id)!.status).toBe("skipped");
  });

  test("defers once the daily cap is reached instead of failing", async () => {
    // The cap is 2 and one message has already gone out above.
    const first = makeMessage(makeContact().id);
    messages.setStatus(first.id, "approved", { approved_at: new Date().toISOString() });
    await sendMessage(projectId, { messageId: first.id });
    expect(messages.sentInLastDay(projectId)).toBe(2);

    const blocked = makeMessage(makeContact().id);
    messages.setStatus(blocked.id, "approved", { approved_at: new Date().toISOString() });
    await expect(sendMessage(projectId, { messageId: blocked.id })).rejects.toThrow(
      /daily cap reached/,
    );
    expect(messages.get(blocked.id)!.status).toBe("approved");
  });

  test("send window guard reports the cap as the reason", () => {
    const verdict = screenSendWindow(projectId);
    expect(verdict.ok).toBe(false);
  });
});

describe("draft stage", () => {
  test("can explicitly create another draft for an existing contact", async () => {
    const { draftMessage } = await import("../src/pipeline/stages/draftMessage");
    const audience = audiences.create(projectId, {
      name: "Follow-up template audience",
      templateSubject: "{{company}} follow-up",
      templateBody: "Hi {{firstName}},\n\n{{observation}}\n\nWorth another look?\n\nRohit",
      valueProp: "keeps teams aligned after calls",
      origin: "manual",
    });
    const company = companies.ensure({
      projectId,
      audienceId: audience.id,
      name: "Follow Co",
      domain: "follow.example",
      description: "Hiring for customer-facing teams",
    });
    const contact = contacts.upsert({
      projectId,
      audienceId: audience.id,
      companyId: company.id,
      fullName: "Alex Follow",
      title: "Founder",
      email: "alex@follow.example",
      emailSource: "manual",
      evidenceUrl: "manual",
    })!;
    contacts.setVerification(contact.id, "valid", null);
    makeMessage(contact.id);

    const skipped = await draftMessage(projectId, { contactId: contact.id });
    expect(skipped).toContain("already has a message");

    const created = await draftMessage(projectId, { contactId: contact.id, forceNew: true });
    expect(created).toContain("Rendered template message");

    const sentContact = contacts.upsert({
      projectId,
      audienceId: audience.id,
      companyId: company.id,
      fullName: "Sam Sent",
      title: "CEO",
      email: "sam@follow.example",
      emailSource: "manual",
      evidenceUrl: "manual",
    })!;
    contacts.setVerification(sentContact.id, "valid", null);
    const sentMessage = makeMessage(sentContact.id);
    messages.setStatus(sentMessage.id, "sent", {
      sent_at: new Date().toISOString().replace("T", " ").slice(0, 19),
      provider_message_id: "test-sent",
    });
    const noRedraft = await draftMessage(projectId, {
      contactId: sentContact.id,
      forceNew: true,
    });
    expect(noRedraft).toContain("already sent");
  });

  test("purges leftover drafts once a contact has been sent", () => {
    const contact = makeContact();
    const draft = makeMessage(contact.id);
    const sent = makeMessage(contact.id);
    messages.setStatus(sent.id, "sent", {
      sent_at: new Date().toISOString().replace("T", " ").slice(0, 19),
      provider_message_id: "purge-test",
    });
    expect(messages.get(draft.id)?.status).toBe("draft");
    const removed = messages.removeUnsentWhereAlreadySent(projectId);
    expect(removed).toBeGreaterThanOrEqual(1);
    expect(messages.get(draft.id)).toBeNull();
    expect(messages.get(sent.id)?.status).toBe("sent");
  });
});

describe("preflight", () => {
  test("passes when live sender basics are configured", () => {
    expect(preflight()).toEqual([]);
  });

  test("requires sender credentials when live sending is enabled", () => {
    const previousProvider = env.sender.provider;
    const previousBrevoKey = env.sender.brevo.apiKey;
    const previousGmailPass = env.sender.gmail.appPassword;
    env.sender.provider = "brevo";
    env.sender.brevo.apiKey = "";
    try {
      expect(preflight().join(" ")).toContain("BREVO_API_KEY");
    } finally {
      env.sender.provider = previousProvider;
      env.sender.brevo.apiKey = previousBrevoKey;
    }

    env.sender.provider = "gmail";
    env.sender.gmail.appPassword = "";
    try {
      expect(preflight().join(" ")).toContain("GMAIL_APP_PASSWORD");
    } finally {
      env.sender.provider = previousProvider;
      env.sender.gmail.appPassword = previousGmailPass;
    }
  });
});

describe("granular job retry", () => {
  test("retries only selected failed jobs, not the whole pipeline", () => {
    db.run(`DELETE FROM jobs WHERE project_id = ?`, [projectId]);

    jobs.enqueue(projectId, "find_contacts", { companyId });
    jobs.enqueue(projectId, "find_contacts", { companyId: companyId + 999_001 });
    jobs.enqueue(projectId, "scan_site");

    const queued = jobs.listByStatuses(projectId, ["pending"], 20);
    expect(queued.length).toBe(3);

    for (const job of queued) {
      db.run(
        `UPDATE jobs SET status = 'failed', attempts = 3, last_error = 'boom', updated_at = datetime('now')
         WHERE id = ?`,
        [job.id],
      );
    }

    const failed = jobs.listByStatuses(projectId, ["failed"], 20);
    const contactFail = failed.find((job) => job.stage === "find_contacts")!;
    const scanFail = failed.find((job) => job.stage === "scan_site")!;

    expect(jobs.retryFailed(projectId, { jobIds: [contactFail.id] })).toBe(1);

    expect(jobs.listByStatuses(projectId, ["pending"], 20).map((job) => job.id)).toEqual([
      contactFail.id,
    ]);
    expect(jobs.listByStatuses(projectId, ["failed"], 20).map((job) => job.id).sort()).toEqual(
      failed.filter((job) => job.id !== contactFail.id).map((job) => job.id).sort(),
    );

    expect(jobs.retryFailed(projectId, { stage: "scan_site" })).toBe(1);
    expect(
      jobs.listByStatuses(projectId, ["pending"], 20).some((job) => job.id === scanFail.id),
    ).toBe(true);
  });

  test("ensure requeues a failed job instead of inserting a duplicate", () => {
    db.run(`DELETE FROM jobs WHERE project_id = ?`, [projectId]);
    jobs.enqueue(projectId, "verify_contact", { contactId: 42 });
    const job = jobs.listByStatuses(projectId, ["pending"], 5)[0]!;
    db.run(
      `UPDATE jobs SET status = 'failed', attempts = 3, last_error = 'out of credits'
       WHERE id = ?`,
      [job.id],
    );

    jobs.ensure(projectId, "verify_contact", { contactId: 42 });
    const pending = jobs.listByStatuses(projectId, ["pending"], 10);
    const failed = jobs.listByStatuses(projectId, ["failed"], 10);
    expect(pending.map((row) => row.id)).toEqual([job.id]);
    expect(failed).toEqual([]);
  });

  test("dismissFailed hides failures from attention counts but keeps the job row", () => {
    db.run(`DELETE FROM jobs WHERE project_id = ?`, [projectId]);
    jobs.enqueue(projectId, "verify_contact", { contactId: 99 });
    const job = jobs.listByStatuses(projectId, ["pending"], 5)[0]!;
    db.run(
      `UPDATE jobs SET status = 'failed', attempts = 3, last_error = 'credits low'
       WHERE id = ?`,
      [job.id],
    );

    expect(jobs.failedCount(projectId)).toBe(1);
    expect(jobs.listByStatuses(projectId, ["failed"], 10).map((row) => row.id)).toEqual([job.id]);

    expect(jobs.dismissFailed(projectId, { jobIds: [job.id] })).toBe(1);
    expect(jobs.failedCount(projectId)).toBe(0);
    expect(jobs.listByStatuses(projectId, ["failed"], 10)).toEqual([]);

    const row = db
      .query<{ status: string; dismissed_at: string | null }, [number]>(
        `SELECT status, dismissed_at FROM jobs WHERE id = ?`,
      )
      .get(job.id)!;
    expect(row.status).toBe("failed");
    expect(row.dismissed_at).toBeTruthy();
  });
});

describe("find_contacts resume", () => {
  test("skips rediscovery when contacts already exist for the company", async () => {
    const { findContacts } = await import("../src/pipeline/stages/findContacts");
    makeContact({ email: "resume.person@acmetalent.example", fullName: "Resume Person" });

    const before = jobs.listByStatuses(projectId, ["pending", "failed", "done"], 50).length;
    const result = await findContacts(projectId, { companyId });
    expect(result).toContain("without re-searching");
    expect(result).toContain("Resumed");

    const verifyJobs = jobs
      .listByStatuses(projectId, ["pending"], 50)
      .filter((job) => job.stage === "verify_contact");
    expect(verifyJobs.length).toBeGreaterThan(0);
    expect(jobs.listByStatuses(projectId, ["pending", "failed", "done"], 50).length).toBeGreaterThanOrEqual(
      before,
    );
  });
});

describe("audience regenerate safety", () => {
  test("replaceGenerated keeps audiences that already have contacts/messages", () => {
    const empty = audiences.create(projectId, {
      name: "Empty generated ICP",
      origin: "generated",
      searchQueries: ["empty icp query"],
    });
    const contact = makeContact({ email: "keep-me@acmetalent.example" });
    const message = makeMessage(contact.id);

    audiences.replaceGenerated(projectId, { preserveProspecting: true });

    expect(audiences.get(empty.id)).toBeNull();
    const kept = audiences.get(audienceId);
    expect(kept).not.toBeNull();
    expect(kept!.origin).toBe("retained");
    expect(kept!.enabled).toBe(false);
    expect(contacts.get(contact.id)?.email).toBe("keep-me@acmetalent.example");
    expect(messages.get(message.id)?.subject).toBe("candidate notes after screens");
  });

  test("replaceGenerated can wipe generated audiences when preserve is off", () => {
    const doomed = audiences.create(projectId, {
      name: "Doomed ICP",
      origin: "generated",
      searchQueries: ["doomed"],
    });
    const company = companies.upsert({
      projectId,
      audienceId: doomed.id,
      name: "Doomed Co",
      domain: "doomed.example",
    })!;
    const contact = contacts.upsert({
      projectId,
      audienceId: doomed.id,
      companyId: company.id,
      fullName: "Gone Person",
      title: "CEO",
      email: "gone@doomed.example",
      emailSource: "pattern",
    })!;

    audiences.replaceGenerated(projectId, { preserveProspecting: false });

    expect(audiences.get(doomed.id)).toBeNull();
    expect(companies.get(company.id)).toBeNull();
    expect(contacts.get(contact.id)).toBeNull();
  });
});

describe("company search query construction", () => {
  test("appends exactly one enrichment constraint and never stacks -site operators", async () => {
    const { buildCompanySearchQuery } = await import("../src/providers/companySearchQuery");
    const query = buildCompanySearchQuery({
      baseQuery: "AI meeting assistant startups",
      markets: ["US", "UK"],
      signals: ["YC", "Series A", "venture-backed"],
      industries: ["developer tools", "productivity"],
      companySizes: ["11-50"],
      workflowType: "customer_outreach",
      page: 1,
      queryIndex: 0,
    });

    expect(query).toContain("AI meeting assistant startups");
    // Free Serper rejects multi -site patterns — filter hosts after search instead.
    expect(query).not.toContain("-site:");
    const enrichmentHits = [
      query.includes('"YC"') || query.includes("YC"),
      /\bUS\b/.test(query),
      query.includes('"developer tools"') || query.includes("developer tools"),
    ].filter(Boolean).length;
    expect(enrichmentHits).toBe(1);
    expect(query).not.toMatch(/"YC" OR "Series A"/);
  });

  test("rotates which enrichment kind is applied across query indexes", async () => {
    const { buildCompanySearchQuery } = await import("../src/providers/companySearchQuery");
    const a = buildCompanySearchQuery({
      baseQuery: "sales teams",
      markets: ["US"],
      signals: ["YC", "Series A"],
      industries: ["SaaS"],
      companySizes: [],
      workflowType: "customer_outreach",
      page: 1,
      queryIndex: 0,
    });
    const b = buildCompanySearchQuery({
      baseQuery: "sales teams",
      markets: ["US"],
      signals: ["YC", "Series A"],
      industries: ["SaaS"],
      companySizes: [],
      workflowType: "customer_outreach",
      page: 1,
      queryIndex: 1,
    });
    expect(a).not.toBe(b);
    expect(a).toContain("sales teams");
    expect(b).toContain("sales teams");
  });

  test("drops prose preference blobs and skips enrichment when base already has market+signal", async () => {
    const { buildCompanySearchQuery } = await import("../src/providers/companySearchQuery");
    const query = buildCompanySearchQuery({
      baseQuery: "Seed startup hiring engineers YC portfolio United States",
      markets: ["Priority 1\nUnited States", "Canada"],
      signals: [
        "Recently funded startups (last 6 months).\n\nPre-Seed",
        "Series A",
      ],
      industries: ["B2B SaaS"],
      companySizes: [],
      workflowType: "customer_outreach",
      page: 1,
      queryIndex: 0,
    });
    expect(query).not.toMatch(/Priority/i);
    expect(query).not.toMatch(/last 6 months/i);
    expect(query.match(/United States/gi)?.length ?? 0).toBe(1);
    expect(query).not.toContain('"B2B SaaS"');
    expect(query).not.toContain('"Series A"');
  });
});

describe("customer prospect query planner", () => {
  test("mixes structured prefs, hiring titles, and competitor lookalikes", async () => {
    const { planCustomerSearchQueries } = await import("../src/providers/customerProspectQueries");
    const plan = planCustomerSearchQueries({
      audienceName: "Recruiting & talent teams",
      jobTitles: ["Recruiter", "Head of Talent", "HR Manager"],
      industries: ["B2B SaaS", "Developer Tools"],
      companySizes: ["11-50", "51-200"],
      searchQueries: ["Series A B2B SaaS startups hiring recruiters"],
      markets: ["United States", "United Kingdom", "Germany"],
      signals: ["YC", "Seed", "Techstars", "Series A"],
      competitors: [
        { name: "Otter", domain: "otter.ai" },
        { name: "Fireflies", domain: "fireflies.ai" },
      ],
      page: 1,
      maxQueries: 8,
    });

    expect(plan.queries.length).toBeGreaterThanOrEqual(5);
    expect(plan.intents).toContain("structured_prefs");
    expect(plan.intents).toContain("hiring_title");
    expect(plan.intents).toContain("competitor_lookalike");
    expect(plan.queries.some((q) => /United States|United Kingdom|Germany/i.test(q))).toBe(true);
    expect(plan.queries.some((q) => /YC|Seed|Techstars|Series A/i.test(q))).toBe(true);
    expect(plan.queries.some((q) => /Recruiter|Head of Talent|HR Manager/i.test(q))).toBe(true);
    expect(plan.queries.some((q) => /Otter|Fireflies/i.test(q))).toBe(true);
    expect(plan.queries.every((q) => !q.includes("-site:"))).toBe(true);
  });

  test("blocks competitor domains from prospecting", async () => {
    const { blockedProspectDomains } = await import("../src/providers/customerProspectQueries");
    const blocked = blockedProspectDomains({
      ownDomains: ["getobserver.app"],
      competitors: [
        { name: "Otter", domain: "otter.ai" },
        { name: "Cluely", domain: "" },
      ],
    });
    expect(blocked).toContain("getobserver.app");
    expect(blocked).toContain("otter.ai");
    expect(blocked).not.toContain("");
  });
});

describe("job prospect query planner", () => {
  test("searches for candidate target roles, not hiring-contact titles", async () => {
    const { planJobSearchQueries } = await import("../src/providers/jobProspectQueries");
    const plan = planJobSearchQueries({
      audienceName: "Founding eng at Seed AI startups",
      targetRoles: ["Founding Engineer", "Senior Full-Stack Engineer"],
      industries: ["AI", "B2B SaaS"],
      companySizes: ["1-10", "11-50"],
      searchQueries: ["Seed AI startups hiring founding engineers"],
      markets: ["Remote", "United States"],
      signals: ["AI hiring", "we're hiring"],
      queryHints: ["funded 2026", "AI startup"],
      page: 1,
      maxQueries: 8,
    });

    expect(plan.queries.length).toBeGreaterThanOrEqual(5);
    expect(plan.intents).toContain("open_role");
    expect(plan.intents).toContain("careers_page");
    expect(plan.intents).toContain("setup_brief");
    expect(plan.queries.some((q) => /Founding Engineer|Full-Stack/i.test(q))).toBe(true);
    // Contact titles must not drive employer search.
    expect(plan.queries.every((q) => !/\bRecruiter\b|\bHiring Manager\b/i.test(q))).toBe(true);
    expect(plan.queries.some((q) => /hiring|careers|job|Wellfound|YC jobs|LinkedIn Jobs/i.test(q))).toBe(
      true,
    );
    expect(plan.queries.some((q) => /funded 2026|AI startup/i.test(q))).toBe(true);
    expect(plan.queries.every((q) => !q.includes("-site:"))).toBe(true);
  });
});

describe("job search brief", () => {
  test("normalizes setup form constraints from workflow_data", async () => {
    const {
      jobSearchBriefFromProject,
      normalizeCompanySizes,
      normalizeJobIndustries,
    } = await import("../src/workflow/jobBrief");

    expect(normalizeCompanySizes(["0-50"])).toEqual(["1-10", "11-50"]);
    expect(normalizeJobIndustries(["Any Industry with a strong preference for an AI company"])).toEqual({
      industries: ["AI"],
      preferAi: true,
    });

    const brief = jobSearchBriefFromProject({
      workflow_type: "job_outreach",
      workflow_data: JSON.stringify({
        targetRoles: ["Senior Frontend Engineer", "Founding Engineer"],
        locations: ["India", "Europe", "Germany", "but EUROPEAN COMPANY is most preferred"],
        remotePreference: "Full Remote or remote from India",
        seniority: "Senior/Lead level",
        notes: "Pays in USD. Small team. Should be fuded in 2026",
        preferences: {
          kind: "job_outreach",
          companySizes: ["0-50"],
          industries: ["Any Industry with a strong preference for an AI company"],
        },
      }),
    });

    expect(brief).not.toBeNull();
    expect(brief!.companySizes).toEqual(["1-10", "11-50"]);
    expect(brief!.industries).toContain("AI");
    expect(brief!.preferAi).toBe(true);
    expect(brief!.fundedAfterYear).toBe(2026);
    expect(brief!.markets[0]).toMatch(/Europe|European/i);
    expect(brief!.queryHints.some((h) => /funded 2026/i.test(h))).toBe(true);
    expect(brief!.hardConstraints.some((c) => /European/i.test(c))).toBe(true);
  });
});

describe("competitor link extraction", () => {
  test("pulls competitor domains from compare-page markdown links", async () => {
    const { extractCompetitorLinksFromPages } = await import("../src/pipeline/stages/scanSite");
    const competitors = extractCompetitorLinksFromPages(
      [
        {
          url: "https://getobserver.app/compare",
          title: "Compare",
          markdown:
            "See how we stack up against [Otter](https://otter.ai) and [Fireflies](https://www.fireflies.ai/pricing).",
        },
      ],
      "getobserver.app",
    );
    expect(competitors.some((c) => c.name === "Otter" && c.domain === "otter.ai")).toBe(true);
    expect(competitors.some((c) => c.name === "Fireflies" && c.domain === "fireflies.ai")).toBe(
      true,
    );
  });
});

describe("serper query safety", () => {
  test("simplifies blocked patterns by stripping site operators and ORs", async () => {
    const { simplifySearchQuery, isSerperPatternBlocked } = await import(
      "../src/providers/searchQuerySafe"
    );
    const simplified = simplifySearchQuery(
      `"Observer" alternatives OR competitors -site:getobserver.app -site:linkedin.com`,
    );
    expect(simplified).not.toMatch(/-site:/i);
    expect(simplified).not.toMatch(/\bOR\b/);
    expect(isSerperPatternBlocked(new Error('400 Query pattern not allowed for free accounts'))).toBe(
      true,
    );
  });

  test("keeps allow-listed hosts when filtering social noise", async () => {
    const { filterSearchResults } = await import("../src/providers/searchQuerySafe");
    const results = [
      { link: "https://www.linkedin.com/in/jane-doe", title: "Jane" },
      { link: "https://au.linkedin.com/in/jane-doe", title: "Jane AU" },
      { link: "https://twitter.com/jane", title: "Twitter" },
      { link: "https://canva.com/about", title: "Canva" },
    ];
    const filtered = filterSearchResults(results, { allowHosts: ["linkedin.com"] });
    expect(filtered.map((r) => r.link)).toEqual([
      "https://www.linkedin.com/in/jane-doe",
      "https://au.linkedin.com/in/jane-doe",
      "https://canva.com/about",
    ]);
  });

  test("people search queries avoid site:, OR, and quoted phrases Serper free tier blocks", async () => {
    const { buildPeopleSearchQuery, parsePersonFromSerp, serpMentionsCompany } = await import(
      "../src/providers/contacts"
    );
    const query = buildPeopleSearchQuery("Surge Ventures", "Head of Talent");
    expect(query).toBe("Surge Ventures Head of Talent linkedin");
    expect(query).not.toMatch(/site:/i);
    expect(query).not.toMatch(/\bOR\b/);
    expect(query).not.toMatch(/"/);

    const parsed = parsePersonFromSerp("Chris Harris - Recruiting @ Canva | LinkedIn");
    expect(parsed).toEqual({ fullName: "Chris Harris", title: "Recruiting @ Canva" });

    expect(
      serpMentionsCompany("Canva", "canva.com", "Chris Harris - Recruiting @ Canva", ""),
    ).toBe(true);
    expect(
      serpMentionsCompany(
        "Canva",
        "canva.com",
        "Olivia MacDowell - Recruiting @ Runway",
        "Results related to Canva recruiting",
      ),
    ).toBe(false);
  });
});

describe("company source adapters", () => {
  test("selects YC and matching accelerators from signals without false EF matches", async () => {
    const { selectCompanySources } = await import("../src/providers/sources");
    const { matchesAnySignal } = await import("../src/providers/sources/types");

    expect(matchesAnySignal(["Pre-Seed", "Seed"], ["ef"])).toBe(false);
    expect(matchesAnySignal(["EF", "Seed"], ["ef"])).toBe(true);
    expect(matchesAnySignal(["YC", "Techstars"], ["yc"])).toBe(true);

    const selected = selectCompanySources([
      "YC",
      "Techstars",
      "500 Global",
      "Antler",
      "EF",
      "Surge",
      "Fast-growing AI",
    ]);
    const ids = selected.map((s) => s.id);
    expect(ids).toContain("yc");
    expect(ids).toContain("techstars");
    expect(ids).toContain("500global");
    expect(ids).toContain("antler");
    expect(ids).toContain("ef");
    expect(ids).toContain("surge");
    expect(ids).toContain("producthunt");
  });

  test("audience sourceAdapters override signal-based selection", async () => {
    const { selectCompanySources } = await import("../src/providers/sources");
    const onlyYc = selectCompanySources(["Techstars", "Antler"], ["yc", "web"]);
    expect(onlyYc.map((s) => s.id)).toEqual(["yc"]);

    const multi = selectCompanySources([], ["techstars", "ef", "web"]);
    expect(multi.map((s) => s.id)).toEqual(["techstars", "ef"]);
  });

  test("company size and funded-after filters match startup buckets", async () => {
    const {
      matchesCompanySizes,
      matchesFundedAfter,
      yearFromBatch,
      sizeHintFromTeamSize,
    } = await import("../src/providers/companyFilters");

    expect(sizeHintFromTeamSize(25)).toBe("11-50");
    expect(matchesCompanySizes("11-50", 25, ["1-10", "11-50"])).toBe(true);
    expect(matchesCompanySizes("201-1000", 400, ["1-10", "11-50"])).toBe(false);
    expect(matchesCompanySizes("", undefined, ["11-50"])).toBe(true);
    expect(yearFromBatch("W24")).toBe(2024);
    expect(matchesFundedAfter(2016, 2018)).toBe(false);
    expect(matchesFundedAfter(2022, 2018)).toBe(true);
    expect(matchesFundedAfter(0, 2018)).toBe(true);
  });

  test("YC adapter returns real companies with domains from the public directory", async () => {
    const { ycSource } = await import("../src/providers/sources/yc");
    try {
      const companies = await ycSource.fetch({
        projectId: 1,
        audienceName: "Recruiting & talent teams",
        industries: ["B2B SaaS", "Recruiting"],
        jobTitles: ["Recruiter", "Head of Talent"],
        markets: ["United States", "United Kingdom"],
        signals: ["YC", "Seed"],
        page: 1,
        limit: 8,
        excludeDomains: ["getobserver.app"],
      });
      expect(companies.length).toBeGreaterThan(0);
      expect(companies.every((c) => c.domain && c.name && c.source === "yc")).toBe(true);
      expect(companies.every((c) => !c.domain.includes("/"))).toBe(true);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      // Sandbox / offline CI may block yc-oss.github.io.
      if (/403|ENOTFOUND|network|fetch/i.test(message)) return;
      throw error;
    }
  }, 60_000);
});

describe("preference list parsing", () => {
  test("flattens multiline array items and strips Priority labels", async () => {
    const { stringList, sanitizeSearchTerms } = await import("../src/workflow/preferences");
    const markets = stringList([
      "Priority 1\nUnited States",
      "Canada\n\nPriority 2\nUnited Kingdom",
    ]);
    expect(markets).toContain("United States");
    expect(markets).toContain("Canada");
    expect(markets).toContain("United Kingdom");
    expect(markets.some((item) => /priority/i.test(item))).toBe(false);

    const signals = sanitizeSearchTerms(
      stringList([
        "Recently funded startups (last 6 months).\n\nPre-Seed",
        "Seed",
        "Series A",
        "YC",
      ]),
    );
    expect(signals).toContain("Seed");
    expect(signals).toContain("Series A");
    expect(signals).toContain("YC");
    expect(signals.some((item) => /last 6 months/i.test(item))).toBe(false);
  });
});

describe("verify gate vs search", () => {
  test("quota exhaustion pauses verify but is independent of search page advance", async () => {
    const { markVerifyQuotaExhausted, verificationGate, isVerifyQuotaExhausted } = await import(
      "../src/providers/verifyGate"
    );
    const project = projects.create(
      "Quota Campaign",
      `quota-${Date.now()}.example`,
      "html",
      "customer_outreach",
      {},
    );
    expect(isVerifyQuotaExhausted(project.id)).toBe(false);
    markVerifyQuotaExhausted(project.id, "reoon 402 out of credits");
    const gate = verificationGate(project.id);
    expect(gate.allow).toBe(false);
    expect(gate.kind).toBe("quota");

    const audience = audiences.create(project.id, {
      name: "Quota ICP",
      searchQueries: ["funded saas startups"],
      origin: "generated",
    });
    expect(audience.nextSearchPage).toBe(1);
    audiences.advanceSearchPage(audience.id);
    expect(audiences.get(audience.id)?.nextSearchPage).toBe(2);
  });
});

describe("domain verify credit protection", () => {
  test("does not mark catch-all from a single all-risky person", async () => {
    const { domainVerifyState } = await import("../src/providers/domainVerifyState");
    const domain = `catch-one-${Date.now()}.example`;
    expect(domainVerifyState.isCatchAll(domain)).toBe(false);
    domainVerifyState.recordPersonOutcome(domain, "all_risky");
    expect(domainVerifyState.isCatchAll(domain)).toBe(false);
    expect(domainVerifyState.allRiskyStreak(domain)).toBe(1);
  });

  test("marks catch-all only after 3 consecutive all-risky people", async () => {
    const {
      CATCH_ALL_PERSON_THRESHOLD,
      domainVerifyState,
    } = await import("../src/providers/domainVerifyState");
    const domain = `catch-three-${Date.now()}.example`;

    domainVerifyState.recordPersonOutcome(domain, "all_risky");
    domainVerifyState.recordPersonOutcome(domain, "all_risky");
    expect(domainVerifyState.isCatchAll(domain)).toBe(false);

    const third = domainVerifyState.recordPersonOutcome(domain, "all_risky");
    expect(third.allRiskyStreak).toBe(CATCH_ALL_PERSON_THRESHOLD);
    expect(domainVerifyState.isCatchAll(domain)).toBe(true);
  });

  test("resets the all-risky streak when a person gets valid or invalid", async () => {
    const { domainVerifyState } = await import("../src/providers/domainVerifyState");
    const domain = `catch-reset-${Date.now()}.example`;

    domainVerifyState.recordPersonOutcome(domain, "all_risky");
    domainVerifyState.recordPersonOutcome(domain, "all_risky");
    expect(domainVerifyState.allRiskyStreak(domain)).toBe(2);

    domainVerifyState.recordPersonOutcome(domain, "saw_invalid");
    expect(domainVerifyState.allRiskyStreak(domain)).toBe(0);
    expect(domainVerifyState.isCatchAll(domain)).toBe(false);

    domainVerifyState.recordPersonOutcome(domain, "all_risky");
    domainVerifyState.recordPersonOutcome(domain, "valid");
    expect(domainVerifyState.allRiskyStreak(domain)).toBe(0);
  });
});
