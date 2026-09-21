---
name: hunt-leads
description: Hunt B2B leads with AI Hunter — start a hunt, wait for it to finish, export Instantly/Smartlead CSV. Use when the user wants leads, prospects, an ICP-matched contact list, or a re-export of an existing hunt. Covers first-run settings and the verbs that actually send mail (campaign / send / auto-send) — which you never run unasked.
user-invocable: true
argument-hint: "[N]"
---

# Finding leads with AI Hunter

AI Hunter is a self-hosted hunt pipeline. You describe a product and a target market; each hunt searches public web pages, extracts companies and people, writes down **why** each row fits, and exports CSV.

The default deliverable of "get me leads" is **CSV**, not outbound mail. Hunt, inspect, export. Sending is a separate, explicit ask.

This skill is original to this tree. OpenOutreach's find-leads skill is a thought source only (GPL-3.0) — do not copy its files.

## What this system is

Backend: FastAPI + LangGraph.

```
parse_description → insight → keyword_gen → search → lead_extract → evaluate
                                              ↑                         |
                                              └──── continue ───────────┘
                                                                   finish → email_craft
```

Optional extras (**default off**):

- Site scan at Insight (`SITE_SCAN_ENABLED`) — high-value pages of the seller's own site (pricing / about / product). Firecrawl if `FIRECRAWL_API_KEY` is set, otherwise plain fetch. Cap `SITE_SCAN_MAX_PAGES` (default 6, hard max 12). Never crawl a whole site.
- OSINT enrichment after LeadExtract (`OSINT_ENRICHMENT_ENABLED`) — public-page emails / tech stack / legal name
- Social presence after LeadExtract (`SOCIAL_PRESENCE_ENABLED`) — bounded username scan (whitelist, ≤20 sites / lead)

Do not turn these on unless the user asked. Do not scan 3000 social sites. Do not enable Firecrawl without a key the user provided.

## Is it running?

Base URL is local by default: `http://127.0.0.1:8000`.

```bash
curl -s http://127.0.0.1:8000/health
```

If `API_ACCESS_TOKEN` is set, every `/api/v1/*` call needs `Authorization: Bearer <token>`.

Settings live at `GET /api/settings` / `POST /api/settings`, and on the Settings page under「可选 enrichment 模块」. Empty values are skipped. Never invent SMTP passwords, never accept legal/compliance notices on the user's behalf, never enable auto-send yourself. Do not flip OSINT / social / licensed-finder / site-scan on from the API if the operator has not asked.

## The work you reach for

"Get me leads" → **create a hunt, wait, export CSV**. That is the whole job.

```bash
# 1. start
curl -s -X POST http://127.0.0.1:8000/api/v1/hunts \
  -H "Content-Type: application/json" \
  -d '{"website_url":"https://seller.example","product_keywords":["solar inverter"],"target_regions":["DE"],"target_lead_count":10}'

# 2. poll (or subscribe to SSE)
curl -s http://127.0.0.1:8000/api/v1/hunts/{hunt_id}/status

# 3. export — this is the integration
curl -s http://127.0.0.1:8000/api/v1/hunts/{hunt_id}/export.csv -o leads.csv

# 4. operator report (optional; not the Instantly file)
curl -s http://127.0.0.1:8000/api/v1/hunts/{hunt_id}/export-report.txt -o hunt-report.txt
```

`GET /api/v1/export.csv` dumps every stored hunt. `GET /api/v1/hunts/{hunt_id}/export.csv` is one hunt. Prefer the hunt-scoped URL when the user pointed at a specific run.

The Hunt detail page button「导出 CSV」hits the hunt-scoped export URL. If that request fails, the UI falls back to the same 10 columns locally — never the old wide score table. The lead list and detail sheet show `reason` as「资格理由」; quote it when summarising. If OSINT / social ran, the detail sheet also shows「公开页补全」(`tech_stack` / `legal_name` / `github_org` / `social_profiles` / `evidence_urls`). EmailCraft uses those fields as personalization material; it never pastes `reason` into outreach. The Evaluate stage table is keyword effectiveness (`keyword_performance`: results / leads / precision / high|medium|low). Search-stage counts are raw, not a substitute for that table. The Hunt detail button「导出报告」hits `GET /api/v1/hunts/{hunt_id}/export-report.txt` (insight / keywords / raw search counts / keyword effectiveness). If that request fails, the UI falls back to the same text locally. The report must not contain per-lead `reason` or emails. Do not invent a ranking from fit scores.

