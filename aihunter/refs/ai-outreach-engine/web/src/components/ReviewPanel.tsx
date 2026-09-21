import { useCallback, useEffect, useMemo, useState } from "react";
import { api, type Health, type Message, type SendSchedule } from "../lib/api";
import { EmailCopyEditor, EmailPreviewModal } from "./EmailPreview";
import { Button, Card, Empty, Notice, Pill } from "./ui";

const STATUS_TONE: Record<string, "good" | "warn" | "bad" | "info" | "neutral"> = {
  draft: "info",
  approved: "warn",
  sending: "warn",
  sent: "good",
  failed: "bad",
  skipped: "neutral",
};

const TABS = ["draft", "approved", "sent", "skipped", "failed"] as const;

/** SQLite stores UTC as "YYYY-MM-DD HH:MM:SS". */
function parseUtc(value: string): Date {
  if (value.includes("T")) return new Date(value.endsWith("Z") ? value : `${value}Z`);
  return new Date(`${value.replace(" ", "T")}Z`);
}

function formatWhen(value: string): string {
  return parseUtc(value).toLocaleString(undefined, {
    weekday: "short",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function formatCountdown(ms: number): string {
  if (ms <= 0) return "any moment";
  const totalSeconds = Math.ceil(ms / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (hours > 0) return `${hours}h ${minutes}m ${seconds}s`;
  if (minutes > 0) return `${minutes}m ${seconds}s`;
  return `${seconds}s`;
}

function scheduleLabel(schedule: SendSchedule | null | undefined, now: number): string {
  if (!schedule) return "Queued to send. Waiting for the next runner cycle.";
  if (schedule.state === "sending") return "Sending now…";
  const due = parseUtc(schedule.runAfter).getTime();
  const wait = formatCountdown(due - now);
  const when = formatWhen(schedule.runAfter);
  if (schedule.reason) {
    return `Scheduled for ${when} (in ${wait}). Hold reason: ${schedule.reason}`;
  }
  return `Scheduled for ${when} (in ${wait}).`;
}

function MessageRow({
  projectId,
  message,
  now,
  onChanged,
  onError,
  onNotice,
}: {
  projectId: number;
  message: Message;
  now: number;
  onChanged: () => void;
  onError: (message: string) => void;
  onNotice: (message: string) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [subject, setSubject] = useState(message.subject);
  const [body, setBody] = useState(message.body);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    setSubject(message.subject);
    setBody(message.body);
  }, [message.id, message.subject, message.body]);

  async function run(action: () => Promise<unknown>) {
    setBusy(true);
    try {
      await action();
      onChanged();
    } catch (error) {
      onError(error instanceof Error ? error.message : "Action failed");
    } finally {
      setBusy(false);
    }
  }

  const flagged = message.personalisation.includes("Review flags:");
  const schedule = message.sendSchedule;
  const renderedText = message.status === "sent" ? message.sent_text_body || "" : "";

  return (
    <article className="rounded-[var(--radius-card)] border border-line bg-surface p-5 shadow-[var(--shadow-soft)] transition-shadow hover:shadow-[var(--shadow-lift)] sm:p-6">
      <header className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-mono text-[13px] font-medium text-ink">{message.email}</span>
            <Pill tone={STATUS_TONE[message.status] ?? "neutral"}>{message.status}</Pill>
            <Pill tone="neutral">plain text</Pill>
            {message.status === "approved" && schedule?.state === "sending" && (
              <Pill tone="info">sending now</Pill>
            )}
            {message.status === "approved" && schedule?.state === "scheduled" && (
              <Pill tone="warn">
                sends in {formatCountdown(parseUtc(schedule.runAfter).getTime() - now)}
              </Pill>
            )}
            {message.status === "approved" && !schedule && <Pill tone="warn">queued</Pill>}
            {message.verify_status !== "valid" && (
              <Pill tone="warn">email {message.verify_status}</Pill>
            )}
            {flagged && <Pill tone="warn">needs review</Pill>}
          </div>
          <p className="mt-1.5 text-xs text-ink-soft">
            {[message.full_name, message.title, message.company_name, message.audience_name]
              .filter(Boolean)
              .join(" · ")}
          </p>
          {(message.company_source_url || message.company_domain) && (
            <p className="mt-1 text-xs">
              {message.company_source_url && /^https?:\/\//i.test(message.company_source_url) ? (
                <a
                  href={message.company_source_url}
                  target="_blank"
                  rel="noreferrer"
                  className="font-semibold text-accent hover:underline"
                >
                  Open job / listing
                </a>
              ) : message.company_domain ? (
                <a
                  href={`https://${message.company_domain}`}
                  target="_blank"
                  rel="noreferrer"
                  className="font-semibold text-accent hover:underline"
                >
                  Open company site
                </a>
              ) : null}
              {message.company_domain ? (
                <span className="text-ink-faint"> · {message.company_domain}</span>
              ) : null}
            </p>
          )}
        </div>
        <div className="flex flex-wrap gap-2">
          {(message.status === "draft" || message.status === "approved") && (
            <>
              <Button disabled={busy} onClick={() => setEditing((value) => !value)}>
                {editing ? "Close editor" : "Edit"}
              </Button>
              <Button onClick={() => setPreviewOpen(true)}>Preview</Button>
            </>
          )}
          {(message.status === "draft" || message.status === "approved" || message.status === "failed") &&
            message.contact_id > 0 && (
              <Button
                disabled={busy}
                onClick={() =>
                  run(async () => {
                    await api.draftContactAgain(message.contact_id);
                    onNotice(`Redraft queued for ${message.email}. Watch Activity for progress.`);
                  })
                }
              >
                Redraft
              </Button>
            )}
          {message.status === "draft" && (
            <>
              <Button
                variant="primary"
                disabled={busy}
                onClick={() => run(() => api.approve(message.id))}
              >
                Approve
              </Button>
              <Button
                variant="ghost"
                disabled={busy}
                onClick={() => run(() => api.reject(message.id, false))}
              >
                Skip
              </Button>
              <Button
                variant="danger"
                disabled={busy}
                onClick={() => run(() => api.reject(message.id, true))}
              >
                Skip and never contact
              </Button>
            </>
          )}
          {message.status === "failed" && (
            <Button disabled={busy} onClick={() => run(() => api.approve(message.id))}>
              Retry
            </Button>
          )}
        </div>
      </header>

      {message.status === "approved" && (
        <div className="mt-3">
          <Notice tone="warn">{scheduleLabel(schedule, now)}</Notice>
        </div>
      )}

      {editing ? (
        <div className="mt-4 space-y-3">
          <EmailCopyEditor
            subject={subject}
            body={body}
            onSubjectChange={setSubject}
            onBodyChange={setBody}
          />
          <div className="flex flex-wrap gap-2">
            <Button
              variant="primary"
              disabled={busy}
              onClick={() =>
                run(async () => {
                  await api.updateMessage(message.id, subject, body, "");
                  setEditing(false);
                })
              }
            >
              Save
            </Button>
            <Button variant="ghost" onClick={() => setEditing(false)}>
              Cancel
            </Button>
            <Button onClick={() => setPreviewOpen(true)}>Preview send</Button>
          </div>
        </div>
      ) : (
        <div className="mt-4 rounded-2xl border border-line bg-surface-2/70 p-4">
          <div className="text-[15px] font-semibold tracking-tight">{message.subject}</div>
          <pre className="mt-2 font-sans text-[13px] leading-relaxed whitespace-pre-wrap text-ink-soft">
            {message.body}
          </pre>
        </div>
      )}

      {message.personalisation && (
        <p className="mt-3 text-[11px] leading-relaxed text-ink-faint">
          <span className="font-semibold tracking-[0.08em] uppercase">Basis:</span>{" "}
          {message.personalisation}
        </p>
      )}
      {message.error && <p className="mt-2 text-xs text-bad">{message.error}</p>}

      <EmailPreviewModal
        open={previewOpen}
        onClose={() => setPreviewOpen(false)}
        projectId={projectId}
        subject={editing ? subject : message.subject}
        body={editing ? body : message.body}
        renderedText={renderedText || undefined}
        title={`Preview · ${message.email}`}
      />
    </article>
  );
}

export function ReviewPanel({
  projectId,
  refreshKey,
  draftCount = 0,
  health,
  onRefresh,
  onError,
  onNotice,
}: {
  projectId: number | null;
  refreshKey: number;
  /** Project-wide draft count (same as sidebar badge). */
  draftCount?: number;
  health: Health | null;
  projectEmailFormat?: string;
  onRefresh: () => void;
  onError: (message: string) => void;
  onNotice: (message: string) => void;
}) {
  const [status, setStatus] = useState<(typeof TABS)[number]>("draft");
  const [messages, setMessages] = useState<Message[]>([]);
  const [busy, setBusy] = useState(false);
  const [now, setNow] = useState(() => Date.now());

  const load = useCallback(() => {
    if (projectId === null) return;
    api
      .messages(projectId, status)
      .then((payload) => setMessages(payload.messages))
      .catch((error) => onError(error instanceof Error ? error.message : "Failed to load messages"));
  }, [projectId, status, onError]);

  useEffect(load, [load, refreshKey]);

  const hasSchedule = useMemo(
    () =>
      status === "approved" &&
      messages.some((message) => message.status === "approved" || message.sendSchedule),
    [messages, status],
  );

  useEffect(() => {
    if (!hasSchedule) return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [hasSchedule]);

  useEffect(() => {
    if (status !== "approved" || !hasSchedule) return;
    const timer = setInterval(load, 5000);
    return () => clearInterval(timer);
  }, [status, hasSchedule, load]);

  if (projectId === null) return <Empty>Create a campaign first.</Empty>;

  const sendingLive = health?.capabilities.sendingLive ?? false;
  const nextDue = messages
    .map((message) => message.sendSchedule?.runAfter)
    .filter((value): value is string => Boolean(value))
    .map((value) => parseUtc(value).getTime())
    .sort((a, b) => a - b)[0];

  return (
    <div className="space-y-5">
      <Card
        title="Review queue"
        action={
          <div className="flex flex-wrap gap-2">
            <Button
              disabled={busy}
              onClick={async () => {
                setBusy(true);
                try {
                  const result = await api.redraftAll(projectId);
                  onNotice(result.note);
                  setStatus("draft");
                  onRefresh();
                } catch (error) {
                  onError(error instanceof Error ? error.message : "Failed to queue redraft");
                } finally {
                  setBusy(false);
                }
              }}
            >
              Redraft all emails
            </Button>
            {status === "draft" && draftCount > 0 && (
              <Button
                variant="primary"
                disabled={busy}
                onClick={async () => {
                  setBusy(true);
                  try {
                    const result = await api.approveAll(projectId);
                    onNotice(`Approved ${result.approved} message(s). Queued for send.`);
                    setStatus("approved");
                    onRefresh();
                  } catch (error) {
                    onError(error instanceof Error ? error.message : "Failed");
                  } finally {
                    setBusy(false);
                  }
                }}
              >
                Approve all {draftCount}
              </Button>
            )}
          </div>
        }
      >
        <div className="mb-4 space-y-2.5">
          <Notice tone="info">
            Jobs: email mode is set on Audiences (lane template vs tailor-from-JD). Customers /
            investors follow the audience template. Use Redraft on one row, or Redraft all after
            you change mode or a template. Sends are plain text only.
          </Notice>

          {!sendingLive && (
            <Notice tone="info">
              Sender is in dry-run mode. Approving queues a message and prints it to the server log,
              but nothing is delivered. Set <code>SENDER_PROVIDER</code> when you are ready.
            </Notice>
          )}

          {health && (
            <p className="rounded-2xl border border-line bg-surface-2/70 px-3.5 py-2.5 text-xs text-ink-soft">
              Send policy: {health.sender.window}
              {health.sender.sendOnWeekends ? ", weekends on" : ", weekends off"} · max{" "}
              {health.sender.dailyCap}/day · {health.sender.minMinutesBetweenSends} min between
              sends.
            </p>
          )}

          {status === "approved" && messages.length > 0 && (
            <Notice tone="warn">
              {messages.length} approved message{messages.length === 1 ? "" : "s"} are queued.
              {nextDue
                ? ` Next attempt ${formatWhen(new Date(nextDue).toISOString())} (in ${formatCountdown(nextDue - now)}).`
                : " Waiting for the runner to pick them up."}{" "}
              This tab refreshes while sends are pending.
            </Notice>
          )}
        </div>

        <div className="mb-5 flex flex-wrap gap-1.5">
          {TABS.map((tab) => (
            <button
              key={tab}
              type="button"
              onClick={() => setStatus(tab)}
              className={`rounded-full px-3.5 py-1.5 text-xs font-semibold capitalize transition-all duration-200 ${
                status === tab
                  ? "bg-accent text-white shadow-[var(--shadow-glow)]"
                  : "bg-surface-2 text-ink-soft hover:bg-surface-3 hover:text-ink"
              }`}
            >
              {tab}
            </button>
          ))}
        </div>

        {messages.length === 0 ? (
          <Empty>Nothing in “{status}”.</Empty>
        ) : (
          <div className="space-y-3">
            {messages.map((message) => (
              <MessageRow
                key={message.id}
                projectId={projectId}
                message={message}
                now={now}
                onError={onError}
                onNotice={onNotice}
                onChanged={() => {
                  load();
                  onRefresh();
                }}
              />
            ))}
          </div>
        )}
      </Card>
    </div>
  );
}
