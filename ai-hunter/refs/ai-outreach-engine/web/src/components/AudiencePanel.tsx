import { useState } from "react";
import {
  api,
  type Audience,
  type Overview,
  type PlaybookPreferences,
  type WorkflowType,
} from "../lib/api";
import { regenWhatHappens } from "../lib/confirmCopy";
import { isPipelineBusy, PROSPECT_STAGES } from "../lib/pipelineBusy";
import {
  COMPANY_SIZE_OPTIONS,
  DEFAULT_SOURCE_ADAPTERS,
  DEFAULT_STARTUP_SIZES,
  SOURCE_ADAPTER_OPTIONS,
} from "../lib/sourceAdapters";
import { EmailCopyEditor, EmailPreviewModal } from "./EmailPreview";
import { PipelineAttentionBar } from "./PipelineAttentionBar";
import {
  Button,
  Card,
  CheckboxRow,
  ConfirmDialog,
  Disclosure,
  Empty,
  Field,
  KebabMenu,
  Modal,
  Notice,
  Pill,
  StackList,
  TagList,
  WhatHappensList,
  inputClass,
} from "./ui";

/** Placeholders supported in Jobs lane subject + body templates. */
const JOB_PLACEHOLDERS: Array<{ field: string; where: string; meaning: string }> = [
  { field: "{{firstName}}", where: "Subject or body", meaning: "Hiring contact’s first name" },
  { field: "{{company}}", where: "Subject or body", meaning: "Employer / company name" },
  { field: "{{role}}", where: "Subject or body", meaning: "Job title from the listing" },
  {
    field: "{{techStack}}",
    where: "Subject or body",
    meaning: "Key stack from the JD (e.g. React, TypeScript)",
  },
  {
    field: "{{companyFocus}}",
    where: "Subject or body",
    meaning: "What the product/team builds or the problem they solve",
  },
  {
    field: "{{workStyle}}",
    where: "Subject or body",
    meaning: "Remote, Hybrid, or On-site (empty if unknown)",
  },
  {
    field: "{{senderFullName}}",
    where: "Body (signature)",
    meaning: "Your full name from sender settings",
  },
];

function JobPlaceholdersModal({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Job email placeholders"
      description="Use these in the lane template subject and body. When “Tailor emails to each job listing” is off, we fill them from that company’s JD and send the template as-is."
      wide
    >
      <div className="space-y-2 overflow-y-auto">
        {JOB_PLACEHOLDERS.map((row) => (
          <div
            key={row.field}
            className="grid gap-1 rounded-xl border border-line bg-surface-2/50 px-3.5 py-3 sm:grid-cols-[10rem_8rem_1fr] sm:items-baseline sm:gap-3"
          >
            <code className="text-[12px] font-semibold text-accent">{row.field}</code>
            <span className="text-[11px] font-medium text-ink-faint">{row.where}</span>
            <span className="text-[13px] text-ink-soft">{row.meaning}</span>
          </div>
        ))}
      </div>
      <div className="mt-4 flex justify-end">
        <Button variant="primary" onClick={onClose}>
          Got it
        </Button>
      </div>
    </Modal>
  );
}

function emptyAudience(workflowType: WorkflowType): Partial<Audience> {
  const isJob = workflowType === "job_outreach";
  return {
    name: "",
    description: "",
    targetRoles: [],
    jobTitles: [],
    industries: [],
    companySizes: [...DEFAULT_STARTUP_SIZES],
    sourceAdapters: isJob ? ["web"] : [...DEFAULT_SOURCE_ADAPTERS],
    fundedAfterYear: 2018,
    searchQueries: [],
    painPoints: [],
    valueProp: "",
    templateSubject: "",
    templateBody: "",
    templateHtml: "",
    enabled: true,
  };
}

function asStringList(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.map((item) => String(item).trim()).filter(Boolean);
  }
  if (typeof value === "string" && value.trim()) {
    return value
      .split(/,|\n/)
      .map((item) => item.trim())
      .filter(Boolean);
  }
  return [];
}

