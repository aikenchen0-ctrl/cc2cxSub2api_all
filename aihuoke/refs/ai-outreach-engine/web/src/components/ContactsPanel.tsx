import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { api, type Audience, type Contact, type WorkflowType } from "../lib/api";
import { Button, Card, ConfirmDialog, Empty, Field, Pill, inputClass } from "./ui";

const VERIFY_TONE: Record<string, "good" | "warn" | "bad" | "neutral"> = {
  valid: "good",
  risky: "warn",
  unknown: "warn",
  invalid: "bad",
  unverified: "neutral",
};

const SOURCE_LABEL: Record<string, string> = {
  scraped: "found on site",
  scraped_generic: "generic inbox",
  pattern: "pattern + verified",
  manual: "added manually",
  imported: "imported",
};

const PLAYBOOK_LABEL: Record<WorkflowType, string> = {
  customer_outreach: "Customers",
  investor_outreach: "Investors",
  job_outreach: "Jobs",
};

type ContactScope = "project" | "workflow" | "all";

type ManualForm = {
  email: string;
  fullName: string;
  title: string;
  companyName: string;
  companyDomain: string;
  evidenceUrl: string;
  audienceId: string;
};

const EMPTY_FORM: ManualForm = {
  email: "",
  fullName: "",
  title: "",
  companyName: "",
  companyDomain: "",
  evidenceUrl: "",
  audienceId: "",
};

function mergeContacts(current: Contact[], incoming: Contact[]): Contact[] {
  if (current.length === 0) return incoming;
  const byId = new Map<number, Contact>();
  for (const contact of current) byId.set(contact.id, contact);
  for (const contact of incoming) byId.set(contact.id, contact);
  return [...byId.values()].sort((a, b) => b.id - a.id);
}

function hasEvidenceLink(url: string): boolean {
  return Boolean(url) && url !== "manual" && url !== "imported" && url !== "import";
}

function hasJobLink(url: string | undefined): boolean {
  if (!url) return false;
  if (!hasEvidenceLink(url)) return false;
  return /^https?:\/\//i.test(url);
}

function contactQueryOptions(
  scope: ContactScope,
  projectId: number | null,
  workflowType: WorkflowType,
  query: string,
): { projectId?: number; workflowType?: WorkflowType; q?: string } {
  return {
    ...(scope === "project" && projectId !== null ? { projectId } : {}),
    ...(scope === "workflow" ? { workflowType } : {}),
    ...(query.trim() ? { q: query.trim() } : {}),
  };
}

function formFromContact(contact: Contact): ManualForm {
  return {
    email: contact.email,
    fullName: contact.full_name === "Contact" ? "" : contact.full_name,
    title: contact.title,
    companyName: contact.company_name ?? "",
    companyDomain: contact.company_domain ?? "",
    evidenceUrl: hasEvidenceLink(contact.evidence_url) ? contact.evidence_url : "",
    audienceId:
      contact.audience_name === "Manual contacts" ? "" : String(contact.audience_id ?? ""),
  };
}

function MoreIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden>
      <circle cx="3.5" cy="8" r="1.35" />
      <circle cx="8" cy="8" r="1.35" />
      <circle cx="12.5" cy="8" r="1.35" />
    </svg>
  );
}

