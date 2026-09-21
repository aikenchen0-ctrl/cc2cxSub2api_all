import { useEffect, useImperativeHandle, useRef, useState, type Ref } from "react";
import {
  api,
  type Competitor,
  type PricingPlan,
  type SiteAnalysis,
  type UseCase,
  type WorkflowType,
} from "../lib/api";
import { Button, Field, InfoPanel, TagList, inputClass } from "./ui";

export type AnalysisEditorHandle = {
  save: () => Promise<void>;
  cancel: () => void;
};

function competitorHref(domain: string): string | null {
  const clean = domain
    .trim()
    .toLowerCase()
    .replace(/^https?:\/\//, "")
    .replace(/^www\./, "")
    .replace(/\/.*$/, "");
  if (!/^[a-z0-9-]+(\.[a-z0-9-]+)+$/.test(clean)) return null;
  return `https://${clean}`;
}

function EmptyValue({ children = "—" }: { children?: string }) {
  return <span className="text-xs text-ink-faint">{children}</span>;
}

function ProblemList({ items, limit = 8 }: { items: string[]; limit?: number }) {
  const [expanded, setExpanded] = useState(false);
  if (items.length === 0) return <EmptyValue />;

  const visible = expanded ? items : items.slice(0, limit);

  return (
    <div className="space-y-2">
      <ul className="m-0 grid list-none gap-1.5 p-0 md:grid-cols-2">
        {visible.map((item, index) => (
          <li
            key={`${item}-${index}`}
            className="flex gap-2.5 rounded-xl border border-warn/20 bg-warn-soft/70 px-3 py-2 text-[13px] leading-snug text-ink"
          >
            <span className="mt-0.5 w-5 shrink-0 text-[11px] font-semibold tabular-nums text-ink-faint">
              {String(index + 1).padStart(2, "0")}
            </span>
            <span className="min-w-0">{item}</span>
          </li>
        ))}
      </ul>
      {items.length > limit && (
        <button
          type="button"
          className="text-xs font-semibold text-accent hover:underline"
          onClick={() => setExpanded((value) => !value)}
        >
          {expanded ? "Show fewer" : `Show all ${items.length}`}
        </button>
      )}
    </div>
  );
}

function CompetitorList({ items }: { items: Competitor[] }) {
  if (items.length === 0) return <EmptyValue />;

  return (
    <ul className="m-0 flex list-none flex-col gap-1.5 p-0">
      {items.map((item) => {
        const href = competitorHref(item.domain);
        const className =
          "inline-flex max-w-full items-baseline gap-2 rounded-md border border-bad/15 bg-bad-soft/60 px-2.5 py-1.5 text-[12px] leading-snug font-medium";

        return (
          <li key={`${item.name}-${item.domain}`} className="max-w-full min-w-0">
            {href ? (
              <a
                href={href}
                target="_blank"
                rel="noreferrer"
                title={href}
                className={`${className} text-ink underline-offset-2 transition-colors hover:bg-bad-soft hover:underline`}
              >
                <span className="min-w-0 break-words font-semibold">{item.name}</span>
                <span className="min-w-0 break-all text-[11px] font-normal text-ink-faint">
                  {item.domain}
                </span>
              </a>
            ) : (
              <span className={`${className} text-ink`}>
                <span className="min-w-0 break-words font-semibold">{item.name}</span>
                <span className="shrink-0 text-[11px] font-normal text-ink-faint">
                  website missing — re-scan to resolve
                </span>
              </span>
            )}
          </li>
        );
      })}
    </ul>
  );
}

function StringListEditor({
  items,
  onChange,
  placeholder,
}: {
  items: string[];
  onChange: (items: string[]) => void;
  placeholder: string;
}) {
  return (
    <div className="space-y-2">
      {items.map((item, index) => (
        <div key={index} className="flex gap-2">
          <input
            className={inputClass}
            value={item}
            placeholder={placeholder}
            onChange={(event) => {
              const next = [...items];
              next[index] = event.target.value;
              onChange(next);
            }}
          />
          <Button
            variant="ghost"
            title="Remove"
            onClick={() => onChange(items.filter((_, i) => i !== index))}
          >
            Remove
          </Button>
        </div>
      ))}
      <Button variant="ghost" onClick={() => onChange([...items, ""])}>
        Add
      </Button>
    </div>
  );
}

function PricingCards({ plans, fallback }: { plans: PricingPlan[]; fallback: string }) {
  if (plans.length === 0) {
    return (
      <p className="text-sm leading-relaxed text-ink-soft">
        {fallback?.trim() && !/^not stated$/i.test(fallback) ? fallback : "No pricing listed."}
      </p>
    );
  }

  return (
    <div className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-3">
      {plans.map((plan, index) => (
        <div
          key={`${plan.name}-${index}`}
          className="flex h-full flex-col rounded-2xl border border-teal/20 bg-surface/80 px-3.5 py-3"
        >
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div className="text-[10px] font-semibold tracking-[0.08em] text-teal uppercase">
              {plan.name || "Plan"}
            </div>
            {plan.trial ? (
              <span className="rounded-full bg-teal-soft px-2 py-0.5 text-[10px] font-semibold text-teal">
                {plan.trial}
              </span>
            ) : null}
          </div>
          <div className="mt-1 text-base font-semibold tracking-tight text-ink">{plan.price || "—"}</div>
          {plan.note ? (
            <p className="mt-1 text-xs leading-relaxed text-ink-soft">{plan.note}</p>
          ) : null}
          {(plan.highlights?.length ?? 0) > 0 && (
            <ul className="mt-2.5 space-y-1 border-t border-line/70 pt-2.5">
              {plan.highlights.map((item) => (
                <li key={item} className="flex gap-2 text-xs leading-snug text-ink-soft">
                  <span className="mt-1.5 h-1 w-1 shrink-0 rounded-full bg-teal" />
                  <span>{item}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
      ))}
    </div>
  );
}

function PricingEditor({
  plans,
  onChange,
}: {
  plans: PricingPlan[];
  onChange: (plans: PricingPlan[]) => void;
}) {
  return (
    <div className="space-y-3">
      {plans.map((plan, index) => (
        <div key={index} className="space-y-2 rounded-2xl border border-line bg-surface/70 p-3">
          <div className="grid gap-2 sm:grid-cols-2">
            <input
              className={inputClass}
              placeholder="Plan name (Free, Pro…)"
              value={plan.name}
              onChange={(event) => {
                const next = [...plans];
                next[index] = { ...plan, name: event.target.value };
                onChange(next);
              }}
            />
            <input
              className={inputClass}
              placeholder="Price ($25/mo, Free, Custom)"
              value={plan.price}
              onChange={(event) => {
                const next = [...plans];
                next[index] = { ...plan, price: event.target.value };
                onChange(next);
              }}
            />
            <input
              className={inputClass}
              placeholder="Trial (14-day free trial, Free forever…)"
              value={plan.trial}
              onChange={(event) => {
                const next = [...plans];
                next[index] = { ...plan, trial: event.target.value };
                onChange(next);
              }}
            />
            <input
              className={inputClass}
              placeholder="Note (seats, annual billing…)"
              value={plan.note}
              onChange={(event) => {
                const next = [...plans];
                next[index] = { ...plan, note: event.target.value };
                onChange(next);
              }}
            />
          </div>
          <Field label="What this plan includes / how it differs" hint="One highlight per line.">
            <textarea
              className={`${inputClass} h-24 resize-y`}
              placeholder={"Unlimited meetings\nPriority support\nTeam seats"}
              value={(plan.highlights ?? []).join("\n")}
              onChange={(event) => {
                const next = [...plans];
                next[index] = {
                  ...plan,
                  highlights: event.target.value
                    .split("\n")
                    .map((line) => line.trim())
                    .filter(Boolean),
                };
                onChange(next);
              }}
            />
          </Field>
          <Button variant="ghost" onClick={() => onChange(plans.filter((_, i) => i !== index))}>
            Remove plan
          </Button>
        </div>
      ))}
      <Button
        variant="ghost"
        onClick={() =>
          onChange([
            ...plans,
            { name: "", price: "", note: "", trial: "", highlights: [] },
          ])
        }
      >
        Add plan
      </Button>
    </div>
  );
}

function CompetitorsEditor({
  items,
  onChange,
}: {
  items: Competitor[];
  onChange: (items: Competitor[]) => void;
}) {
  return (
    <div className="space-y-2">
      {items.map((item, index) => (
        <div key={index} className="grid gap-2 sm:grid-cols-[1.2fr_1fr_auto]">
          <input
            className={inputClass}
            placeholder="Competitor name"
            value={item.name}
            onChange={(event) => {
              const next = [...items];
              next[index] = { ...item, name: event.target.value };
              onChange(next);
            }}
          />
          <input
            className={inputClass}
            placeholder="domain.com or https://…"
            value={item.domain}
            onChange={(event) => {
              const next = [...items];
              next[index] = { ...item, domain: event.target.value };
              onChange(next);
            }}
          />
          <Button
            variant="ghost"
            onClick={() => onChange(items.filter((_, i) => i !== index))}
          >
            Remove
          </Button>
        </div>
      ))}
      <Button
        variant="ghost"
        onClick={() => onChange([...items, { name: "", domain: "" }])}
      >
        Add competitor
      </Button>
    </div>
  );
}

function UseCasesEditor({
  items,
  onChange,
}: {
  items: UseCase[];
  onChange: (items: UseCase[]) => void;
}) {
  return (
    <div className="space-y-3">
      {items.map((item, index) => (
        <div key={index} className="space-y-2 rounded-2xl border border-line bg-surface/70 p-3">
          <input
            className={inputClass}
            placeholder="Use case title"
            value={item.title}
            onChange={(event) => {
              const next = [...items];
              next[index] = { ...item, title: event.target.value };
              onChange(next);
            }}
          />
          <textarea
            className={`${inputClass} h-20 resize-y`}
            placeholder="What happens in this use case / the outcome"
            value={item.description}
            onChange={(event) => {
              const next = [...items];
              next[index] = { ...item, description: event.target.value };
              onChange(next);
            }}
          />
          <input
            className={inputClass}
            placeholder="Who this is for (helps audience targeting)"
            value={item.audienceHint}
            onChange={(event) => {
              const next = [...items];
              next[index] = { ...item, audienceHint: event.target.value };
              onChange(next);
            }}
          />
          <Button variant="ghost" onClick={() => onChange(items.filter((_, i) => i !== index))}>
            Remove use case
          </Button>
        </div>
      ))}
      <Button
        variant="ghost"
        onClick={() =>
          onChange([...items, { title: "", description: "", audienceHint: "" }])
        }
      >
        Add use case
      </Button>
    </div>
  );
}

function cleanDraft(draft: SiteAnalysis): SiteAnalysis {
  const pricingPlans = (draft.pricingPlans ?? [])
    .map((plan) => ({
      name: plan.name.trim(),
      price: plan.price.trim(),
      note: plan.note.trim(),
      trial: (plan.trial ?? "").trim(),
      highlights: (plan.highlights ?? []).map((item) => item.trim()).filter(Boolean),
    }))
    .filter(
      (plan) =>
        plan.name || plan.price || plan.note || plan.trial || plan.highlights.length > 0,
    )
    .map((plan) => ({ ...plan, name: plan.name || "Plan" }));

  const competitors = (draft.competitors ?? [])
    .map((item) => ({
      name: item.name.trim(),
      domain: item.domain
        .trim()
        .toLowerCase()
        .replace(/^https?:\/\//, "")
        .replace(/^www\./, "")
        .replace(/\/.*$/, ""),
    }))
    .filter((item) => item.name);

  const useCases = (draft.useCases ?? [])
    .map((item) => ({
      title: item.title.trim(),
      description: item.description.trim(),
      audienceHint: item.audienceHint.trim(),
    }))
    .filter((item) => item.title || item.description);

  const pricing =
    pricingPlans
      .map((plan) => {
        const bits = [plan.name, plan.price].filter(Boolean);
        if (plan.trial) bits.push(`(${plan.trial})`);
        return bits.join(" ");
      })
      .join(", ") ||
    draft.pricing ||
    "not stated";

  return {
    productName: draft.productName.trim(),
    oneLiner: draft.oneLiner.trim(),
    features: (draft.features ?? []).map((item) => item.trim()).filter(Boolean),
    platforms: (draft.platforms ?? []).map((item) => item.trim()).filter(Boolean),
    painPointsSolved: (draft.painPointsSolved ?? []).map((item) => item.trim()).filter(Boolean),
    competitors,
    pricingPlans,
    pricing,
    useCases,
  };
}

function ensureAnalysisShape(analysis: SiteAnalysis): SiteAnalysis {
  return {
    ...analysis,
    pricingPlans: (analysis.pricingPlans ?? []).map((plan) => ({
      name: plan.name ?? "",
      price: plan.price ?? "",
      note: plan.note ?? "",
      trial: plan.trial ?? "",
      highlights: plan.highlights ?? [],
    })),
    competitors: analysis.competitors ?? [],
    useCases: analysis.useCases ?? [],
    features: analysis.features ?? [],
    platforms: analysis.platforms ?? [],
    painPointsSolved: analysis.painPointsSolved ?? [],
  };
}

export function AnalysisEditor({
  projectId,
  analysis,
  workflowType = "customer_outreach",
  editing,
  onEditingChange,
  onBusyChange,
  editorRef,
  onSaved,
  onError,
  onNotice,
}: {
  projectId: number;
  analysis: SiteAnalysis;
  workflowType?: WorkflowType;
  editing: boolean;
  onEditingChange: (editing: boolean) => void;
  onBusyChange?: (busy: boolean) => void;
  editorRef?: Ref<AnalysisEditorHandle | null>;
  onSaved: () => void;
  onError: (message: string) => void;
  onNotice?: (message: string) => void;
}) {
  const isJob = workflowType === "job_outreach";
  const shaped = ensureAnalysisShape(analysis);
  const [busy, setBusy] = useState(false);
  const [draft, setDraft] = useState<SiteAnalysis>(shaped);
  const draftRef = useRef(draft);
  const analysisRef = useRef(analysis);

  useEffect(() => {
    draftRef.current = draft;
  }, [draft]);

  useEffect(() => {
    analysisRef.current = analysis;
  }, [analysis]);

  useEffect(() => {
    if (!editing) setDraft(ensureAnalysisShape(analysis));
  }, [analysis, editing]);

  useEffect(() => {
    onBusyChange?.(busy);
  }, [busy, onBusyChange]);

  useImperativeHandle(
    editorRef,
    () => ({
      async save() {
        const cleaned = cleanDraft(draftRef.current);
        if (!cleaned.productName) {
          onError(isJob ? "Candidate name is required." : "Product name is required.");
          return;
        }
        setBusy(true);
        try {
          await api.updateAnalysis(projectId, cleaned);
          onEditingChange(false);
          onNotice?.(isJob ? "Candidate profile saved." : "Product analysis saved.");
          onSaved();
        } catch (error) {
          onError(error instanceof Error ? error.message : "Failed to save analysis");
        } finally {
          setBusy(false);
        }
      },
      cancel() {
        setDraft(ensureAnalysisShape(analysisRef.current));
        onEditingChange(false);
      },
    }),
    [projectId, isJob, onEditingChange, onError, onNotice, onSaved],
  );

  if (editing) {
    return (
      <div className="space-y-4">
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={isJob ? "Candidate name" : "Product name"} accent="accent">
            <input
              className={inputClass}
              value={draft.productName}
              onChange={(event) => setDraft({ ...draft, productName: event.target.value })}
            />
          </Field>
          <Field label={isJob ? "Positioning one-liner" : "One-liner"} accent="accent">
            <input
              className={inputClass}
              value={draft.oneLiner}
              onChange={(event) => setDraft({ ...draft, oneLiner: event.target.value })}
            />
          </Field>
        </div>

        <div className="space-y-4">
          <InfoPanel tone="violet">
            <Field label={isJob ? "Skills & strengths" : "Features"} accent="violet">
              <StringListEditor
                items={draft.features}
                onChange={(features) => setDraft({ ...draft, features })}
                placeholder={isJob ? "Skill or strength" : "Feature"}
              />
            </Field>
          </InfoPanel>

          {!isJob && (
            <InfoPanel tone="teal">
              <Field label="Pricing" accent="teal">
                <PricingEditor
                  plans={draft.pricingPlans}
                  onChange={(pricingPlans) => setDraft({ ...draft, pricingPlans })}
                />
              </Field>
            </InfoPanel>
          )}

          <InfoPanel tone="warn">
            <Field
              label={isJob ? "Hiring pains you can help with" : "Problems it solves"}
              accent="warn"
            >
              <StringListEditor
                items={draft.painPointsSolved}
                onChange={(painPointsSolved) => setDraft({ ...draft, painPointsSolved })}
                placeholder={
                  isJob ? "Hiring / team problem you solve" : "Problem this product solves"
                }
              />
            </Field>
          </InfoPanel>

          <div className={`grid gap-4 md:items-stretch ${isJob ? "" : "md:grid-cols-2"}`}>
            <InfoPanel tone="sky" className="h-full">
              <Field label={isJob ? "Tools & tech" : "Platforms"} accent="sky">
                <StringListEditor
                  items={draft.platforms}
                  onChange={(platforms) => setDraft({ ...draft, platforms })}
                  placeholder={isJob ? "Tool or technology" : "Platform"}
                />
              </Field>
            </InfoPanel>

            {!isJob && (
              <InfoPanel tone="bad" className="h-full">
                <Field
                  label="Competitors"
                  accent="bad"
                  hint="Name + website domain (domain.com or full URL)."
                >
                  <CompetitorsEditor
                    items={draft.competitors}
                    onChange={(competitors) => setDraft({ ...draft, competitors })}
                  />
                </Field>
              </InfoPanel>
            )}
          </div>

          <InfoPanel tone="accent">
            <Field
              label={isJob ? "Target role tracks" : "Real-life use cases"}
              accent="accent"
              hint={
                isJob
                  ? "Each track becomes a job-search lane (roles you want × company types)."
                  : "These drive better audience targeting when you regenerate audiences."
              }
            >
              <UseCasesEditor
                items={draft.useCases}
                onChange={(useCases) => setDraft({ ...draft, useCases })}
              />
            </Field>
          </InfoPanel>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="min-w-0">
        <div className="text-lg font-semibold tracking-tight">{shaped.productName}</div>
        <p className="mt-1.5 text-[13px] leading-relaxed text-ink-soft">{shaped.oneLiner}</p>
      </div>

      <div className="space-y-4">
        <InfoPanel tone="violet">
          <Field label={isJob ? "Skills & strengths" : "Features"} accent="violet">
            <TagList items={shaped.features} tone="features" />
          </Field>
        </InfoPanel>

        {!isJob && (
          <InfoPanel tone="teal">
            <Field label="Pricing" accent="teal">
              <PricingCards plans={shaped.pricingPlans} fallback={shaped.pricing} />
            </Field>
          </InfoPanel>
        )}

        <InfoPanel tone="warn">
          <Field
            label={isJob ? "Hiring pains you can help with" : "Problems it solves"}
            accent="warn"
          >
            <ProblemList items={shaped.painPointsSolved} />
          </Field>
        </InfoPanel>

        <div className={`grid gap-4 md:items-stretch ${isJob ? "" : "md:grid-cols-2"}`}>
          <InfoPanel tone="sky" className="h-full">
            <Field label={isJob ? "Tools & tech" : "Platforms"} accent="sky">
              <TagList items={shaped.platforms} tone="platforms" />
            </Field>
          </InfoPanel>

          {!isJob && (
            <InfoPanel tone="bad" className="h-full">
              <Field
                label="Competitors"
                accent="bad"
                hint="Click a name with a website to open it."
              >
                <CompetitorList items={shaped.competitors} />
              </Field>
            </InfoPanel>
          )}
        </div>

        <InfoPanel tone="accent">
          <Field
            label={isJob ? "Target role tracks" : "Real-life use cases"}
            accent="accent"
            hint={
              isJob
                ? "Used when regenerating search lanes so each path matches a credible role track."
                : "Used when regenerating audiences so ICPs match how people actually use the product."
            }
          >
            {shaped.useCases.length === 0 ? (
              <EmptyValue>
                {isJob
                  ? "No role tracks yet. Re-analyse the resume or add them in Edit."
                  : "No use cases yet. Re-scan the site or add them in Edit analysis."}
              </EmptyValue>
            ) : (
              <div className="grid gap-2.5 md:grid-cols-2">
                {shaped.useCases.map((item, index) => (
                  <div
                    key={`${item.title}-${item.audienceHint}-${index}`}
                    className="rounded-2xl border border-accent/15 bg-surface/80 px-3.5 py-3"
                  >
                    <div className="text-[13px] font-semibold tracking-tight text-ink">{item.title}</div>
                    {item.description ? (
                      <p className="mt-1 text-xs leading-relaxed text-ink-soft">{item.description}</p>
                    ) : null}
                    {item.audienceHint ? (
                      <p className="mt-2 text-[11px] font-semibold text-accent">
                        {isJob ? "Lane: " : "For: "}
                        {item.audienceHint}
                      </p>
                    ) : null}
                  </div>
                ))}
              </div>
            )}
          </Field>
        </InfoPanel>
      </div>
    </div>
  );
}
