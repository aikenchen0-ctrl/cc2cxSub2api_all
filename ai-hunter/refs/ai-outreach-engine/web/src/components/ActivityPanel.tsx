import { useEffect, useMemo, useRef, useState } from "react";
import { api, type ActivityEvent, type Overview, type Suppression } from "../lib/api";
import { PipelineStatus } from "./PipelineStatus";
import { Button, Card, Empty, Pill, inputClass } from "./ui";

const KIND_TONE: Record<string, "good" | "warn" | "bad" | "info" | "neutral"> = {
  job_done: "good",
  job_failed: "bad",
  send: "good",
  unsubscribe: "warn",
  llm_call: "info",
  search: "info",
  verify: "neutral",
  crawl: "neutral",
  contact_finder: "warn",
};

const STAGE_KEYS = new Set([
  "scan_site",
  "generate_audiences",
  "generate_audience_email",
  "regenerate_audience",
  "find_companies",
  "find_contacts",
  "verify_contact",
  "draft_message",
  "send_message",
]);

function summarise(event: ActivityEvent): string {
  try {
    const data = JSON.parse(event.data) as Record<string, unknown>;
    if (typeof data.result === "string") return data.result;
    if (typeof data.error === "string") return data.error;
    return Object.entries(data)
      .map(([key, value]) => `${key}=${value}`)
      .join(" ");
  } catch {
    return event.data;
  }
}

function matchesJobFilter(event: ActivityEvent, stage: string, label: string): boolean {
  if (event.ref === stage) return true;
  const blob = `${event.kind} ${event.ref ?? ""} ${summarise(event)}`.toLowerCase();
  return blob.includes(stage.toLowerCase()) || blob.includes(label.toLowerCase());
}

function mergeEvents(current: ActivityEvent[], incoming: ActivityEvent[]): ActivityEvent[] {
  if (current.length === 0) return incoming;
  const byId = new Map<number, ActivityEvent>();
  for (const event of current) byId.set(event.id, event);
  for (const event of incoming) byId.set(event.id, event);
  return [...byId.values()].sort((a, b) => b.id - a.id);
}