/** Surface the setup-form requirements that drive job search lanes. */
function JobSearchBriefCard({ workflowData }: { workflowData: string }) {
  let data: Record<string, unknown> = {};
  try {
    data = JSON.parse(workflowData || "{}") as Record<string, unknown>;
  } catch {
    data = {};
  }
  const prefs =
    data.preferences && typeof data.preferences === "object"
      ? (data.preferences as Record<string, unknown>)
      : {};
  const roles = asStringList(data.targetRoles);
  const locations = asStringList(data.locations);
  const remote = typeof data.remotePreference === "string" ? data.remotePreference.trim() : "";
  const seniority = typeof data.seniority === "string" ? data.seniority.trim() : "";
  const notes = typeof data.notes === "string" ? data.notes.trim() : "";
  const sizes = asStringList(prefs.companySizes);
  const industries = asStringList(prefs.industries);
  const hasAnything =
    roles.length > 0 ||
    locations.length > 0 ||
    remote ||
    seniority ||
    notes ||
    sizes.length > 0 ||
    industries.length > 0;
  if (!hasAnything) return null;

  return (
    <Card title="Your search brief">
      <p className="mb-3 text-xs leading-relaxed text-ink-faint">
        From setup. Search lanes and employer results should follow this — regenerate lanes after
        you change it on the first page.
      </p>
      <div className="grid gap-3 sm:grid-cols-2">
        {roles.length > 0 && (
          <div>
            <p className="text-[11px] font-semibold tracking-wide text-ink-faint uppercase">
              Roles you want
            </p>
            <TagList items={roles} />
          </div>
        )}
        {locations.length > 0 && (
          <div>
            <p className="text-[11px] font-semibold tracking-wide text-ink-faint uppercase">
              Locations
            </p>
            <TagList items={locations} />
          </div>
        )}
        {remote && (
          <div>
            <p className="text-[11px] font-semibold tracking-wide text-ink-faint uppercase">
              Remote
            </p>
            <p className="mt-1 text-[13px] text-ink-soft">{remote}</p>
          </div>
        )}
        {seniority && (
          <div>
            <p className="text-[11px] font-semibold tracking-wide text-ink-faint uppercase">
              Seniority
            </p>
            <p className="mt-1 text-[13px] text-ink-soft">{seniority}</p>
          </div>
        )}
        {sizes.length > 0 && (
          <div>
            <p className="text-[11px] font-semibold tracking-wide text-ink-faint uppercase">
              Company sizes
            </p>
            <TagList items={sizes} />
          </div>
        )}
        {industries.length > 0 && (
          <div>
            <p className="text-[11px] font-semibold tracking-wide text-ink-faint uppercase">
              Industries
            </p>
            <TagList items={industries} />
          </div>
        )}
      </div>
      {notes && (
        <div className="mt-3">
          <p className="text-[11px] font-semibold tracking-wide text-ink-faint uppercase">Notes</p>
          <p className="mt-1 text-[13px] leading-relaxed text-ink-soft whitespace-pre-wrap">
            {notes}
          </p>
        </div>
      )}
    </Card>
  );
}

function toggleInList(list: string[], value: string, on: boolean): string[] {
  if (on) return list.includes(value) ? list : [...list, value];
  return list.filter((item) => item !== value);
}

function adapterLabel(id: string): string {
  return SOURCE_ADAPTER_OPTIONS.find((option) => option.id === id)?.label ?? id;
}

const linesToArray = (value: string) =>
  value
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);