function ContactMenu({
  onDraftAgain,
  onEdit,
  onDelete,
}: {
  onDraftAgain: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [coords, setCoords] = useState<{ top: number; right: number } | null>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    if (!open || !buttonRef.current) {
      setCoords(null);
      return;
    }
    const place = () => {
      const rect = buttonRef.current!.getBoundingClientRect();
      setCoords({
        top: rect.bottom + 4,
        right: window.innerWidth - rect.right,
      });
    };
    place();
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    return () => {
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", place, true);
    };
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onPointer = (event: MouseEvent) => {
      const target = event.target as Node;
      if (buttonRef.current?.contains(target) || menuRef.current?.contains(target)) return;
      setOpen(false);
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    window.addEventListener("mousedown", onPointer);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("mousedown", onPointer);
      window.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <>
      <button
        ref={buttonRef}
        type="button"
        title="Contact actions"
        aria-label="Contact actions"
        aria-haspopup="menu"
        aria-expanded={open}
        className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-ink-faint transition-colors hover:bg-surface-2 hover:text-ink"
        onClick={() => setOpen((value) => !value)}
      >
        <MoreIcon />
      </button>
      {open &&
        coords &&
        createPortal(
          <div
            ref={menuRef}
            role="menu"
            style={{ top: coords.top, right: coords.right }}
            className="fixed z-[1100] min-w-36 overflow-hidden rounded-xl border border-line bg-surface py-1 shadow-[var(--shadow-lift)]"
          >
            <button
              type="button"
              role="menuitem"
              className="block w-full px-3.5 py-2 text-left text-[13px] font-medium text-ink hover:bg-surface-2"
              onClick={() => {
                setOpen(false);
                onDraftAgain();
              }}
            >
              Draft email again
            </button>
            <button
              type="button"
              role="menuitem"
              className="block w-full px-3.5 py-2 text-left text-[13px] font-medium text-ink hover:bg-surface-2"
              onClick={() => {
                setOpen(false);
                onEdit();
              }}
            >
              Edit
            </button>
            <button
              type="button"
              role="menuitem"
              className="block w-full px-3.5 py-2 text-left text-[13px] font-medium text-bad hover:bg-bad-soft"
              onClick={() => {
                setOpen(false);
                onDelete();
              }}
            >
              Delete
            </button>
          </div>,
          document.body,
        )}
    </>
  );
}

function ContactFormFields({
  form,
  setForm,
  audiences,
  autoFocusEmail,
}: {
  form: ManualForm;
  setForm: (updater: (current: ManualForm) => ManualForm) => void;
  audiences: Audience[];
  autoFocusEmail?: boolean;
}) {
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <Field label="Email" hint="Required. Company domain is inferred if you leave it blank.">
        <input
          className={inputClass}
          type="email"
          autoFocus={autoFocusEmail}
          placeholder="jane@acme.com"
          value={form.email}
          onChange={(event) => setForm((current) => ({ ...current, email: event.target.value }))}
        />
      </Field>
      <Field label="Full name">
        <input
          className={inputClass}
          placeholder="Jane Doe"
          value={form.fullName}
          onChange={(event) => setForm((current) => ({ ...current, fullName: event.target.value }))}
        />
      </Field>
      <Field label="Title">
        <input
          className={inputClass}
          placeholder="Head of Growth"
          value={form.title}
          onChange={(event) => setForm((current) => ({ ...current, title: event.target.value }))}
        />
      </Field>
      <Field
        label="Audience"
        hint="Which ICP this contact belongs to. Used for templates and personalisation."
      >
        <select
          className={inputClass}
          value={form.audienceId}
          onChange={(event) =>
            setForm((current) => ({ ...current, audienceId: event.target.value }))
          }
        >
          <option value="">Manual contacts</option>
          {audiences
            .filter((audience) => audience.name !== "Manual contacts")
            .map((audience) => (
              <option key={audience.id} value={audience.id}>
                {audience.name}
              </option>
            ))}
        </select>
      </Field>
      <Field label="Organization name">
        <input
          className={inputClass}
          placeholder="Acme"
          value={form.companyName}
          onChange={(event) =>
            setForm((current) => ({ ...current, companyName: event.target.value }))
          }
        />
      </Field>
      <Field label="Organization domain" hint="Optional. Taken from the email when empty.">
        <input
          className={inputClass}
          placeholder="acme.com"
          value={form.companyDomain}
          onChange={(event) =>
            setForm((current) => ({ ...current, companyDomain: event.target.value }))
          }
        />
      </Field>
      <Field
        label="Evidence URL"
        hint="Optional LinkedIn or public page that shows why this person is a fit (useful for compliance)."
      >
        <input
          className={inputClass}
          type="url"
          placeholder="https://linkedin.com/in/…"
          value={form.evidenceUrl}
          onChange={(event) =>
            setForm((current) => ({ ...current, evidenceUrl: event.target.value }))
          }
        />
      </Field>
    </div>
  );
}

