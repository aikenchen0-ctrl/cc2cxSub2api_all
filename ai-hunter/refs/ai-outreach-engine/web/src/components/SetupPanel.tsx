import { useEffect, useRef, useState } from "react";
import {
  api,
  type Health,
  type Overview,
  type PlaybookPreferences,
  type WorkflowType,
} from "../lib/api";
import {
  CUSTOMER_DEFAULT_MARKETS,
  CUSTOMER_DEFAULT_SIGNALS,
} from "../lib/customerDefaults";
import { AnalysisEditor, type AnalysisEditorHandle } from "./AnalysisEditor";
import { OverviewStats } from "./OverviewStats";
import { isPipelineBusy, SETUP_STAGES } from "../lib/pipelineBusy";
import { PipelineAttentionBar } from "./PipelineAttentionBar";
import { regenWhatHappens, rescanWhatHappens } from "../lib/confirmCopy";
import {
  Button,
  Card,
  CheckboxRow,
  ConfirmDialog,
  Empty,
  Notice,
  Pill,
  WhatHappensList,
} from "./ui";

function joinList(values: string[]): string {
  return values.join(", ");
}

function splitList(raw: string): string[] {
  return raw
    .split(/\n|,/)
    .map((part) => part.replace(/^priority\s*\d+\s*[:.\-]?\s*/i, "").trim())
    .filter((part) => part && !/^priority\s*\d+$/i.test(part));
}

type PrefsDraft = {
  targetMarkets: string;
  targetCompanySignals: string;
  crawlMaxPages: string;
  investorTypes: string;
  thesisSignals: string;
  companySizes: string;
  industries: string;
  maxContacts: string;
  localizeEmails: boolean;
};

function emptyPrefsDraft(
  workflowType: WorkflowType,
  defaults?: { markets: string[]; signals: string[] },
): PrefsDraft {
  const markets = defaults?.markets?.length ? defaults.markets : CUSTOMER_DEFAULT_MARKETS;
  const signals = defaults?.signals?.length ? defaults.signals : CUSTOMER_DEFAULT_SIGNALS;
  return {
    targetMarkets: workflowType === "customer_outreach" ? joinList(markets) : "",
    targetCompanySignals: workflowType === "customer_outreach" ? joinList(signals) : "",
    crawlMaxPages: "15",
    investorTypes: "",
    thesisSignals: "",
    companySizes: "",
    industries: "",
    maxContacts: workflowType === "job_outreach" ? "5" : "10",
    localizeEmails: true,
  };
}

function draftFromPreferences(prefs: PlaybookPreferences): PrefsDraft {
  const base = emptyPrefsDraft(prefs.kind);
  if (prefs.kind === "customer_outreach") {
    return {
      ...base,
      targetMarkets: joinList(prefs.targetMarkets),
      targetCompanySignals: joinList(prefs.targetCompanySignals),
      crawlMaxPages: String(prefs.crawlMaxPages),
      maxContacts: String(prefs.maxContactsPerCompany),
      localizeEmails: prefs.localizeEmails,
    };
  }
  if (prefs.kind === "investor_outreach") {
    return {
      ...base,
      investorTypes: joinList(prefs.investorTypes),
      thesisSignals: joinList(prefs.thesisSignals),
      maxContacts: String(prefs.maxContactsPerFirm),
      localizeEmails: prefs.localizeEmails,
    };
  }
  return {
    ...base,
    companySizes: joinList(prefs.companySizes),
    industries: joinList(prefs.industries),
    maxContacts: String(prefs.maxContactsPerCompany),
    localizeEmails: prefs.localizeEmails,
  };
}

function preferencesPayload(workflowType: WorkflowType, draft: PrefsDraft): Record<string, unknown> {
  if (workflowType === "investor_outreach") {
    return {
      investorTypes: splitList(draft.investorTypes),
      thesisSignals: splitList(draft.thesisSignals),
      maxContactsPerFirm: Number(draft.maxContacts) || 10,
      localizeEmails: draft.localizeEmails,
    };
  }
  if (workflowType === "job_outreach") {
    return {
      companySizes: splitList(draft.companySizes),
      industries: splitList(draft.industries),
      maxContactsPerCompany: Number(draft.maxContacts) || 5,
      localizeEmails: draft.localizeEmails,
    };
  }
  return {
    targetMarkets: splitList(draft.targetMarkets),
    targetCompanySignals: splitList(draft.targetCompanySignals),
    maxContactsPerCompany: Number(draft.maxContacts) || 10,
    crawlMaxPages: Number(draft.crawlMaxPages) || 15,
    localizeEmails: draft.localizeEmails,
  };
}

const WORKFLOWS: Record<
  WorkflowType,
  { label: string; short: string; description: string; cta: string }