function AudienceEditor({
  projectId,
  workflowType,
  initial,
  onSave,
  onCancel,
}: {
  projectId: number;
  workflowType: WorkflowType;
  initial: Partial<Audience>;
  onSave: (draft: Partial<Audience>) => void;
  onCancel: () => void;
}) {
  const [draft, setDraft] = useState(initial);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [placeholdersOpen, setPlaceholdersOpen] = useState(false);
  const isJob = workflowType === "job_outreach";

  const set = <K extends keyof Audience>(key: K, value: Audience[K]) =>
    setDraft((current) => ({ ...current, [key]: value }));

  return (
    <div className="space-y-5">
      <div className="grid gap-4 md:grid-cols-2">
        <Field label="Name">
          <input
            className={inputClass}
            value={draft.name ?? ""}
            placeholder={
              isJob
                ? "Founding eng at Seed AI startups"
                : "Recruiters at fast-growing startups"
            }
            onChange={(event) => set("name", event.target.value)}
          />
        </Field>
        <Field
          label={isJob ? "Candidate positioning hook" : "One-line value proposition"}
        >
          <input
            className={inputClass}
            value={draft.valueProp ?? ""}
            onChange={(event) => set("valueProp", event.target.value)}
          />
        </Field>
      </div>

      <Field label="Description">
        <textarea
          className={`${inputClass} h-16 resize-y`}
          value={draft.description ?? ""}
          onChange={(event) => set("description", event.target.value)}
        />
      </Field>

      <div className="rounded-2xl border border-line bg-surface-2/50 p-4 sm:p-5">
        <div className="mb-3 flex flex-wrap items-start justify-between gap-3">
          <div>
            <h4 className="text-[13px] font-semibold tracking-tight">
              {isJob ? "Lane email template" : "Email template"}
            </h4>
            <p className="mt-1 text-xs leading-relaxed text-ink-faint">
              {isJob
                ? "Used for every contact in this lane when “Tailor emails to each job listing” is off. Placeholders fill from that company’s JD. No links."
                : "Plain-text cold email for this ICP."}{" "}
              {!isJob && (
                <>
                  Merge fields:{" "}
                  <code className="rounded bg-surface px-1 py-0.5 text-[11px]">{"{{firstName}}"}</code>
                  ,{" "}
                  <code className="rounded bg-surface px-1 py-0.5 text-[11px]">{"{{company}}"}</code>,{" "}
                  <code className="rounded bg-surface px-1 py-0.5 text-[11px]">
                    {"{{observation}}"}
                  </code>
                  ,{" "}
                  <code className="rounded bg-surface px-1 py-0.5 text-[11px]">{"{{painPoint}}"}</code>,{" "}
                  <code className="rounded bg-surface px-1 py-0.5 text-[11px]">{"{{valueProp}}"}</code>,{" "}
                  <code className="rounded bg-surface px-1 py-0.5 text-[11px]">
                    {"{{senderFirstName}}"}
                  </code>
                  .
                </>
              )}
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            {isJob && (
              <Button onClick={() => setPlaceholdersOpen(true)}>Placeholders</Button>
            )}
            <Button
              disabled={!(draft.templateBody ?? "").trim()}
              onClick={() => setPreviewOpen(true)}
            >
              Preview send
            </Button>
          </div>
        </div>
        <EmailCopyEditor
          subject={draft.templateSubject ?? ""}
          body={draft.templateBody ?? ""}
          onSubjectChange={(value) => set("templateSubject", value)}
          onBodyChange={(value) => set("templateBody", value)}
        />
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        {isJob && (
          <Field
            label="Roles you want"
            hint="One per line. Used to search for open jobs / hiring employers."
          >
            <textarea
              className={`${inputClass} h-24 resize-y`}
              value={(draft.targetRoles ?? []).join("\n")}
              placeholder={"Founding Engineer\nSenior Full-Stack Engineer"}
              onChange={(event) => set("targetRoles", linesToArray(event.target.value))}
            />
          </Field>
        )}
        <Field
          label={isJob ? "People to contact" : "Job titles"}
          hint={
            isJob
              ? "One per line. Hiring contacts to email (Recruiter, CTO, Hiring Manager) — not the role you want."
              : "One per line."
          }
        >
          <textarea
            className={`${inputClass} h-24 resize-y`}
            value={(draft.jobTitles ?? []).join("\n")}
            placeholder={
              isJob
                ? "Hiring Manager\nCTO\nRecruiter"
                : undefined
            }
            onChange={(event) => set("jobTitles", linesToArray(event.target.value))}
          />
        </Field>
        <Field label="Industries" hint="One per line.">
          <textarea
            className={`${inputClass} h-24 resize-y`}
            value={(draft.industries ?? []).join("\n")}
            onChange={(event) => set("industries", linesToArray(event.target.value))}
          />
        </Field>
        <Field
          label={isJob ? "Hiring pains you solve" : "Pain points"}
          hint="One per line. These shape the email copy."
        >
          <textarea
            className={`${inputClass} h-24 resize-y`}
            value={(draft.painPoints ?? []).join("\n")}
            onChange={(event) => set("painPoints", linesToArray(event.target.value))}
          />
        </Field>
        <Field
          label="Search queries"
          hint={
            isJob
              ? "One per line. Seeds open-role / careers search (planner also adds role×location queries)."
              : "One per line. Used by Open web search (and as Serper fallback)."
          }
        >
          <textarea
            className={`${inputClass} h-24 resize-y`}
            value={(draft.searchQueries ?? []).join("\n")}
            onChange={(event) => set("searchQueries", linesToArray(event.target.value))}
          />
        </Field>
      </div>

      <div className="space-y-4 rounded-2xl border border-line bg-surface-2/40 p-4 sm:p-5">
        <div>
          <h4 className="text-[13px] font-semibold tracking-tight">
            {isJob ? "Employer search filters" : "Company search filters"}
          </h4>
          <p className="mt-1 text-xs leading-relaxed text-ink-faint">
            {isJob
              ? "Team size and year filters for employers. Jobs search uses open web (Serper) for hiring signals — directory adapters are for customer outreach."
              : "These are included in every company batch: team size, launch/funding year, and which directories / open-web sources to pull from. Domains already saved in this playbook are skipped automatically."}
          </p>
        </div>

        <Field
          label="Team size"
          hint="Hard filter when the source knows headcount. New AI audiences default to 1–10 / 11–50 / 51–200 if the model omits sizes."
        >
          <div className="flex flex-wrap gap-2">
            {COMPANY_SIZE_OPTIONS.map((size) => {
              const checked = (draft.companySizes ?? []).includes(size);
              return (
                <button
                  key={size}
                  type="button"
                  className={`rounded-lg border px-3 py-1.5 text-[12px] font-medium transition ${
                    checked
                      ? "border-warn/40 bg-warn-soft text-warn"
                      : "border-line bg-surface text-ink-soft hover:bg-surface-2"
                  }`}
                  onClick={() =>
                    set("companySizes", toggleInList(draft.companySizes ?? [], size, !checked))
                  }
                >
                  {size}
                </button>
              );
            })}
          </div>
        </Field>

        <Field
          label="Funded / launched after"
          hint="Uses accelerator batch years when known (e.g. YC W24). Server default for new AI audiences is 2018; 0 = no year filter."
        >
          <input
            type="number"
            min={0}
            max={2100}
            className={`${inputClass} max-w-[10rem]`}
            value={draft.fundedAfterYear ?? 0}
            onChange={(event) =>
              set("fundedAfterYear", Number.parseInt(event.target.value || "0", 10) || 0)
            }
          />
        </Field>

        {!isJob && (
          <Field
            label="Company sources"
            hint="Adapters pull from known startup directories. Open web uses Serper + your search queries. Leave all unchecked only if you want signal-based auto-select."
          >
            <div className="grid gap-2 sm:grid-cols-2">
              {SOURCE_ADAPTER_OPTIONS.map((option) => {
                const checked = (draft.sourceAdapters ?? []).includes(option.id);
                return (
                  <CheckboxRow
                    key={option.id}
                    checked={checked}
                    label={option.label}
                    hint={option.hint}
                    onChange={(on) =>
                      set(
                        "sourceAdapters",
                        toggleInList(draft.sourceAdapters ?? [], option.id, on),
                      )
                    }
                  />
                );
              })}
            </div>
          </Field>
        )}
      </div>

      <div className="flex flex-wrap gap-2">
        <Button variant="primary" disabled={!draft.name?.trim()} onClick={() => onSave(draft)}>
          {isJob ? "Save search lane" : "Save audience"}
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
      </div>

      <EmailPreviewModal
        open={previewOpen}
        onClose={() => setPreviewOpen(false)}
        projectId={projectId}
        subject={draft.templateSubject ?? ""}
        body={draft.templateBody ?? ""}
        fillSampleMergeFields
        painPoint={(draft.painPoints ?? [])[0] ?? ""}
        valueProp={draft.valueProp ?? ""}
        title="Audience template preview"
      />
      <JobPlaceholdersModal open={placeholdersOpen} onClose={() => setPlaceholdersOpen(false)} />
    </div>
  );
}