Re-export of an existing hunt is just the export URL again. Do not start a second hunt to "give me that file again".

`N` in the user's ask maps to `target_lead_count`. It is a budget of qualified rows, not a promise that search will fill it.

## CSV contract

Columns, in this order — **do not rename**:

```
email, first_name, last_name, company, title, website, linkedin_url, reason, lead_id, qualified_at
```

- These names are the importers' names (Instantly / Smartlead). The file should import without column mapping.
- **`reason` is the point.** It is the written rationale for choosing this row. Prose, so it contains commas and quotes: parse with a real CSV reader, never by splitting on `,`. When summarising leads for the user, quote the reason.
- **There is no score column, on purpose.** Fit/match scores may exist inside the JSON hunt record as spend gates. Do not add them to the CSV, and do not synthesise a ranking for the operator from them.
- `lead_id` is the stable dedupe key across exports. `qualified_at` is when the row was sealed.
- **`reason` is written for the operator, not the prospect.** Third-person, evaluative. Never paste it into a message to the lead. Outreach copy comes from the lead profile / page evidence, not from `reason`.

**The CSV is the integration.** Instantly, Smartlead, Lemlist, HubSpot, a spreadsheet — the file imports as-is. There is no adapter to look for. Tell the operator to turn on their tool's import deduplication; a re-exported lead can otherwise be contacted twice.

Blank `email` is allowed. A person + company + reason with no address is still a lead.

## What costs money / what is gated

Discovery burns the user's own search + LLM keys. Extra paid lookups (email verification credits, BuiltWith, licensed finders) are opt-in.

- Do not set `emails` / hunter / verify flags unless the user asked for addresses.
- `should_spend_verify_credits` only fires when the row already has a usable reason and cleared the fit gate. Do not lower that gate yourself.
- OSINT, social presence, site scan, and the licensed finder stay off until explicitly enabled.
- Licensed finder: `POST /api/v1/licensed-finder/search`. Needs `LICENSED_FINDER_ENABLED` and `BETTERCONTACT_API_KEY`. Bare search cannot spend enrichment credits; `enrich_emails: true` is the paid form. Operator UI is `/licensed-finder` (nav「持牌找人」). Do not start a hunt to use this bypass.

## Sending mail — never unasked

The pipeline can also draft and send. **Never do that unless the user asked for mail to go out, in this session, in so many words.** "Find me leads", "export CSV", "set this up", "continue the hunt" are not that ask.

Outbound surfaces in this tree:

- `POST /api/v1/hunts/{hunt_id}/email-sequences/{sequence_index}/send`
- campaign + scheduler routes under `/api/v1` (email)
- `email_auto_send_enabled`
- `email_sequence_enabled`

Hard rules:

- Default `email_dry_run=true`. Leave it on unless the user asked to actually send.
- Default `email_require_approval_before_send=true`. Do not approve sequences yourself.
- Role inboxes and freemail are blocked by guards. Do not bypass.
- Daily / hourly caps and the business-hours window are load-bearing. A send pass that opens nothing can still be correct.
- Suppression list is load-bearing. Unsubscribed addresses stay blocked. Live sends append an opt-out line and `List-Unsubscribe` headers. Do not strip them.
- Public unsubscribe: `GET/POST /api/v1/unsubscribe?email=&token=`. Operator list/add: `GET/POST /api/v1/suppressions` (auth). Set `EMAIL_PUBLIC_BASE_URL` to a URL the recipient can actually reach.
- Do not enable `email_auto_send_enabled` as a side effect of a hunt.

If they later say "email them", confirm dry-run is off, SMTP is theirs, and the sequences are approved — then send only what they named.

## Things not to do

- Don't send mail the user didn't ask for. Hunt + CSV is the answer to "get me leads".
- Don't invent a watch loop beyond status/SSE for the hunt they started.
- Don't spend verify / OSINT / licensed-finder / 3000-site social scans they didn't ask for.
- Don't rename CSV columns or add a score column.
- Don't paste `reason` into outreach.
- Don't copy files out of `refs/OpenOutreach` (GPL-3.0). Thought only.
- Don't write into `refs/` — that tree is read-only.

## Operator checklist after a hunt

1. Hand them `leads.csv` (or the export URL).
2. Quote a few `reason` values so they can see why rows survived.
3. Remind them to enable import dedupe on the sequencer.
4. Stop. Wait for an explicit send ask.
