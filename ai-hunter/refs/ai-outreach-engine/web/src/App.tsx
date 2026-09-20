import { useCallback, useEffect, useMemo, useState } from "react";
import { api, type Health, type Overview, type Project, type WorkflowType } from "./lib/api";
import { SetupPanel } from "./components/SetupPanel";
import { AudiencePanel } from "./components/AudiencePanel";
import { ContactsPanel } from "./components/ContactsPanel";
import { ReviewPanel } from "./components/ReviewPanel";
import { ActivityPanel } from "./components/ActivityPanel";
import { SettingsPanel } from "./components/SettingsPanel";
import { LivePipelineToast } from "./components/LivePipelineToast";
import { StatusBar } from "./components/ui";
import { isPipelineBusy } from "./lib/pipelineBusy";

const TABS = [
  { id: "setup", label: "Setup", hint: "Scan & configure" },
  { id: "audiences", label: "Audiences", hint: "Target segments" },
  { id: "contacts", label: "Contacts", hint: "Found people" },
  { id: "review", label: "Review & send", hint: "Approve drafts" },
  { id: "activity", label: "Activity", hint: "Pipeline & log" },
  { id: "settings", label: "Settings", hint: "Platform brand & limits" },
] as const;

type TabId = (typeof TABS)[number]["id"];

const PLAYBOOKS: Record<WorkflowType, { label: string; hint: string }> = {
  customer_outreach: { label: "Customers", hint: "Find buyers" },
  investor_outreach: { label: "Investors", hint: "Raise funding" },
  job_outreach: { label: "Jobs", hint: "Reach hiring teams" },
};

const WORKFLOW_TYPES = Object.keys(PLAYBOOKS) as WorkflowType[];

function isWorkflowType(value: string | undefined): value is WorkflowType {
  return WORKFLOW_TYPES.includes(value as WorkflowType);
}

function isTabId(value: string | undefined): value is TabId {
  return TABS.some((entry) => entry.id === value);
}