function AudienceCard({
  projectId,
  workflowType,
  audience,
  busy,
  prospectingBusy,
  searchReady,
  editing,
  onEdit,
  onCancelEdit,
  onSave,
  onProspect,
  onRegenerate,
  onRewriteEmail,
  onToggleEnabled,
  onDelete,
}: {
  projectId: number;
  workflowType: WorkflowType;
  audience: Audience;
  busy: boolean;
  prospectingBusy: boolean;
  searchReady: boolean;
  editing: boolean;
  onEdit: () => void;
  onCancelEdit: () => void;
  onSave: (draft: Partial<Audience>) => void;
  onProspect: () => void;
  onRegenerate: () => void;
  onRewriteEmail: () => void;
  onToggleEnabled: () => void;
  onDelete: () => void;
}) {
  const [previewOpen, setPreviewOpen] = useState(false);
  const locked = busy || prospectingBusy;
  const isJob = workflowType === "job_outreach";

  if (editing) {
    return (
      <article className="animate-rise rounded-[var(--radius-card)] border border-line bg-surface p-5 shadow-[var(--shadow-soft)] sm:p-6">
        <AudienceEditor
          projectId={projectId}
          workflowType={workflowType}
          initial={audience}
          onCancel={onCancelEdit}
          onSave={onSave}
        />
      </article>
    );
  }

  return (
    <article
      className={`animate-rise rounded-[var(--radius-card)] border bg-surface shadow-[var(--shadow-soft)] transition-shadow hover:shadow-[var(--shadow-lift)] ${
        audience.enabled ? "border-line" : "border-line opacity-70"
      }`}
    >
      <header className="flex items-center justify-between gap-3 border-b border-line px-5 py-4 sm:px-6">
        <div className="min-w-0">
          <h3 className="truncate text-[16px] font-semibold tracking-tight text-ink">
            {audience.name}
          </h3>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button
            variant="primary"
            disabled={locked || !searchReady || !audience.enabled}
            title={
              prospectingBusy
                ? "Prospecting is already running — watch the bottom-right toast or hit Stop"
                : searchReady
                  ? isJob
                    ? `Search open roles + hiring employers from page ${audience.nextSearchPage}, then find hiring contacts`
                    : `Pull YC/accelerators + search from page ${audience.nextSearchPage} (~20 companies → ~50 contacts)`
                  : "Needs SERPER_API_KEY"
            }
            onClick={onProspect}
          >
            {prospectingBusy ? "Finding…" : isJob ? "Find hiring targets" : "Find next batch"}
          </Button>
          <KebabMenu
            items={[
              { label: "Edit", disabled: locked, onClick: onEdit },
              {
                label: "Preview email",
                disabled: !audience.templateBody.trim(),
                onClick: () => setPreviewOpen(true),
              },
              {
                label: "Rewrite email",
                disabled: locked,
                onClick: onRewriteEmail,
              },
              {
                label: isJob ? "Regenerate search lane" : "Regenerate audience",
                disabled: locked,
                onClick: onRegenerate,
              },
              {
                label: audience.enabled ? "Disable" : "Enable",
                disabled: locked,
                onClick: onToggleEnabled,
              },
              { label: "Delete", disabled: locked, danger: true, onClick: onDelete },
            ]}
          />
        </div>
      </header>

      <div className="space-y-5 px-5 py-5 sm:px-6">
        {(audience.description || audience.valueProp) && (
          <div className="max-w-3xl space-y-2">
            {audience.description && (
              <p className="text-sm leading-relaxed text-ink-soft">{audience.description}</p>
            )}
            {audience.valueProp && (
              <p className="text-xs leading-relaxed text-ink-faint italic">“{audience.valueProp}”</p>
            )}
          </div>
        )}

        <div className="flex flex-wrap gap-2">
          <Pill
            tone={
              audience.origin === "generated"
                ? "info"
                : audience.origin === "retained"
                  ? "warn"
                  : "neutral"
            }
          >
            {audience.origin === "retained" ? "kept (has data)" : audience.origin}
          </Pill>
          <Pill tone="neutral">batch {audience.nextSearchPage}</Pill>
          {!audience.enabled && <Pill tone="warn">disabled</Pill>}
        </div>

        <div className={`grid min-w-0 gap-4 ${isJob ? "md:grid-cols-2 lg:grid-cols-4" : "md:grid-cols-3"}`}>
          {isJob && (
            <Field label="Roles you want" accent="accent">
              <TagList
                items={audience.targetRoles.length ? audience.targetRoles : ["—"]}
                tone="titles"
              />
            </Field>
          )}
          <Field label={isJob ? "People to contact" : "Job titles"} accent="accent">
            <TagList items={audience.jobTitles} tone="titles" />
          </Field>
          <Field label="Industries" accent="teal">
            <TagList items={audience.industries} tone="industries" />
          </Field>
          <Field label="Team size" accent="warn">
            <TagList
              items={
                audience.companySizes.length
                  ? audience.companySizes
                  : ["any size"]
              }
              tone="sizes"
            />
          </Field>
        </div>

        <div className="grid min-w-0 gap-4 md:grid-cols-2">
          {!isJob && (
            <Field label="Company sources" accent="sky">
              <TagList
                items={
                  audience.sourceAdapters?.length
                    ? audience.sourceAdapters.map(adapterLabel)
                    : ["Auto (from campaign signals)"]
                }
                tone="platforms"
              />
            </Field>
          )}
          <Field label="Funded / launched after" accent="violet">
            <TagList
              items={[
                (audience.fundedAfterYear ?? 0) > 0
                  ? String(audience.fundedAfterYear)
                  : "No year filter",
              ]}
              tone="features"
            />
          </Field>
        </div>

        <div className="space-y-2">
          {audience.searchQueries.length > 0 && (
            <Disclosure title="Search queries" count={audience.searchQueries.length} tone="sky">
              <ul className="space-y-2">
                {audience.searchQueries.map((query) => (
                  <li
                    key={query}
                    className="rounded-xl border border-sky/15 bg-surface px-3 py-2 font-mono text-[12px] leading-relaxed text-ink"
                  >
                    {query}
                  </li>
                ))}
              </ul>
            </Disclosure>
          )}

          {(audience.templateSubject || audience.templateBody) && (
            <Disclosure
              title={isJob ? "Lane email template" : "Email template"}
              tone="accent"
            >
              <div className="space-y-3">
                <div className="rounded-xl border border-accent/15 bg-surface p-3.5">
                  <div className="text-[13px] font-semibold text-ink">
                    {audience.templateSubject || "(AI writes subject)"}
                  </div>
                  <pre className="mt-2 font-mono text-[12px] leading-relaxed whitespace-pre-wrap text-ink-soft">
                    {audience.templateBody || "(AI writes body)"}
                  </pre>
                </div>
                <Button onClick={() => setPreviewOpen(true)}>Preview send</Button>
              </div>
            </Disclosure>
          )}

          {audience.painPoints.length > 0 && (
            <Disclosure
              title={isJob ? "Hiring pains you solve" : "Pain points"}
              count={audience.painPoints.length}
              tone="warn"
            >
              <StackList items={audience.painPoints} tone="pain" />
            </Disclosure>
          )}
        </div>
      </div>

      <EmailPreviewModal
        open={previewOpen}
        onClose={() => setPreviewOpen(false)}
        projectId={projectId}
        subject={audience.templateSubject}
        body={audience.templateBody}
        fillSampleMergeFields
        painPoint={audience.painPoints[0] ?? ""}
        valueProp={audience.valueProp}
        title={`Preview · ${audience.name}`}
      />
    </article>
  );
}