> = {
  customer_outreach: {
    label: "Find customers",
    short: "Product website -> buyers",
    description: "Read a product site, generate customer ICPs, find companies and buyer contacts.",
    cta: "Scan and build audiences",
  },
  investor_outreach: {
    label: "Reach investors",
    short: "Product website -> investors",
    description: "Use your website and raise context to find relevant funds, angels, and partners.",
    cta: "Build investor strategy",
  },
  job_outreach: {
    label: "Find jobs",
    short: "Resume -> hiring teams",
    description: "Analyse a resume, find relevant hiring companies, and draft outreach to hiring teams.",
    cta: "Build job strategy",
  },
};

const compactInputClass =
  "w-full rounded-2xl border border-white/20 bg-white/95 px-4 py-3 text-sm font-medium text-ink placeholder:text-ink-faint outline-none ring-0 focus:ring-4 focus:ring-white/30";

function linesToArray(value: string): string[] {
  return value
    .split(/\n|,/)
    .map((part) => part.trim())
    .filter(Boolean);
}

export function SetupPanel({
  overview,
  health,
  workflowType,
  workflowProjectCount,
  onWorkflowChange,
  onRefresh,
  onProjectCreated,
  onProjectDeleted,
  onError,
  onNotice,
  onOpenActivity,
}: {
  overview: Overview | null;
  health: Health | null;
  workflowType: WorkflowType;
  workflowProjectCount: number;
  onWorkflowChange: (workflowType: WorkflowType) => void;
  onRefresh: () => void;
  onProjectCreated: (projectId: number) => void;
  onProjectDeleted: (projectId: number) => void;
  onError: (message: string) => void;
  onNotice?: (message: string) => void;
  onOpenActivity: () => void;
}) {
  const [domain, setDomain] = useState("");
  const [campaignName, setCampaignName] = useState("");
  const [raiseStage, setRaiseStage] = useState("");
  const [checkSize, setCheckSize] = useState("");
  const [geography, setGeography] = useState("");
  const [sectors, setSectors] = useState("");
  const [traction, setTraction] = useState("");
  const [fundraisingNotes, setFundraisingNotes] = useState("");
  const [resumeText, setResumeText] = useState("");
  const [targetRoles, setTargetRoles] = useState("");
  const [locations, setLocations] = useState("");
  const [remotePreference, setRemotePreference] = useState("");
  const [seniority, setSeniority] = useState("");
  const [jobSearchNotes, setJobSearchNotes] = useState("");
  const [linkedinUrl, setLinkedinUrl] = useState("");
  const [resumeUrl, setResumeUrl] = useState("");
  const [prefsDraft, setPrefsDraft] = useState<PrefsDraft>(() =>
    emptyPrefsDraft(workflowType, {
      markets: health?.prospecting.targetMarkets ?? CUSTOMER_DEFAULT_MARKETS,
      signals: health?.prospecting.targetCompanySignals ?? CUSTOMER_DEFAULT_SIGNALS,
    }),
  );
  const [busy, setBusy] = useState(false);
  const [savingPlaybook, setSavingPlaybook] = useState(false);
  const [editingAnalysis, setEditingAnalysis] = useState(false);
  const [savingAnalysis, setSavingAnalysis] = useState(false);
  const [rescanOpen, setRescanOpen] = useState(false);
  const [regenOpen, setRegenOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteAcknowledged, setDeleteAcknowledged] = useState(false);
  const [alsoRegenerate, setAlsoRegenerate] = useState(false);
  const [preserveProspecting, setPreserveProspecting] = useState(true);
  const analysisEditorRef = useRef<AnalysisEditorHandle | null>(null);
  const editingSelectedCampaign =
    overview !== null && overview.project.workflow_type === workflowType;

  useEffect(() => {
    setEditingAnalysis(false);
    setSavingAnalysis(false);
  }, [overview?.project.id]);

  useEffect(() => {
    if (editingSelectedCampaign && overview?.preferences) {
      setPrefsDraft(draftFromPreferences(overview.preferences));
      try {
        const data = JSON.parse(overview.project.workflow_data || "{}") as Record<string, unknown>;
        setLinkedinUrl(typeof data.linkedinUrl === "string" ? data.linkedinUrl : "");
        setResumeUrl(typeof data.resumeUrl === "string" ? data.resumeUrl : "");
      } catch {
        setLinkedinUrl("");
        setResumeUrl("");
      }
      return;
    }
    setPrefsDraft(
      emptyPrefsDraft(workflowType, {
        markets: health?.prospecting.targetMarkets ?? CUSTOMER_DEFAULT_MARKETS,
        signals: health?.prospecting.targetCompanySignals ?? CUSTOMER_DEFAULT_SIGNALS,
      }),
    );
    // Intentionally omit health from deps — defaults match server seed; don't wipe edits mid-type.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workflowType, editingSelectedCampaign, overview?.project.id, overview?.preferences]);

  async function run(action: () => Promise<unknown>) {
    setBusy(true);
    try {
      await action();
      onRefresh();
    } catch (error) {
      onError(error instanceof Error ? error.message : "Something went wrong");
    } finally {
      setBusy(false);
    }
  }

  async function createProject() {
    setBusy(true);
    try {
      const result = await api.createProject({
        workflowType,
        domain: domain.trim() || undefined,
        name: campaignName.trim() || undefined,
        raiseStage: raiseStage.trim() || undefined,
        checkSize: checkSize.trim() || undefined,
        geography: geography.trim() || undefined,
        sectors: linesToArray(sectors),
        traction: traction.trim() || undefined,
        fundraisingNotes: fundraisingNotes.trim() || undefined,
        resumeText: resumeText.trim() || undefined,
        targetRoles: linesToArray(targetRoles),
        locations: linesToArray(locations),
        remotePreference: remotePreference.trim() || undefined,
        seniority: seniority.trim() || undefined,
        jobSearchNotes: jobSearchNotes.trim() || undefined,
        linkedinUrl: linkedinUrl.trim() || undefined,
        resumeUrl: resumeUrl.trim() || undefined,
        preferences: preferencesPayload(workflowType, prefsDraft),
      });
      onProjectCreated(result.project.id);
      onNotice?.(
        "Scanning website & building audiences — watch the live status bottom-right. Form stays locked until that finishes.",
      );
    } catch (error) {
      onError(error instanceof Error ? error.message : "Something went wrong");
    } finally {
      setBusy(false);
    }
  }

  async function deleteCampaign() {
    if (!overview) return;
    setBusy(true);
    try {
      const id = overview.project.id;
      await api.deleteProject(id);
      setDeleteOpen(false);
      setDeleteAcknowledged(false);
      onProjectDeleted(id);
      onNotice?.("Campaign deleted. Contacts, messages, targets, and queued jobs tied to it were removed.");
    } catch (error) {
      onError(error instanceof Error ? error.message : "Failed to delete campaign");
    } finally {
      setBusy(false);
    }
  }

  async function savePlaybookSettings() {
    if (!editingSelectedCampaign || !overview) return;
    setSavingPlaybook(true);
    try {
      const result = await api.updateProject(overview.project.id, {
        preferences: preferencesPayload(workflowType, prefsDraft),
        ...(workflowType === "job_outreach"
          ? {
              linkedinUrl: linkedinUrl.trim(),
              resumeUrl: resumeUrl.trim(),
            }
          : {}),
      });
      if (result.preferences) setPrefsDraft(draftFromPreferences(result.preferences));
      onRefresh();
      onNotice?.("Campaign preferences saved. Future audience/search steps will use them.");
    } catch (error) {
      onError(error instanceof Error ? error.message : "Failed to save campaign preferences");
    } finally {
      setSavingPlaybook(false);
    }
  }

  function patchPrefs<K extends keyof PrefsDraft>(key: K, value: PrefsDraft[K]) {
    setPrefsDraft((current) => ({ ...current, [key]: value }));
  }

  const analysis = overview?.analysis;
  const contactCount = overview?.counts.contacts ?? 0;
  const companyCount = overview?.counts.companies ?? 0;
  const hasProspecting = contactCount > 0 || companyCount > 0;
  const selectedWorkflow = WORKFLOWS[workflowType];
  const overviewWorkflow = overview?.project.workflow_type ?? "customer_outreach";
  const overviewWorkflowConfig = WORKFLOWS[overviewWorkflow];
  const canCreate =
    workflowType === "job_outreach" ? resumeText.trim().length >= 80 : domain.trim().length > 0;
  const setupRunning = isPipelineBusy(overview, SETUP_STAGES);
  const formLocked = busy || setupRunning;

  return (
    <div className="space-y-5">
      <Card className="overflow-hidden">
        <div className="-m-5 sm:-m-6">
          <div className="relative overflow-hidden bg-gradient-to-br from-accent via-accent to-[#5b56ff] px-5 py-7 text-white sm:px-8 sm:py-9">
            <div
              className="pointer-events-none absolute inset-0 opacity-30"
              style={{
                backgroundImage:
                  "radial-gradient(circle at 85% 20%, white 0%, transparent 35%), radial-gradient(circle at 10% 80%, white 0%, transparent 30%)",
              }}
            />
            <div className="relative grid gap-6 lg:grid-cols-[minmax(0,1.35fr)_minmax(280px,0.9fr)] lg:items-start">
              <div className="min-w-0">
                <p className="text-[10px] font-bold tracking-[0.14em] text-white/70 uppercase">
                  Create campaign
                </p>
                <h3 className="mt-2 text-xl font-bold tracking-tight sm:text-2xl">
                  Choose an outreach playbook
                </h3>
                <p className="mt-2 text-sm text-white/80">
                  One safe review-first pipeline for customers, investors, and job-search outreach.
                </p>
                <fieldset
                  disabled={formLocked}
                  className={`min-w-0 border-0 p-0 m-0 ${formLocked ? "opacity-60" : ""}`}
                >
                <div className="mt-5 grid gap-2 sm:grid-cols-3">
                  {(Object.keys(WORKFLOWS) as WorkflowType[]).map((type) => {
                    const active = workflowType === type;
                    return (
                      <button
                        key={type}
                        type="button"
                        onClick={() => onWorkflowChange(type)}
                        className={`rounded-2xl border p-3 text-left transition ${
                          active
                            ? "border-white bg-white text-accent shadow-lg"
                            : "border-white/20 bg-white/10 text-white hover:bg-white/15"
                        }`}
                      >
                        <span className="block text-[13px] font-bold">{WORKFLOWS[type].label}</span>
                        <span className={`mt-1 block text-[11px] ${active ? "text-ink-soft" : "text-white/70"}`}>
                          {WORKFLOWS[type].short}
                        </span>
                      </button>
                    );
                  })}
                </div>
                <p className="mt-3 text-xs leading-relaxed text-white/75">
                  {selectedWorkflow.description}
                  {workflowProjectCount > 0
                    ? ` Showing ${workflowProjectCount} saved campaign${workflowProjectCount === 1 ? "" : "s"} for this playbook.`
                    : " No saved campaigns in this playbook yet."}
                </p>
                {setupRunning && (
                  <p className="mt-2 text-xs font-semibold text-white">
                    Setup is running — this form is locked. Use Stop on the bottom-right toast to cancel.
                  </p>
                )}

                <div className="mt-5 space-y-3">
                  <p className="text-[11px] font-semibold tracking-wide text-white/70 uppercase">
                    Required
                  </p>
                  <div className="grid gap-3 sm:grid-cols-2">
                    <label className="block">
                      <span className="mb-1.5 block text-[11px] font-semibold tracking-wide text-white/70 uppercase">
                        Campaign name
                      </span>
                      <input
                        className={compactInputClass}
                        placeholder={
                          workflowType === "job_outreach"
                            ? "Senior frontend search"
                            : workflowType === "investor_outreach"
                              ? "Seed investor outreach"
                              : "Customer outreach"
                        }
                        value={campaignName}
                        onChange={(event) => setCampaignName(event.target.value)}
                      />
                    </label>

                    {workflowType !== "job_outreach" && (
                      <label className="block">
                        <span className="mb-1.5 block text-[11px] font-semibold tracking-wide text-white/70 uppercase">
                          Product website
                        </span>
                        <input
                          className={compactInputClass}
                          placeholder="getobserver.app"
                          value={domain}
                          onChange={(event) => setDomain(event.target.value)}
                          onKeyDown={(event) => {
                            if (event.key === "Enter" && canCreate) void createProject();
                          }}
                        />
                      </label>
                    )}
                  </div>

                  {workflowType === "investor_outreach" && (
                    <div className="space-y-3">
                      <p className="pt-1 text-[11px] font-semibold tracking-wide text-white/70 uppercase">
                        Optional raise context
                      </p>
                      <p className="text-[11px] leading-relaxed text-white/65">
                        Helps AI pick the right investor audiences. Leave blank and it infers from
                        your website.
                      </p>
                      <div className="grid gap-3 sm:grid-cols-2">
                        <input
                          className={compactInputClass}
                          placeholder="Raise stage, e.g. pre-seed / seed"
                          value={raiseStage}
                          onChange={(event) => setRaiseStage(event.target.value)}
                        />
                        <input
                          className={compactInputClass}
                          placeholder="Check size, e.g. $100k-$500k"
                          value={checkSize}
                          onChange={(event) => setCheckSize(event.target.value)}
                        />
                        <input
                          className={compactInputClass}
                          placeholder="Geography, e.g. US, India, global"
                          value={geography}
                          onChange={(event) => setGeography(event.target.value)}
                        />
                        <input
                          className={compactInputClass}
                          placeholder="Sectors, comma separated"
                          value={sectors}
                          onChange={(event) => setSectors(event.target.value)}
                        />
                        <input
                          className={`${compactInputClass} sm:col-span-2`}
                          placeholder="Traction, e.g. 12 design partners, $8k MRR, shipped MVP"
                          value={traction}
                          onChange={(event) => setTraction(event.target.value)}
                        />
                        <textarea
                          className={`${compactInputClass} min-h-20 resize-y sm:col-span-2`}
                          placeholder="Round notes or investor targeting constraints"
                          value={fundraisingNotes}
                          onChange={(event) => setFundraisingNotes(event.target.value)}
                        />
                      </div>
                    </div>
                  )}

                  {workflowType === "job_outreach" && (
                    <div className="space-y-3">
                      <label className="block">
                        <span className="mb-1.5 block text-[11px] font-semibold tracking-wide text-white/70 uppercase">
                          Resume / profile text
                        </span>
                        <textarea
                          className={`${compactInputClass} min-h-36 resize-y`}
                          placeholder="Paste resume, LinkedIn summary, or candidate profile text"
                          value={resumeText}
                          onChange={(event) => setResumeText(event.target.value)}
                        />
                      </label>
                      <p className="pt-1 text-[11px] font-semibold tracking-wide text-white/70 uppercase">
                        Optional search focus
                      </p>
                      <p className="text-[11px] leading-relaxed text-white/65">
                        Leave blank and AI will infer roles/locations from the resume.
                      </p>
                      <div className="grid gap-3 sm:grid-cols-2">
                        <input
                          className={compactInputClass}
                          placeholder="Target roles, comma separated"
                          value={targetRoles}
                          onChange={(event) => setTargetRoles(event.target.value)}
                        />
                        <input
                          className={compactInputClass}
                          placeholder="Locations, comma separated"
                          value={locations}
                          onChange={(event) => setLocations(event.target.value)}
                        />
                        <input
                          className={compactInputClass}
                          placeholder="Remote preference"
                          value={remotePreference}
                          onChange={(event) => setRemotePreference(event.target.value)}
                        />
                        <input
                          className={compactInputClass}
                          placeholder="Seniority, e.g. founding engineer"
                          value={seniority}
                          onChange={(event) => setSeniority(event.target.value)}
                        />
                      </div>
                      <textarea
                        className={`${compactInputClass} min-h-20 resize-y`}
                        placeholder="Optional notes: preferred companies, industries, visa constraints, compensation, etc."
                        value={jobSearchNotes}
                        onChange={(event) => setJobSearchNotes(event.target.value)}
                      />
                    </div>
                  )}

                  <Button
                    variant="default"
                    disabled={formLocked || !canCreate}
                    onClick={() => void createProject()}
                    className="bg-white text-accent shadow-none hover:bg-white/90"
                  >
                    {setupRunning
                      ? "Scanning…"
                      : busy
                        ? "Working…"
                        : selectedWorkflow.cta}
                  </Button>
                </div>
                </fieldset>
              </div>

              <aside className={`rounded-3xl border border-white/20 bg-black/15 p-4 backdrop-blur-sm sm:p-5 ${formLocked ? "opacity-60" : ""}`}>
                <fieldset disabled={formLocked} className="min-w-0 border-0 p-0 m-0">
                <p className="text-[10px] font-bold tracking-[0.14em] text-white/70 uppercase">
                  {editingSelectedCampaign ? "Campaign preferences" : "Preferences"}
                </p>
                <p className="mt-1 text-[11px] leading-relaxed text-white/70">
                  Optional for {WORKFLOWS[workflowType].label.toLowerCase()}. Leave blank and AI
                  fills them after setup — then edit anytime.
                </p>

                <div className="mt-4 space-y-3">
                  <label className="flex cursor-pointer gap-3 rounded-2xl border border-white/15 bg-white/10 px-3.5 py-3 text-left">
                    <input
                      type="checkbox"
                      className="mt-0.5 h-4 w-4 shrink-0 accent-white"
                      checked={prefsDraft.localizeEmails}
                      disabled={savingPlaybook}
                      onChange={(event) => patchPrefs("localizeEmails", event.target.checked)}
                    />
                    <span className="min-w-0">
                      <span className="block text-[13px] font-semibold text-white">
                        Localize emails
                      </span>
                      <span className="mt-0.5 block text-[11px] leading-relaxed text-white/70">
                        Translate drafts for non-English recipient countries when possible.
                      </span>
                    </span>
                  </label>

                  {workflowType === "customer_outreach" && (
                    <>
                      <label className="block">
                        <span className="mb-1.5 block text-[11px] font-semibold tracking-wide text-white/70 uppercase">
                          Target markets
                        </span>
                        <textarea
                          className={`${compactInputClass} min-h-[72px] resize-y`}
                          value={prefsDraft.targetMarkets}
                          disabled={savingPlaybook}
                          onChange={(event) => patchPrefs("targetMarkets", event.target.value)}
                          placeholder="United States, United Kingdom, Germany, Singapore…"
                        />
                      </label>
                      <label className="block">
                        <span className="mb-1.5 block text-[11px] font-semibold tracking-wide text-white/70 uppercase">
                          Company signals
                        </span>
                        <textarea
                          className={`${compactInputClass} min-h-[72px] resize-y`}
                          value={prefsDraft.targetCompanySignals}
                          disabled={savingPlaybook}
                          onChange={(event) =>
                            patchPrefs("targetCompanySignals", event.target.value)
                          }
                          placeholder="Recently funded, Pre-Seed, Seed, Series A, YC, Techstars…"
                        />
                      </label>
                      <div className="grid gap-3 sm:grid-cols-2">
                        <label className="block">
                          <span className="mb-1.5 block text-[11px] font-semibold tracking-wide text-white/70 uppercase">
                            Max contacts / company
                          </span>
                          <input
                            className={compactInputClass}
                            type="number"
                            min={1}
                            max={15}
                            value={prefsDraft.maxContacts}
                            disabled={savingPlaybook}
                            onChange={(event) => patchPrefs("maxContacts", event.target.value)}
                          />
                        </label>
                        <label className="block">
                          <span className="mb-1.5 block text-[11px] font-semibold tracking-wide text-white/70 uppercase">
                            Crawl max pages
                          </span>
                          <input
                            className={compactInputClass}
                            type="number"
                            min={1}
                            max={50}
                            value={prefsDraft.crawlMaxPages}
                            disabled={savingPlaybook}
                            onChange={(event) => patchPrefs("crawlMaxPages", event.target.value)}
                          />
                        </label>
                      </div>
                    </>
                  )}

                  {workflowType === "investor_outreach" && (
                    <>
                      <label className="block">
                        <span className="mb-1.5 block text-[11px] font-semibold tracking-wide text-white/70 uppercase">
                          Investor types
                        </span>
                        <textarea
                          className={`${compactInputClass} min-h-[72px] resize-y`}
                          value={prefsDraft.investorTypes}
                          disabled={savingPlaybook}
                          onChange={(event) => patchPrefs("investorTypes", event.target.value)}
                          placeholder="angels, seed funds, micro-VCs, accelerators — blank = AI suggests"
                        />
                      </label>
                      <label className="block">
                        <span className="mb-1.5 block text-[11px] font-semibold tracking-wide text-white/70 uppercase">
                          Thesis / search signals
                        </span>
                        <textarea
                          className={`${compactInputClass} min-h-[72px] resize-y`}
                          value={prefsDraft.thesisSignals}
                          disabled={savingPlaybook}
                          onChange={(event) => patchPrefs("thesisSignals", event.target.value)}
                          placeholder="B2B SaaS seed investor, AI productivity fund — blank = AI suggests"
                        />
                      </label>
                      <label className="block">
                        <span className="mb-1.5 block text-[11px] font-semibold tracking-wide text-white/70 uppercase">
                          Max contacts / firm
                        </span>
                        <input
                          className={compactInputClass}
                          type="number"
                          min={1}
                          max={10}
                          value={prefsDraft.maxContacts}
                          disabled={savingPlaybook}
                          onChange={(event) => patchPrefs("maxContacts", event.target.value)}
                        />
                      </label>
                    </>
                  )}

                  {workflowType === "job_outreach" && (
                    <>
                      <label className="block">
                        <span className="mb-1.5 block text-[11px] font-semibold tracking-wide text-white/70 uppercase">
                          Target company sizes
                        </span>
                        <textarea
                          className={`${compactInputClass} min-h-[72px] resize-y`}
                          value={prefsDraft.companySizes}
                          disabled={savingPlaybook}
                          onChange={(event) => patchPrefs("companySizes", event.target.value)}
                          placeholder="11-50, 51-200, 201-1000 — blank = AI suggests"
                        />
                      </label>
                      <label className="block">
                        <span className="mb-1.5 block text-[11px] font-semibold tracking-wide text-white/70 uppercase">
                          Industries / company types
                        </span>
                        <textarea
                          className={`${compactInputClass} min-h-[72px] resize-y`}
                          value={prefsDraft.industries}
                          disabled={savingPlaybook}
                          onChange={(event) => patchPrefs("industries", event.target.value)}
                          placeholder="developer tools, fintech, climate — blank = AI suggests"
                        />
                      </label>
                      <label className="block">
                        <span className="mb-1.5 block text-[11px] font-semibold tracking-wide text-white/70 uppercase">
                          Max contacts / company
                        </span>
                        <input
                          className={compactInputClass}
                          type="number"
                          min={1}
                          max={10}
                          value={prefsDraft.maxContacts}
                          disabled={savingPlaybook}
                          onChange={(event) => patchPrefs("maxContacts", event.target.value)}
                        />
                      </label>
                    </>
                  )}

                  {editingSelectedCampaign ? (
                    <Button
                      variant="default"
                      disabled={savingPlaybook}
                      onClick={() => void savePlaybookSettings()}
                      className="w-full bg-white text-accent shadow-none hover:bg-white/90"
                    >
                      {savingPlaybook ? "Saving…" : "Save campaign preferences"}
                    </Button>
                  ) : (
                    <p className="text-[11px] leading-relaxed text-white/65">
                      These preferences are saved with the new campaign. Blank lists are filled by AI
                      after analysis.
                    </p>
                  )}
                </div>
                </fieldset>
              </aside>
            </div>
          </div>
          {health && !health.capabilities.llm && (
            <div className="px-5 py-4 sm:px-8">
              <Notice tone="warn">
                No LLM key is configured, so scanning will fail. Set <code>OPENAI_API_KEY</code> in{" "}
                <code>server/.env</code> and restart.
              </Notice>
            </div>
          )}
        </div>
      </Card>

      {overview && (
        <>
          <OverviewStats overview={overview} />

          <PipelineAttentionBar overview={overview} onOpenActivity={onOpenActivity} />

          <Card
            title={`Campaign profile · ${overview.project.name || overview.project.domain}`}
            action={
              <div className="flex flex-wrap gap-2">
                <Pill tone="info">{overviewWorkflowConfig.label}</Pill>
                {analysis && editingAnalysis ? (
                  <>
                    <Button
                      variant="primary"
                      disabled={formLocked || savingAnalysis}
                      onClick={() => void analysisEditorRef.current?.save()}
                    >
                      {savingAnalysis ? "Saving…" : "Save changes"}
                    </Button>
                    <Button
                      variant="ghost"
                      disabled={formLocked || savingAnalysis}
                      onClick={() => analysisEditorRef.current?.cancel()}
                    >
                      Cancel
                    </Button>
                  </>
                ) : analysis ? (
                  <Button disabled={formLocked} onClick={() => setEditingAnalysis(true)}>
                    {overviewWorkflow === "job_outreach" ? "Edit profile" : "Edit analysis"}
                  </Button>
                ) : null}
                <Button
                  disabled={formLocked || editingAnalysis}
                  onClick={() => {
                    setAlsoRegenerate(false);
                    setPreserveProspecting(true);
                    setRescanOpen(true);
                  }}
                >
                  {setupRunning
                    ? "Scanning…"
                    : overviewWorkflow === "job_outreach"
                      ? "Re-analyse resume"
                      : "Re-scan site"}
                </Button>
                <Button
                  disabled={formLocked || !analysis || editingAnalysis}
                  onClick={() => {
                    setPreserveProspecting(true);
                    setRegenOpen(true);
                  }}
                >
                  {overviewWorkflow === "job_outreach"
                    ? "Regenerate search lanes"
                    : "Regenerate audiences"}
                </Button>
                <Button
                  variant="danger"
                  disabled={formLocked || editingAnalysis}
                  onClick={() => {
                    setDeleteAcknowledged(false);
                    setDeleteOpen(true);
                  }}
                >
                  Delete campaign
                </Button>
              </div>
            }
          >
            {!analysis ? (
              <Empty>
                {overview.counts.pendingJobs > 0
                  ? "Campaign analysis in progress — open Activity for live steps."
                  : (overview.pipeline?.failed.length ?? 0) > 0
                    ? "Scan did not finish. Open Activity to retry only the failed step."
                    : "No analysis yet. Complete setup to get started."}
              </Empty>
            ) : (
              <AnalysisEditor
                projectId={overview.project.id}
                analysis={analysis}
                workflowType={overview.project.workflow_type}
                editing={editingAnalysis}
                onEditingChange={setEditingAnalysis}
                onBusyChange={setSavingAnalysis}
                editorRef={analysisEditorRef}
                onSaved={onRefresh}
                onError={onError}
                onNotice={onNotice}
              />
            )}
          </Card>
        </>
      )}

      {health && (
        <Card title="Provider status">
          <div className="flex flex-wrap gap-2">
            <Pill tone={health.capabilities.llm ? "good" : "bad"}>
              LLM {health.capabilities.llm ? "ready" : "missing key"}
            </Pill>
            <Pill tone={health.capabilities.crawler ? "good" : "warn"}>
              Crawler {health.capabilities.crawler ? "ready" : "plain fetch fallback"}
            </Pill>
            <Pill tone={health.capabilities.search ? "good" : "bad"}>
              Search {health.capabilities.search ? "ready" : "missing key"}
            </Pill>
            <Pill tone={health.capabilities.verify ? "good" : "warn"}>
              Verification {health.capabilities.verify ? "ready" : "off"}
            </Pill>
            <Pill tone="info">Email finding: scrape + guess</Pill>
            <Pill tone={health.capabilities.sendingLive ? "warn" : "info"}>
              Sender {health.sender.provider}
            </Pill>
            <Pill tone="neutral">
              {health.sender.dailyCap}/day · {health.sender.window}
            </Pill>
            <Pill tone="neutral">
              up to {health.prospecting.maxContactsPerCompany}/target
            </Pill>
            {health.prospecting.localizeEmails && <Pill tone="info">localization on</Pill>}
          </div>
          {health.blockers.length > 0 && (
            <ul className="mt-4 space-y-1.5 text-xs">
              {health.blockers.map((blocker) => (
                <li key={blocker}>
                  <Notice tone="warn">{blocker}</Notice>
                </li>
              ))}
            </ul>
          )}
        </Card>
      )}

      {overview && (
        <>
          <ConfirmDialog
            open={rescanOpen}
            title={overviewWorkflow === "job_outreach" ? "Re-analyse resume?" : "Re-scan website?"}
            description={
              <>
                Updates the campaign profile from the latest setup input.
                {hasProspecting
                  ? ` You currently have ${companyCount} targets and ${contactCount} contacts.`
                  : null}{" "}
                Contact lists are never rebuilt here — finding contacts stays a separate step.
              </>
            }
            confirmLabel="Re-scan"
            danger={alsoRegenerate && !preserveProspecting}
            busy={busy}
            onCancel={() => setRescanOpen(false)}
            onConfirm={() => {
              void run(async () => {
                await api.rescan(overview.project.id, {
                  regenerateAudiences: alsoRegenerate,
                  preserveProspecting,
                });
                setRescanOpen(false);
                onNotice?.(
                  alsoRegenerate
                    ? preserveProspecting
                      ? "Re-scan queued. Audiences will regenerate; previous contacts kept."
                      : "Re-scan queued. Audiences will regenerate and old prospecting data will be removed."
                    : "Re-scan queued. Audiences and contacts will not change.",
                );
              });
            }}
          >
            <WhatHappensList {...rescanWhatHappens(alsoRegenerate, preserveProspecting)} />
            <CheckboxRow
              checked={alsoRegenerate}
              onChange={setAlsoRegenerate}
              label="Also regenerate audiences with AI"
              hint="Optional. Refreshes who to target. Does not find contacts — that still happens later via “Find next batch”."
            />
            {alsoRegenerate && (
              <CheckboxRow
                checked={preserveProspecting}
                onChange={setPreserveProspecting}
                label="Keep existing targets, contacts & emails"
                hint="On by default. Turn off only if you want a clean slate for AI-generated audiences and their prospecting data."
              />
            )}
            {alsoRegenerate && !preserveProspecting && (
              <Notice tone="bad">
                Permanent delete: targets, contacts, and draft/sent emails tied to AI-generated
                audiences will be removed.
              </Notice>
            )}
          </ConfirmDialog>

          <ConfirmDialog
            open={regenOpen}
            title="Regenerate audiences?"
            description={
              <>
                Rebuilds who to target from the current campaign profile.
                {hasProspecting
                  ? ` You currently have ${companyCount} targets and ${contactCount} contacts.`
                  : null}{" "}
                Contact lists are never rebuilt here — finding contacts stays a separate step.
              </>
            }
            confirmLabel="Regenerate"
            danger={!preserveProspecting}
            busy={busy}
            onCancel={() => setRegenOpen(false)}
            onConfirm={() => {
              void run(async () => {
                await api.regenerateAudiences(overview.project.id, { preserveProspecting });
                setRegenOpen(false);
                onNotice?.(
                  preserveProspecting
                    ? "Regenerating audiences. Previous contacts and messages will be kept."
                    : "Regenerating audiences. Previous prospecting data for generated audiences will be deleted.",
                );
              });
            }}
          >
            <WhatHappensList {...regenWhatHappens(preserveProspecting)} />
            <CheckboxRow
              checked={preserveProspecting}
              onChange={setPreserveProspecting}
              label="Keep existing targets, contacts & emails"
              hint="On by default. Old AI audiences with data stay (disabled as “kept”). Turn off only for a clean slate."
            />
            {!preserveProspecting && (
              <Notice tone="bad">
                Permanent delete: targets, contacts, and draft/sent emails tied to AI-generated
                audiences will be removed.
              </Notice>
            )}
          </ConfirmDialog>

          <ConfirmDialog
            open={deleteOpen}
            title={`Delete ${overview.project.name || overview.project.domain}?`}
            description={
              <>
                This permanently deletes the current campaign. In the current data model, contacts are
                tied to a campaign through <code>project_id</code>, so deleting the campaign also
                deletes its related targets, contacts, draft/sent emails, activity, and queued jobs.
              </>
            }
            confirmLabel="Delete campaign"
            danger
            busy={busy}
            confirmDisabled={!deleteAcknowledged}
            onCancel={() => {
              setDeleteOpen(false);
              setDeleteAcknowledged(false);
            }}
            onConfirm={() => {
              if (deleteAcknowledged) void deleteCampaign();
            }}
          >
            <WhatHappensList
              will={[
                `Delete ${companyCount} target${companyCount === 1 ? "" : "s"}.`,
                `Delete ${contactCount} contact${contactCount === 1 ? "" : "s"} and all tied emails.`,
                "Remove pending or failed jobs for this campaign.",
              ]}
              willNot={[
                "Delete contacts from other campaigns or playbooks.",
                "Delete global settings, suppressions, or sender configuration.",
              ]}
            />
            <CheckboxRow
              checked={deleteAcknowledged}
              onChange={setDeleteAcknowledged}
              label="I understand campaign contacts and emails will be deleted"
              hint="Keeping contacts after campaign deletion needs a separate global contact-book model; this app currently stores contacts inside campaigns."
            />
          </ConfirmDialog>
        </>
      )}
    </div>
  );
}