export function ActivityPanel({
  overview,
  refreshKey,
  onRefresh,
  onError,
  onNotice,
}: {
  overview: Overview | null;
  refreshKey: number;
  onRefresh: () => void;
  onError: (message: string) => void;
  onNotice: (message: string) => void;
}) {
  const [events, setEvents] = useState<ActivityEvent[]>([]);
  const [totalEvents, setTotalEvents] = useState(0);
  const [nextBeforeId, setNextBeforeId] = useState<number | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [suppressions, setSuppressions] = useState<Suppression[]>([]);
  const [newSuppression, setNewSuppression] = useState("");
  const [kindFilter, setKindFilter] = useState<string | null>(null);
  const [jobFilter, setJobFilter] = useState<{ stage: string; label: string } | null>(null);
  const hasLoadedMoreRef = useRef(false);

  const projectId = overview?.project.id ?? null;

  // Hard reset only when switching projects — never when the live overview refreshes.
  useEffect(() => {
    hasLoadedMoreRef.current = false;
    setEvents([]);
    setTotalEvents(0);
    setNextBeforeId(null);
    setKindFilter(null);
    setJobFilter(null);
  }, [projectId]);

  useEffect(() => {
    if (projectId === null) return;
    void api
      .events(projectId, { limit: 50 })
      .then((payload) => {
        setTotalEvents(payload.total);
        setEvents((current) => mergeEvents(current, payload.events));
        // Keep the older-page cursor after Load more so a live refresh cannot
        // wipe paginated history and snap the list back to the first page.
        if (!hasLoadedMoreRef.current) {
          setNextBeforeId(payload.nextBeforeId);
        }
      })
      .catch(() => {});
    void api.suppressions().then((payload) => setSuppressions(payload.suppressions)).catch(() => {});
  }, [projectId, refreshKey]);

  const kindCounts = useMemo(() => {
    const counts = new Map<string, number>();
    for (const event of events) {
      counts.set(event.kind, (counts.get(event.kind) ?? 0) + 1);
    }
    return [...counts.entries()].sort((a, b) => b[1] - a[1]);
  }, [events]);

  const filteredEvents = useMemo(() => {
    return events.filter((event) => {
      if (kindFilter && event.kind !== kindFilter) return false;
      if (jobFilter && !matchesJobFilter(event, jobFilter.stage, jobFilter.label)) return false;
      return true;
    });
  }, [events, kindFilter, jobFilter]);

  async function loadMore() {
    if (projectId === null || nextBeforeId === null) return;
    setLoadingMore(true);
    try {
      const payload = await api.events(projectId, { limit: 50, beforeId: nextBeforeId });
      hasLoadedMoreRef.current = true;
      setEvents((current) => mergeEvents(current, payload.events));
      setTotalEvents(payload.total);
      setNextBeforeId(payload.nextBeforeId);
    } catch (error) {
      onError(error instanceof Error ? error.message : "Failed to load more activity");
    } finally {
      setLoadingMore(false);
    }
  }

  if (!overview) return <Empty>Create a campaign first.</Empty>;

  return (
    <div className="space-y-5">
      <PipelineStatus
        overview={overview}
        onRefresh={onRefresh}
        onError={onError}
        onNotice={onNotice}
      />

      <div className="grid gap-5 xl:grid-cols-2">
        <Card title="Pipeline stages">
          {overview.jobs.length === 0 ? (
            <Empty>No jobs have run yet.</Empty>
          ) : (
            <div className="flex flex-wrap gap-2">
              {overview.jobs.map((job) => {
                const key = `${job.stage}-${job.status}`;
                const active = jobFilter?.stage === job.stage && jobFilter.label === job.label;
                return (
                  <Pill
                    key={key}
                    active={active}
                    title={active ? "Clear filter" : `Filter log by “${job.label}”`}
                    onClick={() =>
                      setJobFilter(active ? null : { stage: job.stage, label: job.label })
                    }
                    tone={
                      job.status === "failed"
                        ? "bad"
                        : job.status === "done"
                          ? "good"
                          : job.status === "running"
                            ? "info"
                            : "warn"
                    }
                  >
                    {job.label}
                    <span className="opacity-70">·</span>
                    {job.status}
                    <span className="tabular-nums opacity-80">{job.n}</span>
                  </Pill>
                );
              })}
            </div>
          )}
          {jobFilter && (
            <button
              type="button"
              onClick={() => setJobFilter(null)}
              className="mt-3 text-xs font-semibold text-accent hover:underline"
            >
              Clear pipeline filter
            </button>
          )}
          <p className="mt-3 text-xs leading-relaxed text-ink-faint">
            Counts are grouped by stage and status. Use Needs attention above to retry or dismiss
            failures. Dismissed errors leave the prompts on Setup/Audiences/Activity but stay in
            the log below.
          </p>
        </Card>

        <Card title="Do not contact">
          <div className="mb-4 flex flex-col gap-2 sm:flex-row sm:flex-wrap">
            <input
              className={`${inputClass} min-w-0 flex-1`}
              placeholder="someone@company.com"
              value={newSuppression}
              onChange={(event) => setNewSuppression(event.target.value)}
            />
            <Button
              variant="primary"
              disabled={!newSuppression.includes("@")}
              onClick={async () => {
                try {
                  await api.suppress(newSuppression.trim());
                  setNewSuppression("");
                  onNotice("Added to the do-not-contact list.");
                  onRefresh();
                } catch (error) {
                  onError(error instanceof Error ? error.message : "Failed");
                }
              }}
            >
              Add
            </Button>
          </div>
          {suppressions.length === 0 ? (
            <Empty>Nobody is suppressed yet.</Empty>
          ) : (
            <ul className="space-y-1.5">
              {suppressions.map((entry) => (
                <li
                  key={`${entry.email ?? entry.domain}-${entry.created_at}`}
                  className="flex items-center justify-between gap-3 rounded-2xl border border-line bg-surface-2/60 px-3.5 py-2.5 text-sm"
                >
                  <span className="font-mono font-medium">{entry.email ?? `*@${entry.domain}`}</span>
                  <Pill>{entry.reason}</Pill>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      <Card
        title="Activity log"
        action={
          <span className="text-xs font-medium text-ink-faint">
            showing {events.length}
            {totalEvents > events.length ? ` of ${totalEvents}` : ""} · spend $
            {overview.estimatedSpendUsd.toFixed(4)}
          </span>
        }
      >
        {kindCounts.length > 0 && (
          <div className="mb-4 flex flex-wrap gap-2">
            <Pill
              tone="neutral"
              active={kindFilter === null}
              onClick={() => setKindFilter(null)}
              title="Show all loaded events"
            >
              Loaded
              <span className="tabular-nums opacity-70">{events.length}</span>
            </Pill>
            {kindCounts.map(([kind, count]) => (
              <Pill
                key={kind}
                tone={KIND_TONE[kind] ?? "neutral"}
                active={kindFilter === kind}
                onClick={() => setKindFilter(kindFilter === kind ? null : kind)}
                title={kindFilter === kind ? "Clear filter" : `Filter by ${kind}`}
              >
                {kind}
                <span className="tabular-nums opacity-70">{count}</span>
              </Pill>
            ))}
          </div>
        )}

        {filteredEvents.length === 0 ? (
          <Empty>
            {events.length === 0
              ? "Nothing has happened yet."
              : "No events match this filter."}
          </Empty>
        ) : (
          <ul className="space-y-0">
            {filteredEvents.map((event) => (
              <li
                key={event.id}
                className="flex flex-col gap-2 border-b border-line/70 py-3.5 last:border-0 sm:flex-row sm:items-start sm:gap-4"
              >
                <span className="w-auto shrink-0 font-mono text-[11px] text-ink-faint sm:w-40">
                  {event.created_at}
                </span>
                <span className="w-auto shrink-0 sm:w-32">
                  <Pill
                    tone={KIND_TONE[event.kind] ?? "neutral"}
                    onClick={() => setKindFilter(event.kind)}
                    title={`Filter by ${event.kind}`}
                  >
                    {event.kind}
                  </Pill>
                </span>
                <span className="min-w-0 flex-1 text-[13px] leading-relaxed text-ink-soft">
                  {event.ref && (
                    <button
                      type="button"
                      className="text-ink-faint hover:text-accent hover:underline"
                      onClick={() => {
                        if (STAGE_KEYS.has(event.ref)) {
                          const match = overview.jobs.find((job) => job.stage === event.ref);
                          setJobFilter({
                            stage: event.ref,
                            label: match?.label ?? event.ref,
                          });
                        }
                      }}
                    >
                      {event.ref} —{" "}
                    </button>
                  )}
                  {summarise(event)}
                </span>
                {event.cost_usd > 0 && (
                  <span className="shrink-0 font-mono text-xs font-medium text-ink-faint">
                    ${event.cost_usd.toFixed(4)}
                  </span>
                )}
              </li>
            ))}
          </ul>
        )}

        {nextBeforeId !== null && (
          <div className="mt-4 flex items-center justify-between gap-3">
            <p className="text-xs text-ink-faint">
              {Math.max(totalEvents - events.length, 0)} older event
              {totalEvents - events.length === 1 ? "" : "s"} still available
            </p>
            <Button disabled={loadingMore} onClick={() => void loadMore()}>
              {loadingMore ? "Loading…" : "Load more"}
            </Button>
          </div>
        )}
      </Card>
    </div>
  );
}