function JobEmailModeCard({
  preferences,
  projectId,
  disabled,
  onSaved,
  onError,
  onNotice,
}: {
  preferences: PlaybookPreferences | undefined;
  projectId: number;
  disabled: boolean;
  onSaved: () => void;
  onError: (message: string) => void;
  onNotice: (message: string) => void;
}) {
  const [placeholdersOpen, setPlaceholdersOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const tailor =
    preferences?.kind === "job_outreach" && Boolean(preferences.tailorEmailsToJobListing);

  async function setTailor(next: boolean) {
    const base: Extract<PlaybookPreferences, { kind: "job_outreach" }> =
      preferences?.kind === "job_outreach"
        ? preferences
        : {
            kind: "job_outreach",
            companySizes: [],
            industries: [],
            maxContactsPerCompany: 5,
            localizeEmails: true,
            tailorEmailsToJobListing: false,
          };
    setSaving(true);
    try {
      await api.updateProject(projectId, {
        preferences: {
          ...base,
          tailorEmailsToJobListing: next,
        },
      });
      onNotice(
        next
          ? "On: each contact email will be written from that job listing."
          : "Off: each contact uses the lane template (JD placeholders filled).",
      );
      onSaved();
    } catch (error) {
      onError(error instanceof Error ? error.message : "Failed to save email mode");
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      <Card title="Contact email mode">
        <label className="flex cursor-pointer gap-3 rounded-2xl border border-line bg-surface-2/50 px-3.5 py-3 text-left">
          <input
            type="checkbox"
            className="mt-0.5 h-4 w-4 shrink-0 accent-[var(--accent)]"
            checked={tailor}
            disabled={disabled || saving}
            onChange={(event) => void setTailor(event.target.checked)}
          />
          <span className="min-w-0">
            <span className="block text-[13px] font-semibold text-ink">
              Tailor emails to each job listing
            </span>
            <span className="mt-0.5 block text-[12px] leading-relaxed text-ink-soft">
              {tailor
                ? "On — AI writes a short email from that company’s JD for every contact (lane template is ignored)."
                : "Off — use each search lane’s email template as-is. We only fill placeholders like role, stack, and remote/hybrid from the JD."}
            </span>
          </span>
        </label>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <Button disabled={disabled || saving} onClick={() => setPlaceholdersOpen(true)}>
            View placeholders
          </Button>
          <p className="text-[11px] text-ink-faint">
            After changing mode or a lane template, redraft contacts on Review &amp; send.
          </p>
        </div>
      </Card>
      <JobPlaceholdersModal open={placeholdersOpen} onClose={() => setPlaceholdersOpen(false)} />
    </>
  );
}

export function AudiencePanel({
  overview,
  searchReady,
  onRefresh,
  onError,
  onNotice,
  onOpenActivity,
}: {
  overview: Overview | null;
  searchReady: boolean;
  onRefresh: () => void;
  onError: (message: string) => void;
  onNotice: (message: string) => void;
  onOpenActivity: () => void;
}) {
  const [editingId, setEditingId] = useState<number | null>(null);
  const [creating, setCreating] = useState(false);
  const [busy, setBusy] = useState(false);
  const [regenOpen, setRegenOpen] = useState(false);
  const [preserveProspecting, setPreserveProspecting] = useState(true);
  const [deleteTarget, setDeleteTarget] = useState<Audience | null>(null);

  if (!overview) return <Empty>Create a campaign first.</Empty>;

  async function run(action: () => Promise<unknown>, notice?: string) {
    setBusy(true);
    try {
      await action();
      if (notice) onNotice(notice);
      onRefresh();
    } catch (error) {
      onError(error instanceof Error ? error.message : "Something went wrong");
    } finally {
      setBusy(false);
    }
  }

  const workflowType = overview.project.workflow_type;
  const isJob = workflowType === "job_outreach";
  const unit = isJob ? "search lane" : "target audience";
  const contactCount = overview.counts.contacts;
  const companyCount = overview.counts.companies;
  const prospectingBusy = isPipelineBusy(overview, PROSPECT_STAGES);
  const actionsLocked = busy || prospectingBusy;
  const visibleAudiences = overview.audiences.filter(
    (audience) => audience.name !== "Manual contacts",
  );

  return (
    <div className="space-y-5">
      <PipelineAttentionBar
        overview={overview}
        onOpenActivity={onOpenActivity}
        stages={PROSPECT_STAGES}
      />

      {isJob && <JobSearchBriefCard workflowData={overview.project.workflow_data} />}

      {isJob && (
        <JobEmailModeCard
          preferences={overview.preferences}
          projectId={overview.project.id}
          disabled={actionsLocked}
          onSaved={onRefresh}
          onError={onError}
          onNotice={onNotice}
        />
      )}

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <p className="text-[15px] font-semibold text-ink">
            {visibleAudiences.length} {unit}
            {visibleAudiences.length === 1 ? "" : "s"}
          </p>
          <p className="mt-1 text-xs text-ink-faint">
            {isJob
              ? "Each lane = roles you want → employers hiring → people to contact."
              : "Full-width segments — use the menu on each card for actions."}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button disabled={actionsLocked} onClick={() => setCreating((value) => !value)}>
            {creating ? "Close" : isJob ? "Add search lane" : "Add audience"}
          </Button>
          <Button
            variant="primary"
            disabled={actionsLocked}
            onClick={() => {
              setPreserveProspecting(true);
              setRegenOpen(true);
            }}
          >
            Regenerate with AI
          </Button>
        </div>
      </div>

      {!searchReady && (
        <Notice tone="warn">
          Finding targets needs a search key. Set <code>SERPER_API_KEY</code> in{" "}
          <code>server/.env</code> — 2,500 queries are free.
        </Notice>
      )}

      {overview.verification && !overview.verification.allow && (
        <Notice tone="warn">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <p>
              {overview.verification.reason ??
                "Email verification is paused. Company search still works — Find next batch will use the next Google page."}
            </p>
            <Button
              disabled={actionsLocked}
              onClick={() =>
                void run(
                  () => api.resumeVerify(overview.project.id),
                  "Verify resumed — top up Reoon first if credits are empty.",
                )
              }
            >
              Resume verify
            </Button>
          </div>
        </Notice>
      )}

      {creating && (
        <Card title={isJob ? "New search lane" : "New audience"}>
          <AudienceEditor
            projectId={overview.project.id}
            workflowType={workflowType}
            initial={emptyAudience(workflowType)}
            onCancel={() => setCreating(false)}
            onSave={(draft) => {
              setCreating(false);
              void run(
                () => api.createAudience(overview.project.id, draft),
                isJob ? "Search lane added." : "Audience added.",
              );
            }}
          />
        </Card>
      )}

      {visibleAudiences.length === 0 ? (
        <Empty>
          {overview.counts.pendingJobs > 0
            ? isJob
              ? "Building job-search lanes from your resume…"
              : "Generating audiences from your campaign profile…"
            : isJob
              ? "No search lanes yet. Regenerate with AI, or add a lane by hand."
              : "No audiences yet. Regenerate with AI, or add a category by hand."}
        </Empty>
      ) : (
        <div className="space-y-4">
          {visibleAudiences.map((audience) => (
            <AudienceCard
              key={audience.id}
              projectId={overview.project.id}
              workflowType={workflowType}
              audience={audience}
              busy={busy}
              prospectingBusy={prospectingBusy}
              searchReady={searchReady}
              editing={editingId === audience.id}
              onEdit={() => setEditingId(audience.id)}
              onCancelEdit={() => setEditingId(null)}
              onSave={(draft) => {
                setEditingId(null);
                void run(
                  () => api.updateAudience(audience.id, draft),
                  isJob ? "Search lane updated." : "Audience updated.",
                );
              }}
              onProspect={() =>
                run(async () => {
                  const result = await api.prospect(audience.id);
                  onNotice(
                    `Google page ${result.searchPage} for “${audience.name}” (next click → page ${result.nextSearchPageAfter}). ${result.note}`,
                  );
                })
              }
              onRegenerate={() =>
                void run(
                  () => api.regenerateAudience(audience.id),
                  isJob
                    ? `Refreshing “${audience.name}” (lane + email). Watch Activity for progress.`
                    : `Refreshing “${audience.name}” (ICP + email). Watch Activity for progress.`,
                )
              }
              onRewriteEmail={() =>
                void run(
                  () => api.regenerateAudienceEmail(audience.id),
                  `Rewriting email for “${audience.name}”. Watch Activity for progress.`,
                )
              }
              onToggleEnabled={() =>
                run(() => api.updateAudience(audience.id, { enabled: !audience.enabled }))
              }
              onDelete={() => setDeleteTarget(audience)}
            />
          ))}
        </div>
      )}

      <ConfirmDialog
        open={regenOpen}
        title={isJob ? "Regenerate all search lanes?" : "Regenerate all audiences?"}
        description={
          <>
            {isJob
              ? "Rebuilds every AI search lane from your resume in one batch, then writes each email template individually. Prefer regenerating a single lane when you only want to improve one path."
              : "Rebuilds every AI audience from the campaign profile in one batch, then writes each email template individually. Prefer “Regenerate audience” / “Rewrite email” on a single card when you only want to improve one ICP."}{" "}
            You currently have {companyCount} targets and {contactCount} contacts. Contact lists
            are never rebuilt here — finding contacts stays a separate step.
          </>
        }
        confirmLabel="Regenerate"
        danger={!preserveProspecting}
        busy={actionsLocked}
        onCancel={() => setRegenOpen(false)}
        onConfirm={() => {
          void run(async () => {
            await api.regenerateAudiences(overview.project.id, { preserveProspecting });
            setRegenOpen(false);
          }, preserveProspecting
            ? `Regenerating ${isJob ? "search lanes" : "audiences"}. Previous contacts and messages will be kept.`
            : `Regenerating ${isJob ? "search lanes" : "audiences"}. Previous prospecting data for generated items will be deleted.`);
        }}
      >
        <WhatHappensList {...regenWhatHappens(preserveProspecting)} />
        <CheckboxRow
          checked={preserveProspecting}
          onChange={setPreserveProspecting}
          label="Keep existing targets, contacts & emails"
          hint={`On by default. Old AI ${isJob ? "lanes" : "audiences"} with data stay (disabled as “kept”). Turn off only for a clean slate.`}
        />
        {!preserveProspecting && (
          <Notice tone="bad">
            Permanent delete: targets, contacts, and draft/sent emails tied to AI-generated
            {isJob ? " lanes" : " audiences"} will be removed.
          </Notice>
        )}
      </ConfirmDialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        title={
          deleteTarget
            ? `Delete “${deleteTarget.name}”?`
            : isJob
              ? "Delete search lane?"
              : "Delete audience?"
        }
        description={`This permanently removes the ${unit} and, via cascade, any targets, contacts, and draft/sent emails tied to it.`}
        confirmLabel={isJob ? "Delete search lane" : "Delete audience"}
        danger
        busy={actionsLocked}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          const id = deleteTarget.id;
          void run(async () => {
            await api.deleteAudience(id);
            setDeleteTarget(null);
          }, "Audience removed.");
        }}
      />
    </div>
  );
}
