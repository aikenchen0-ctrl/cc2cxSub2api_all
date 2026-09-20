import type { Overview } from "../lib/api";

type Metric = {
  label: string;
  value: string | number;
  emphasize?: boolean;
};

function MetricCell({ label, value, emphasize }: Metric) {
  return (
    <div className="min-w-0 px-1 py-1">
      <div className="text-[10px] font-semibold tracking-[0.1em] text-ink-faint uppercase">
        {label}
      </div>
      <div
        className={`mt-1 tabular-nums tracking-tight ${
          emphasize ? "text-xl font-bold text-accent" : "text-lg font-semibold text-ink"
        }`}
      >
        {value}
      </div>
    </div>
  );
}

function Group({
  title,
  metrics,
}: {
  title: string;
  metrics: Metric[];
}) {
  return (
    <div className="min-w-0">
      <p className="mb-3 text-[10px] font-bold tracking-[0.14em] text-ink-faint uppercase">
        {title}
      </p>
      <div className="grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-3">
        {metrics.map((metric) => (
          <MetricCell key={metric.label} {...metric} />
        ))}
      </div>
    </div>
  );
}

export function OverviewStats({ overview }: { overview: Overview }) {
  const messages = overview.counts.messages ?? {};

  return (
    <section className="overflow-hidden rounded-[var(--radius-card)] border border-line bg-surface shadow-[var(--shadow-soft)]">
      <div className="grid gap-0 lg:grid-cols-[1.1fr_1.2fr_1fr]">
        <div className="p-5 sm:p-6">
          <Group
            title="Discovery"
            metrics={[
              { label: "Pages read", value: overview.pagesCrawled },
              { label: "Audiences", value: overview.audiences.length },
              { label: "Targets", value: overview.counts.companies },
            ]}
          />
        </div>

        <div className="border-t border-line p-5 sm:border-t-0 sm:border-l sm:p-6">
          <Group
            title="Pipeline"
            metrics={[
              { label: "Contacts", value: overview.counts.contacts },
              {
                label: "Unsent",
                value: overview.counts.sendableUnsentContacts,
                emphasize: overview.counts.sendableUnsentContacts > 0,
              },
              {
                label: "Awaiting review",
                value: messages.draft ?? 0,
                emphasize: (messages.draft ?? 0) > 0,
              },
            ]}
          />
        </div>

        <div className="border-t border-line bg-gradient-to-br from-surface-2/80 to-surface p-5 sm:p-6 lg:border-t-0 lg:border-l">
          <Group
            title="Sending"
            metrics={[
              { label: "Sent today", value: overview.counts.sentLast24h },
              { label: "Sent total", value: overview.counts.sentTotal },
              {
                label: "Spend",
                value: `$${overview.estimatedSpendUsd.toFixed(3)}`,
              },
            ]}
          />
        </div>
      </div>
    </section>
  );
}