const SCOPE_COPY: Record<
  ContactScope,
  { label: string; description: (playbook: string, campaign: string | null) => string }
> = {
  project: {
    label: "This campaign",
    description: (_playbook, campaign) =>
      campaign
        ? `Contacts owned by “${campaign}”. Add/import go into this campaign only.`
        : "Contacts owned by the campaign selected in the sidebar. Add/import go here only.",
  },
  workflow: {
    label: "This playbook",
    description: (playbook) =>
      `All contacts from every ${playbook} campaign. Export this list; switch to This campaign to add or import.`,
  },
  all: {
    label: "All playbooks",
    description: () =>
      "Contacts from every playbook (Customers, Investors, Jobs). Export this list; add/import stay campaign-specific.",
  },
};

export function ContactsPanel({
  projectId,
  campaignName,
  activeWorkflowType,
  audiences,
  refreshKey,
  onRefresh,
  onError,
  onNotice,
}: {
  projectId: number | null;
  campaignName: string | null;
  activeWorkflowType: WorkflowType;
  audiences: Audience[];
  refreshKey: number;
  onRefresh: () => void;
  onError: (message: string) => void;
  onNotice: (message: string) => void;
}) {
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [total, setTotal] = useState(0);
  const [nextBeforeId, setNextBeforeId] = useState<number | null>(null);
  const [scope, setScope] = useState<ContactScope>("project");
  const [query, setQuery] = useState("");
  const [loadingInitial, setLoadingInitial] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<Contact | null>(null);
  const [audiencesByProject, setAudiencesByProject] = useState<Record<number, Audience[]>>({});
  const [importing, setImporting] = useState(false);
  const [form, setForm] = useState<ManualForm>(EMPTY_FORM);
  const [csvText, setCsvText] = useState("");
  const [importAudienceId, setImportAudienceId] = useState("");
  const [busy, setBusy] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Contact | null>(null);
  const hasLoadedMoreRef = useRef(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const activeScope: ContactScope = scope === "project" && projectId === null ? "workflow" : scope;

  useEffect(() => {
    hasLoadedMoreRef.current = false;
    setContacts([]);
    setTotal(0);
    setNextBeforeId(null);
    setAdding(false);
    setEditing(null);
    setImporting(false);
    setForm(EMPTY_FORM);
    setCsvText("");
    setImportAudienceId("");
  }, [projectId, activeWorkflowType, activeScope, query]);

  useEffect(() => {
    let cancelled = false;
    setLoadingInitial(true);
    const timer = window.setTimeout(() => {
      const opts = contactQueryOptions(activeScope, projectId, activeWorkflowType, query);
      void api
        .contacts({ ...opts, limit: 50 })
        .then((payload) => {
          if (cancelled) return;
          setTotal(payload.total);
          setContacts(payload.contacts);
          setNextBeforeId(payload.nextBeforeId);
        })
        .catch((error) => {
          if (!cancelled) {
            onError(error instanceof Error ? error.message : "Failed to load contacts");
          }
        })
        .finally(() => {
          if (!cancelled) setLoadingInitial(false);
        });
    }, query.trim() ? 250 : 0);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [projectId, activeWorkflowType, activeScope, query, refreshKey, onError]);

  function closeEditors() {
    setAdding(false);
    setEditing(null);
    setForm(EMPTY_FORM);
  }

  function startAdd() {
    setImporting(false);
    setEditing(null);
    setForm(EMPTY_FORM);
    setAdding((open) => !open);
  }

  async function startEdit(contact: Contact) {
    setAdding(false);
    setImporting(false);
    setEditing(contact);
    setForm(formFromContact(contact));
    if (contact.project_id !== projectId && audiencesByProject[contact.project_id] === undefined) {
      try {
        const payload = await api.listAudiences(contact.project_id);
        setAudiencesByProject((current) => ({
          ...current,
          [contact.project_id]: payload.audiences,
        }));
      } catch (error) {
        onError(error instanceof Error ? error.message : "Failed to load contact audiences");
      }
    }
  }

  async function loadMore() {
    if (nextBeforeId === null) return;
    setLoadingMore(true);
    try {
      const opts = contactQueryOptions(activeScope, projectId, activeWorkflowType, query);
      const payload = await api.contacts({ ...opts, limit: 50, beforeId: nextBeforeId });
      hasLoadedMoreRef.current = true;
      setContacts((current) => mergeContacts(current, payload.contacts));
      setTotal(payload.total);
      setNextBeforeId(payload.nextBeforeId);
    } catch (error) {
      onError(error instanceof Error ? error.message : "Failed to load more contacts");
    } finally {
      setLoadingMore(false);
    }
  }

  async function submitManual() {
    if (projectId === null || !form.email.trim()) return;
    setBusy(true);
    try {
      const body = {
        email: form.email.trim(),
        fullName: form.fullName.trim() || undefined,
        title: form.title.trim() || undefined,
        companyName: form.companyName.trim() || undefined,
        companyDomain: form.companyDomain.trim() || undefined,
        evidenceUrl: form.evidenceUrl.trim() || undefined,
        audienceId: form.audienceId ? Number(form.audienceId) : undefined,
      };
      const { contact } = await api.createContact(projectId, body);
      setContacts((current) => mergeContacts(current, [contact]));
      setTotal((value) => value + 1);
      closeEditors();
      onNotice("Contact saved. Open Review & send to see the drafted email.");
      onRefresh();
    } catch (error) {
      onError(error instanceof Error ? error.message : "Failed to add contact");
    } finally {
      setBusy(false);
    }
  }

  async function submitEdit() {
    if (!editing || !form.email.trim()) return;
    setBusy(true);
    try {
      const { contact } = await api.updateContact(editing.id, {
        email: form.email.trim(),
        fullName: form.fullName.trim() || undefined,
        title: form.title.trim(),
        companyName: form.companyName.trim() || undefined,
        companyDomain: form.companyDomain.trim() || undefined,
        evidenceUrl: form.evidenceUrl.trim(),
        audienceId: form.audienceId ? Number(form.audienceId) : null,
      });
      setContacts((current) => mergeContacts(current, [contact]));
      closeEditors();
      onNotice("Contact updated.");
      onRefresh();
    } catch (error) {
      onError(error instanceof Error ? error.message : "Failed to update contact");
    } finally {
      setBusy(false);
    }
  }

  async function submitImport() {
    if (projectId === null || !csvText.trim()) return;
    setBusy(true);
    try {
      const result = await api.importContacts(projectId, {
        csv: csvText,
        audienceId: importAudienceId ? Number(importAudienceId) : undefined,
      });
      if (result.contacts.length > 0) {
        setContacts((current) => mergeContacts(current, result.contacts));
      }
      setTotal((value) => value + result.created);
      setCsvText("");
      setImporting(false);
      const parts = [
        `Imported ${result.created}`,
        result.skipped ? `${result.skipped} already present` : null,
        result.errors.length ? `${result.errors.length} failed` : null,
      ].filter(Boolean);
      onNotice(
        `${parts.join(" · ")}.${
          result.created > 0 ? " Drafts will appear under Review & send shortly." : ""
        }`,
      );
      if (result.errors.length > 0) {
        onError(
          result.errors
            .slice(0, 3)
            .map((entry) => `${entry.email}: ${entry.reason}`)
            .join("; "),
        );
      }
      onRefresh();
    } catch (error) {
      onError(error instanceof Error ? error.message : "Failed to import contacts");
    } finally {
      setBusy(false);
    }
  }

  async function draftAgain(contact: Contact) {
    setBusy(true);
    try {
      await api.draftContactAgain(contact.id);
      onNotice("Draft queued. Open Review & send to edit and approve it.");
      onRefresh();
    } catch (error) {
      onError(error instanceof Error ? error.message : "Failed to queue draft");
    } finally {
      setBusy(false);
    }
  }

  function onPickCsv(file: File | null) {
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      setCsvText(typeof reader.result === "string" ? reader.result : "");
      setImporting(true);
    };
    reader.onerror = () => onError("Could not read that CSV file");
    reader.readAsText(file);
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    setBusy(true);
    try {
      const id = deleteTarget.id;
      await api.deleteContact(id);
      setContacts((current) => current.filter((contact) => contact.id !== id));
      setTotal((value) => Math.max(0, value - 1));
      if (editing?.id === id) closeEditors();
      setDeleteTarget(null);
      onNotice("Contact deleted.");
      onRefresh();
    } catch (error) {
      onError(error instanceof Error ? error.message : "Failed to delete contact");
    } finally {
      setBusy(false);
    }
  }

  const remaining = Math.max(total - contacts.length, 0);
  const audienceOptions =
    editing && editing.project_id !== projectId
      ? (audiencesByProject[editing.project_id] ?? [])
      : audiences;
  const canMutateCampaign = activeScope === "project" && projectId !== null;
  const playbookLabel = PLAYBOOK_LABEL[activeWorkflowType];
  const scopeMeta = SCOPE_COPY[activeScope];
  const scopeDescription = scopeMeta.description(playbookLabel, campaignName);
  const exportOpts = contactQueryOptions(activeScope, projectId, activeWorkflowType, query);

  return (
    <div className="space-y-4">
      <Card
        title={`${total} contact${total === 1 ? "" : "s"}`}
        action={
          <div className="flex flex-wrap items-center justify-end gap-2">
            {canMutateCampaign && (
              <>
                <Button variant={adding ? "primary" : "default"} onClick={startAdd}>
                  {adding ? "Close" : "Add contact"}
                </Button>
                <Button
                  variant={importing ? "primary" : "default"}
                  onClick={() => {
                    setImporting((open) => !open);
                    closeEditors();
                  }}
                >
                  {importing ? "Close" : "Import CSV"}
                </Button>
              </>
            )}
            <input
              ref={fileRef}
              type="file"
              accept=".csv,text/csv"
              className="hidden"
              onChange={(event) => {
                onPickCsv(event.target.files?.[0] ?? null);
                event.target.value = "";
              }}
            />
            {total > 0 && (
              <Button
                onClick={() => {
                  window.location.href = api.exportContactsCsvUrl(exportOpts);
                }}
                title="Download contacts in the current scope as UTF-8 CSV"
              >
                Export CSV
              </Button>
            )}
          </div>
        }
      >
        <div className="mb-4 space-y-3">
          <div className="flex flex-wrap gap-2">
            {(
              [
                { id: "project" as const, label: SCOPE_COPY.project.label, disabled: projectId === null },
                {
                  id: "workflow" as const,
                  label: `${playbookLabel} playbook`,
                  disabled: false,
                },
                { id: "all" as const, label: SCOPE_COPY.all.label, disabled: false },
              ] as const
            ).map((option) => (
              <button
                key={option.id}
                type="button"
                disabled={option.disabled}
                onClick={() => setScope(option.id)}
                className={`rounded-full px-3 py-1.5 text-xs font-bold transition-colors ${
                  activeScope === option.id
                    ? "bg-accent text-white"
                    : "bg-surface-2 text-ink-soft hover:text-ink"
                } ${option.disabled ? "cursor-not-allowed opacity-45" : ""}`}
              >
                {option.label}
              </button>
            ))}
          </div>
          <p className="text-xs leading-relaxed text-ink-faint">{scopeDescription}</p>
          <input
            className={inputClass}
            placeholder="Search by email, name, title, company, audience, or campaign"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
          <p className="text-[11px] font-medium text-ink-faint">
            Showing {contacts.length}
            {total > contacts.length ? ` of ${total}` : ""}
            {remaining > 0 ? ` · ${remaining} more` : ""}
            {query.trim() ? " · filtered by search" : ""}
          </p>
        </div>
        {loadingInitial && contacts.length === 0 ? (
          <Empty>Loading contacts...</Empty>
        ) : contacts.length === 0 && !adding && !importing && !editing ? (
          <Empty>
            {canMutateCampaign
              ? "No contacts in this campaign yet. Add one, import a CSV, or run “Find targets” on Audiences."
              : "No contacts in this scope yet. Switch to a campaign to add or import contacts."}
          </Empty>
        ) : contacts.length === 0 ? null : (
          <>
            <div className="space-y-3 md:hidden">
              {contacts.map((contact) => (
                <div
                  key={contact.id}
                  className="rounded-2xl border border-line bg-surface-2/50 p-4"
                >
                  <div className="flex items-start gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="font-mono text-[13px] font-medium break-all">
                        {contact.email}
                      </div>
                      <p className="mt-1.5 text-xs text-ink-soft">
                        {[contact.full_name, contact.title, contact.company_name]
                          .filter(Boolean)
                          .join(" · ") || "—"}
                      </p>
                      {activeScope !== "project" && (
                        <p className="mt-1 text-[11px] font-medium text-ink-faint">
                          {contact.project_name || `Campaign #${contact.project_id}`} ·{" "}
                          {PLAYBOOK_LABEL[contact.workflow_type]}
                        </p>
                      )}
                    </div>
                    <ContactMenu
                      onDraftAgain={() => void draftAgain(contact)}
                      onEdit={() => void startEdit(contact)}
                      onDelete={() => setDeleteTarget(contact)}
                    />
                  </div>
                  <div className="mt-3 flex flex-wrap items-center gap-2">
                    <Pill>{SOURCE_LABEL[contact.email_source] ?? contact.email_source}</Pill>
                    <Pill tone={VERIFY_TONE[contact.verify_status] ?? "neutral"}>
                      {contact.verify_status}
                    </Pill>
                    {hasJobLink(contact.company_source_url) && (
                      <a
                        href={contact.company_source_url}
                        target="_blank"
                        rel="noreferrer"
                        className="text-xs font-semibold text-accent hover:underline"
                      >
                        job / listing
                      </a>
                    )}
                    {hasEvidenceLink(contact.evidence_url) && (
                      <a
                        href={contact.evidence_url}
                        target="_blank"
                        rel="noreferrer"
                        className="text-xs font-semibold text-accent hover:underline"
                      >
                        profile
                      </a>
                    )}
                  </div>
                </div>
              ))}
            </div>

            <div className="-mx-5 hidden overflow-x-auto sm:-mx-6 md:block">
              <table className="w-full table-fixed text-[13px]">
                <colgroup>
                  <col className="w-[24%]" />
                  <col className="w-[14%]" />
                  <col className="w-[14%]" />
                  <col className="w-[12%]" />
                  <col className="w-[10%]" />
                  <col className="w-[12%]" />
                  <col className="w-[10%]" />
                  <col className="w-[4%]" />
                </colgroup>
                <thead>
                  <tr className="border-b border-line text-left text-[11px] font-semibold tracking-[0.08em] text-ink-faint uppercase">
                    <th className="px-5 py-2.5 font-semibold sm:px-6">Email</th>
                    <th className="px-3 py-2.5 font-semibold">Name</th>
                    <th className="px-3 py-2.5 font-semibold">Title</th>
                    <th className="px-3 py-2.5 font-semibold">Source</th>
                    <th className="px-3 py-2.5 font-semibold">Verified</th>
                    <th className="px-2 py-2.5 font-semibold">Job</th>
                    <th className="px-2 py-2.5 font-semibold">Profile</th>
                    <th className="px-2 py-2.5 sm:px-3" />
                  </tr>
                </thead>
                <tbody>
                  {contacts.map((contact) => (
                    <tr
                      key={contact.id}
                      className="border-b border-line/70 transition-colors last:border-0 hover:bg-surface-2/60"
                    >
                      <td
                        className="truncate px-5 py-3 font-mono text-[13px] font-medium sm:px-6"
                        title={contact.email}
                      >
                        <div className="truncate">{contact.email}</div>
                        {activeScope !== "project" && (
                          <div className="mt-1 truncate font-sans text-[11px] font-medium text-ink-faint">
                            {contact.project_name || `Campaign #${contact.project_id}`} ·{" "}
                            {PLAYBOOK_LABEL[contact.workflow_type]}
                          </div>
                        )}
                      </td>
                      <td
                        className="truncate px-3 py-3 text-ink-soft"
                        title={contact.full_name || undefined}
                      >
                        {contact.full_name || "—"}
                      </td>
                      <td
                        className="truncate px-3 py-3 text-ink-soft"
                        title={contact.title || undefined}
                      >
                        {contact.title || "—"}
                      </td>
                      <td className="px-3 py-3">
                        <Pill>{SOURCE_LABEL[contact.email_source] ?? contact.email_source}</Pill>
                      </td>
                      <td className="px-3 py-3">
                        <Pill tone={VERIFY_TONE[contact.verify_status] ?? "neutral"}>
                          {contact.verify_status}
                        </Pill>
                      </td>
                      <td className="px-2 py-3">
                        {hasJobLink(contact.company_source_url) ? (
                          <a
                            href={contact.company_source_url}
                            target="_blank"
                            rel="noreferrer"
                            className="text-xs font-semibold text-accent hover:underline"
                            title={contact.company_source_url}
                          >
                            listing
                          </a>
                        ) : contact.company_domain ? (
                          <a
                            href={`https://${contact.company_domain}`}
                            target="_blank"
                            rel="noreferrer"
                            className="text-xs font-semibold text-accent hover:underline"
                            title={contact.company_domain}
                          >
                            site
                          </a>
                        ) : (
                          <span className="text-sm text-ink-faint">—</span>
                        )}
                      </td>
                      <td className="px-2 py-3">
                        {hasEvidenceLink(contact.evidence_url) ? (
                          <a
                            href={contact.evidence_url}
                            target="_blank"
                            rel="noreferrer"
                            className="text-xs font-semibold text-accent hover:underline"
                          >
                            profile
                          </a>
                        ) : (
                          <span className="text-sm text-ink-faint">—</span>
                        )}
                      </td>
                      <td className="px-2 py-2 text-right sm:px-3">
                        <ContactMenu
                          onDraftAgain={() => void draftAgain(contact)}
                          onEdit={() => void startEdit(contact)}
                          onDelete={() => setDeleteTarget(contact)}
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {nextBeforeId !== null && (
              <div className="mt-4 flex items-center justify-between gap-3 border-t border-line pt-4">
                <p className="text-xs text-ink-faint">
                  {remaining} older contact{remaining === 1 ? "" : "s"} still available
                </p>
                <Button disabled={loadingMore} onClick={() => void loadMore()}>
                  {loadingMore ? "Loading…" : "Load more"}
                </Button>
              </div>
            )}
          </>
        )}
      </Card>

      {adding && canMutateCampaign && (
        <Card title="Add contact">
          <p className="mb-4 text-sm text-ink-soft">
            Saves into{" "}
            <span className="font-medium text-ink">
              {campaignName || "the selected campaign"}
            </span>
            , then queues a draft under{" "}
            <span className="font-medium text-ink">Review & send</span>.
          </p>
          <ContactFormFields
            form={form}
            setForm={setForm}
            audiences={audienceOptions}
            autoFocusEmail
          />
          <div className="mt-4 flex flex-wrap items-center gap-2">
            <Button
              variant="primary"
              disabled={busy || !form.email.trim()}
              onClick={() => void submitManual()}
            >
              {busy ? "Saving…" : "Save contact"}
            </Button>
            <Button disabled={busy} onClick={closeEditors}>
              Cancel
            </Button>
            <span className="text-xs text-ink-faint">Draft appears in Review & send</span>
          </div>
        </Card>
      )}

      {editing && (
        <Card title={`Edit ${editing.email}`}>
          <p className="mb-4 text-sm text-ink-soft">
            This contact belongs to{" "}
            <span className="font-medium text-ink">
              {editing.project_name || `Campaign #${editing.project_id}`}
            </span>
            . Audience choices are loaded from that campaign.
          </p>
          <ContactFormFields form={form} setForm={setForm} audiences={audienceOptions} />
          <div className="mt-4 flex flex-wrap items-center gap-2">
            <Button
              variant="primary"
              disabled={busy || !form.email.trim()}
              onClick={() => void submitEdit()}
            >
              {busy ? "Saving…" : "Save changes"}
            </Button>
            <Button disabled={busy} onClick={closeEditors}>
              Cancel
            </Button>
          </div>
        </Card>
      )}

      {importing && canMutateCampaign && (
        <Card title="Import contacts">
          <p className="mb-4 text-sm text-ink-soft">
            Imports into{" "}
            <span className="font-medium text-ink">
              {campaignName || "the selected campaign"}
            </span>
            {" "}only — not the whole playbook. CSV needs a header row with at least{" "}
            <code className="rounded-md bg-surface-2 px-1.5 py-0.5 text-[12px]">email</code>.
            Optional:{" "}
            <code className="rounded-md bg-surface-2 px-1.5 py-0.5 text-[12px]">full_name</code>,{" "}
            <code className="rounded-md bg-surface-2 px-1.5 py-0.5 text-[12px]">title</code>,{" "}
            <code className="rounded-md bg-surface-2 px-1.5 py-0.5 text-[12px]">company_name</code>,{" "}
            <code className="rounded-md bg-surface-2 px-1.5 py-0.5 text-[12px]">company_domain</code>.
            Each new contact queues a draft under Review & send.
          </p>
          <div className="mb-4 grid gap-4 sm:grid-cols-2">
            <Field
              label="Audience"
              hint="Defaults to Manual contacts. Pick another if these people belong to an existing ICP."
            >
              <select
                className={inputClass}
                value={importAudienceId}
                onChange={(event) => setImportAudienceId(event.target.value)}
              >
                <option value="">Manual contacts</option>
                {audienceOptions
                  .filter((audience) => audience.name !== "Manual contacts")
                  .map((audience) => (
                    <option key={audience.id} value={audience.id}>
                      {audience.name}
                    </option>
                  ))}
              </select>
            </Field>
            <Field label="Or choose a file">
              <Button onClick={() => fileRef.current?.click()}>Choose CSV…</Button>
            </Field>
          </div>
          <Field label="CSV">
            <textarea
              className={`${inputClass} h-44 resize-y font-mono text-xs`}
              placeholder={
                "email,full_name,title,company_name,company_domain\njane@acme.com,Jane Doe,CEO,Acme,acme.com"
              }
              value={csvText}
              onChange={(event) => setCsvText(event.target.value)}
            />
          </Field>
          <div className="mt-4 flex flex-wrap gap-2">
            <Button
              variant="primary"
              disabled={busy || !csvText.trim()}
              onClick={() => void submitImport()}
            >
              {busy ? "Importing…" : "Import & draft emails"}
            </Button>
            <Button disabled={busy} onClick={() => setImporting(false)}>
              Cancel
            </Button>
          </div>
        </Card>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title={deleteTarget ? `Delete ${deleteTarget.email}?` : "Delete contact?"}
        description="This permanently removes the contact and any draft or sent emails tied to them."
        confirmLabel="Delete contact"
        danger
        busy={busy}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => void confirmDelete()}
      />
    </div>
  );
}