function parseHashRoute(): { workflowType: WorkflowType; projectId: number | null; tab: TabId } {
  const [workflowRaw, projectRaw, tabRaw] = window.location.hash.replace(/^#\/?/, "").split("/");
  const projectId = projectRaw && projectRaw !== "new" ? Number(projectRaw) : null;
  return {
    workflowType: isWorkflowType(workflowRaw) ? workflowRaw : "customer_outreach",
    projectId: Number.isFinite(projectId) ? projectId : null,
    tab: isTabId(tabRaw) ? tabRaw : "setup",
  };
}

function routeHash(workflowType: WorkflowType, projectId: number | null, tab: TabId): string {
  return `#/${workflowType}/${projectId ?? "new"}/${tab}`;
}

function BrandMark({ size = 36 }: { size?: number }) {
  return (
    <div
      className="flex shrink-0 items-center justify-center rounded-[11px] bg-accent shadow-[var(--shadow-glow)]"
      style={{ width: size, height: size }}
    >
      <svg width={size * 0.55} height={size * 0.55} viewBox="0 0 18 18" fill="none" aria-hidden>
        <rect x="3" y="4" width="12" height="2.2" rx="1.1" fill="white" />
        <rect x="3" y="8" width="9" height="2.2" rx="1.1" fill="white" opacity="0.85" />
        <rect x="3" y="12" width="6" height="2.2" rx="1.1" fill="white" opacity="0.65" />
      </svg>
    </div>
  );
}

export function App() {
  const initialRoute = parseHashRoute();
  const [tab, setTab] = useState<TabId>(initialRoute.tab);
  const [activeWorkflowType, setActiveWorkflowType] = useState<WorkflowType>(
    initialRoute.workflowType,
  );
  const [health, setHealth] = useState<Health | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectId, setProjectId] = useState<number | null>(initialRoute.projectId);
  const [overview, setOverview] = useState<Overview | null>(null);
  const [projectsRefreshKey, setProjectsRefreshKey] = useState(0);
  const [overviewRefreshKey, setOverviewRefreshKey] = useState(0);
  const [toast, setToast] = useState<{ tone: "error" | "info"; text: string } | null>(null);
  const [mobileNavOpen, setMobileNavOpen] = useState(false);

  const notify = useCallback((tone: "error" | "info", text: string) => {
    setToast({ tone, text });
    setTimeout(() => setToast(null), 6000);
  }, []);

  const onError = useCallback((message: string) => notify("error", message), [notify]);
  const onNotice = useCallback((message: string) => notify("info", message), [notify]);
  const refreshProjects = useCallback(() => setProjectsRefreshKey((key) => key + 1), []);
  const refreshOverview = useCallback(() => setOverviewRefreshKey((key) => key + 1), []);
  const handleProjectCreated = useCallback(
    (createdProjectId: number) => {
      setProjectId(createdProjectId);
      refreshProjects();
      refreshOverview();
    },
    [refreshOverview, refreshProjects],
  );
  const handleProjectDeleted = useCallback(
    (deletedProjectId: number) => {
      setProjectId((current) => (current === deletedProjectId ? null : current));
      setOverview((current) => (current?.project.id === deletedProjectId ? null : current));
      refreshProjects();
      refreshOverview();
    },
    [refreshOverview, refreshProjects],
  );

  const refreshHealth = useCallback(() => {
    void api.health().then(setHealth).catch(() => {});
  }, []);

  useEffect(() => {
    const onHashChange = () => {
      const route = parseHashRoute();
      setActiveWorkflowType(route.workflowType);
      setProjectId(route.projectId);
      setTab(route.tab);
    };
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  useEffect(() => {
    const next = routeHash(activeWorkflowType, projectId, tab);
    if (window.location.hash !== next) {
      window.history.replaceState(null, "", next);
    }
  }, [activeWorkflowType, projectId, tab]);

  useEffect(() => {
    refreshHealth();
  }, [refreshHealth]);

  useEffect(() => {
    void api
      .listProjects()
      .then((payload) => {
        setProjects(payload.projects);
      })
      .catch(() => {});
  }, [projectsRefreshKey]);

  const projectsForWorkflow = useMemo(
    () => projects.filter((project) => project.workflow_type === activeWorkflowType),
    [activeWorkflowType, projects],
  );

  useEffect(() => {
    setProjectId((current) => {
      if (
        current !== null &&
        projects.some((project) => project.id === current && project.workflow_type === activeWorkflowType)
      ) {
        return current;
      }
      return projectsForWorkflow[0]?.id ?? null;
    });
  }, [activeWorkflowType, projects, projectsForWorkflow]);

  useEffect(() => {
    if (projectId === null) {
      setOverview(null);
      return;
    }
    void api.overview(projectId).then(setOverview).catch(() => {});
  }, [projectId, overviewRefreshKey]);

  useEffect(() => {
    if (projectId === null) return;

    let closed = false;
    let source: EventSource | null = null;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;
    let attempt = 0;

    const connect = () => {
      if (closed) return;
      source = new EventSource(`/api/projects/${projectId}/stream`);
      source.addEventListener("update", () => {
        attempt = 0;
        refreshOverview();
      });
      source.onerror = () => {
        source?.close();
        source = null;
        if (closed) return;
        const delay = Math.min(1000 * 2 ** attempt, 15_000);
        attempt += 1;
        retryTimer = setTimeout(connect, delay);
      };
    };

    connect();
    return () => {
      closed = true;
      if (retryTimer) clearTimeout(retryTimer);
      source?.close();
    };
  }, [projectId, refreshOverview]);

  useEffect(() => {
    const pending = overview?.counts.pendingJobs ?? 0;
    const failed = overview?.pipeline?.failedCount ?? overview?.counts.failedJobs ?? 0;
    // Poll while work is active; also briefly after failures so retries surface quickly
    // even if the SSE connection blipped.
    if (pending === 0 && failed === 0) return;
    const timer = setInterval(refreshOverview, pending > 0 ? 3000 : 8000);
    return () => clearInterval(timer);
  }, [overview?.counts.pendingJobs, overview?.counts.failedJobs, overview?.pipeline?.failedCount, refreshOverview]);

  const drafts = overview?.counts.messages.draft ?? 0;
  const pending = overview?.counts.pendingJobs ?? 0;
  const failed = overview?.pipeline?.failedCount ?? overview?.counts.failedJobs ?? 0;
  const runningLabel =
    overview?.pipeline?.active.find((job) => job.status === "running")?.label ??
    overview?.pipeline?.active[0]?.label ??
    null;
  const activeTab = TABS.find((entry) => entry.id === tab)!;

  function selectTab(id: TabId) {
    setTab(id);
    setMobileNavOpen(false);
  }

  function selectWorkflow(type: WorkflowType) {
    setActiveWorkflowType(type);
    setProjectId(projects.find((project) => project.workflow_type === type)?.id ?? null);
    setTab("setup");
    setMobileNavOpen(false);
  }

  function selectProject(value: string) {
    if (!value) {
      setProjectId(null);
      return;
    }
    const id = Number(value);
    const project = projects.find((entry) => entry.id === id);
    if (project) setActiveWorkflowType(project.workflow_type);
    setProjectId(Number.isFinite(id) ? id : null);
  }

  return (
    <div className="flex min-h-dvh w-full">
      {/* Flush left rail — full viewport height, footer always visible */}
      <aside className="sticky top-0 z-30 hidden h-dvh w-[272px] shrink-0 flex-col border-r border-line bg-surface lg:flex xl:w-[300px]">
        <div className="flex items-center gap-3 px-5 pt-6 pb-5">
          <BrandMark />
          <div className="min-w-0">
            <div className="text-[15px] font-bold tracking-tight text-ink">outreach</div>
            <div className="text-[11px] font-medium text-ink-faint">scan · find · send</div>
          </div>
        </div>

        <p className="mb-2 px-5 text-[10px] font-bold tracking-[0.14em] text-ink-faint uppercase">
          Playbook
        </p>

        <div className="space-y-1 px-3 pb-3">
          {WORKFLOW_TYPES.map((type) => {
            const active = activeWorkflowType === type;
            const count = projects.filter((project) => project.workflow_type === type).length;
            return (
              <button
                key={type}
                type="button"
                onClick={() => selectWorkflow(type)}
                className={`flex w-full items-center justify-between rounded-2xl px-3 py-2.5 text-left transition-all duration-200 ${
                  active
                    ? "bg-surface-2 text-ink ring-1 ring-line"
                    : "text-ink-soft hover:bg-surface-2 hover:text-ink"
                }`}
              >
                <span className="min-w-0">
                  <span className="block text-[13px] font-semibold tracking-tight">
                    {PLAYBOOKS[type].label}
                  </span>
                  <span className="mt-0.5 block text-[11px] text-ink-faint">
                    {PLAYBOOKS[type].hint}
                  </span>
                </span>
                <span className="rounded-full bg-surface px-2 py-0.5 text-[10px] font-bold tabular-nums text-ink-faint">
                  {count}
                </span>
              </button>
            );
          })}
        </div>

        <p className="mb-2 px-5 text-[10px] font-bold tracking-[0.14em] text-ink-faint uppercase">
          Workspace
        </p>

        <nav className="min-h-0 flex-1 space-y-1 overflow-y-auto px-3 pb-3">
          {TABS.map((entry) => {
            const active = tab === entry.id;
            return (
              <button
                key={entry.id}
                type="button"
                onClick={() => selectTab(entry.id)}
                className={`flex w-full items-center justify-between rounded-2xl px-3 py-2.5 text-left transition-all duration-200 ${
                  active
                    ? "bg-accent text-white shadow-[var(--shadow-glow)]"
                    : "text-ink-soft hover:bg-surface-2 hover:text-ink"
                }`}
              >
                <span className="min-w-0">
                  <span className="block text-[13px] font-semibold tracking-tight">{entry.label}</span>
                  <span className={`mt-0.5 block text-[11px] ${active ? "text-white/70" : "text-ink-faint"}`}>
                    {entry.hint}
                  </span>
                </span>
                {entry.id === "review" && drafts > 0 && (
                  <span
                    className={`ml-2 rounded-full px-2 py-0.5 text-[10px] font-bold tabular-nums ${
                      active ? "bg-white/20 text-white" : "bg-accent-soft text-accent"
                    }`}
                  >
                    {drafts}
                  </span>
                )}
              </button>
            );
          })}
        </nav>

        <div className="shrink-0 space-y-3 border-t border-line bg-surface-2/40 p-4">
          {projectsForWorkflow.length > 0 && (
            <select
              value={projectId ?? ""}
              onChange={(event) => selectProject(event.target.value)}
              className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-xs font-semibold text-ink outline-none focus:ring-2 focus:ring-accent/20"
            >
              {projectsForWorkflow.map((project) => (
                <option key={project.id} value={project.id}>
                  {project.name || project.domain}
                </option>
              ))}
            </select>
          )}
          <div className="space-y-1.5 text-xs">
            {pending > 0 && (
              <button
                type="button"
                onClick={() => selectTab("activity")}
                className="block w-full text-left font-semibold text-accent hover:underline"
                title="Open Activity for live pipeline detail"
              >
                {pending} job{pending === 1 ? "" : "s"} in progress
                {runningLabel ? ` · ${runningLabel}` : ""}
              </button>
            )}
            {failed > 0 && (
              <button
                type="button"
                onClick={() => {
                  const setupOnly = new Set(["scan_site", "generate_audiences"]);
                  const stages = overview?.pipeline?.failed.map((job) => job.stage) ?? [];
                  const allSetup =
                    stages.length > 0 && stages.every((stage) => setupOnly.has(stage));
                  selectTab(allSetup ? "setup" : "activity");
                }}
                className="block w-full text-left font-semibold text-bad hover:underline"
                title="Open failed steps to retry individually"
              >
                {failed} failed · retry available
              </button>
            )}
            {health && (
              <div className="flex items-center gap-2 font-medium text-ink">
                <span
                  className={`h-2 w-2 rounded-full ${
                    health.capabilities.sendingLive ? "live-dot bg-warn" : "bg-accent"
                  }`}
                />
                {health.capabilities.sendingLive ? "Sending live" : "Dry run"}
              </div>
            )}
            {overview && (
              <div className="font-medium tabular-nums text-ink-soft">
                <span className="text-ink">{overview.counts.sentLast24h}</span>
                <span className="text-ink-faint"> / {health?.sender.dailyCap ?? "?"} </span>
                sent today
              </div>
            )}
          </div>
        </div>
      </aside>

      {/* Main column fills remaining width on any screen size */}
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-20 border-b border-line/80 bg-surface/90 px-4 py-3 backdrop-blur-xl sm:px-6 lg:hidden">
          <div className="flex items-center justify-between gap-3">
            <div className="flex min-w-0 items-center gap-2.5">
              <BrandMark size={32} />
              <div className="min-w-0">
                <div className="truncate text-sm font-bold tracking-tight">outreach</div>
                <div className="truncate text-[11px] text-ink-faint">
                  {PLAYBOOKS[activeWorkflowType].label} · {activeTab.label}
                </div>
              </div>
            </div>
            <div className="flex items-center gap-2">
              <select
                value={activeWorkflowType}
                onChange={(event) => selectWorkflow(event.target.value as WorkflowType)}
                className="max-w-[120px] truncate rounded-full border-0 bg-surface-2 px-3 py-1.5 text-[11px] font-semibold outline-none"
              >
                {WORKFLOW_TYPES.map((type) => (
                  <option key={type} value={type}>
                    {PLAYBOOKS[type].label}
                  </option>
                ))}
              </select>
              {projectsForWorkflow.length > 0 && (
                <select
                  value={projectId ?? ""}
                  onChange={(event) => selectProject(event.target.value)}
                  className="max-w-[140px] truncate rounded-full border-0 bg-surface-2 px-3 py-1.5 text-[11px] font-semibold outline-none"
                >
                  {projectsForWorkflow.map((project) => (
                    <option key={project.id} value={project.id}>
                      {project.name || project.domain}
                    </option>
                  ))}
                </select>
              )}
              <button
                type="button"
                aria-label={mobileNavOpen ? "Close navigation" : "Open navigation"}
                onClick={() => setMobileNavOpen((open) => !open)}
                className="flex h-9 w-9 items-center justify-center rounded-full bg-surface-2 text-ink"
              >
                <svg width="18" height="18" viewBox="0 0 16 16" fill="none" aria-hidden>
                  {mobileNavOpen ? (
                    <path
                      d="M4 4l8 8M12 4l-8 8"
                      stroke="currentColor"
                      strokeWidth="1.8"
                      strokeLinecap="round"
                    />
                  ) : (
                    <path
                      d="M3 4.5h10M3 8h10M3 11.5h10"
                      stroke="currentColor"
                      strokeWidth="1.8"
                      strokeLinecap="round"
                    />
                  )}
                </svg>
              </button>
            </div>
          </div>

          {mobileNavOpen && (
            <nav className="animate-rise mt-3 grid gap-1.5 pb-1">
              {TABS.map((entry) => (
                <button
                  key={entry.id}
                  type="button"
                  onClick={() => selectTab(entry.id)}
                  className={`flex items-center justify-between rounded-2xl px-3.5 py-2.5 text-left text-sm font-semibold ${
                    tab === entry.id ? "bg-accent text-white" : "bg-surface-2 text-ink-soft"
                  }`}
                >
                  {entry.label}
                  {entry.id === "review" && drafts > 0 && (
                    <span className="rounded-full bg-white/20 px-2 py-0.5 text-[10px]">{drafts}</span>
                  )}
                </button>
              ))}
            </nav>
          )}
        </header>

        <header className="hidden items-center justify-between gap-4 px-6 pt-6 pb-2 lg:flex xl:px-10 2xl:px-14">
          <div className="animate-fade min-w-0">
            <p className="text-[11px] font-bold tracking-[0.14em] text-ink-faint uppercase">
              {PLAYBOOKS[activeWorkflowType].label} · {activeTab.hint}
            </p>
            <h1 className="mt-1 text-2xl font-bold tracking-tight text-ink xl:text-[1.75rem]">
              {activeTab.label}
            </h1>
          </div>
          {(health || overview) && (
            <StatusBar
              live={health?.capabilities.sendingLive}
              fromEmail={health?.sender.fromEmail}
              sentToday={overview?.counts.sentLast24h}
              dailyCap={health?.sender.dailyCap}
            />
          )}
        </header>

        <div className="hidden gap-2 overflow-x-auto px-6 pb-2 md:flex lg:hidden">
          {TABS.map((entry) => (
            <button
              key={entry.id}
              type="button"
              onClick={() => selectTab(entry.id)}
              className={`shrink-0 rounded-full px-3.5 py-1.5 text-xs font-semibold transition-colors ${
                tab === entry.id
                  ? "bg-accent text-white shadow-[var(--shadow-glow)]"
                  : "bg-surface text-ink-soft shadow-[var(--shadow-soft)] hover:text-ink"
              }`}
            >
              {entry.label}
              {entry.id === "review" && drafts > 0 && (
                <span className="ml-1.5 opacity-80">{drafts}</span>
              )}
            </button>
          ))}
        </div>

        <main className="animate-rise flex-1 px-4 py-4 sm:px-6 sm:py-5 lg:px-6 lg:pb-10 xl:px-10 2xl:px-14">
          <div className="mx-auto w-full max-w-[1600px]">
            {tab === "setup" && (
              <SetupPanel
                overview={overview}
                health={health}
                workflowType={activeWorkflowType}
                workflowProjectCount={projectsForWorkflow.length}
                onWorkflowChange={selectWorkflow}
                onRefresh={() => {
                  refreshOverview();
                  refreshHealth();
                }}
                onProjectCreated={handleProjectCreated}
                onProjectDeleted={handleProjectDeleted}
                onError={onError}
                onNotice={onNotice}
                onOpenActivity={() => selectTab("activity")}
              />
            )}
            {tab === "audiences" && (
              <AudiencePanel
                overview={overview}
                searchReady={health?.capabilities.search ?? false}
                onRefresh={refreshOverview}
                onError={onError}
                onNotice={onNotice}
                onOpenActivity={() => selectTab("activity")}
              />
            )}
            {tab === "contacts" && (
              <ContactsPanel
                projectId={projectId}
                campaignName={overview?.project.name || overview?.project.domain || null}
                activeWorkflowType={activeWorkflowType}
                audiences={overview?.audiences ?? []}
                refreshKey={overviewRefreshKey}
                onRefresh={refreshOverview}
                onError={onError}
                onNotice={onNotice}
              />
            )}
            {tab === "review" && (
              <ReviewPanel
                projectId={projectId}
                refreshKey={overviewRefreshKey}
                draftCount={drafts}
                health={health}
                onRefresh={refreshOverview}
                onError={onError}
                onNotice={onNotice}
              />
            )}
            {tab === "activity" && (
              <ActivityPanel
                overview={overview}
                refreshKey={overviewRefreshKey}
                onRefresh={refreshOverview}
                onError={onError}
                onNotice={onNotice}
              />
            )}
            {tab === "settings" && (
              <SettingsPanel
                onRefresh={() => {
                  refreshOverview();
                  refreshHealth();
                }}
                onError={onError}
                onNotice={onNotice}
              />
            )}
          </div>
        </main>
      </div>

      <LivePipelineToast
        overview={overview}
        onRefresh={refreshOverview}
        onNotice={onNotice}
        onError={onError}
      />

      {toast && (
        <div
          className={`animate-rise fixed right-4 z-[60] max-w-sm rounded-2xl border px-4 py-3 text-xs font-medium shadow-[var(--shadow-lift)] sm:right-6 ${
            isPipelineBusy(overview) ? "bottom-36 sm:bottom-40" : "bottom-4 sm:bottom-6"
          } ${
            toast.tone === "error"
              ? "border-bad/20 bg-surface text-bad"
              : "border-accent/20 bg-surface text-accent"
          }`}
        >
          {toast.text}
        </div>
      )}
    </div>
  );
}
