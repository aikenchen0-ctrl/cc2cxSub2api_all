import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

export type ChipTone =
  | "neutral"
  | "info"
  | "titles"
  | "industries"
  | "sizes"
  | "features"
  | "platforms"
  | "competitors"
  | "pain"
  | "good"
  | "warn"
  | "bad";

const chipStyles: Record<ChipTone, string> = {
  neutral: "bg-surface-2 text-ink-soft border border-line",
  info: "bg-accent-soft text-accent border border-accent/10",
  titles: "bg-accent-soft text-accent border border-accent/15",
  industries: "bg-teal-soft text-teal border border-teal/15",
  sizes: "bg-warn-soft text-warn border border-warn/15",
  features: "bg-violet-soft text-violet border border-violet/15",
  platforms: "bg-sky-soft text-sky border border-sky/15",
  competitors: "bg-bad-soft text-bad border border-bad/10",
  pain: "bg-warn-soft text-ink border border-warn/20",
  good: "bg-good-soft text-good border border-good/15",
  warn: "bg-warn-soft text-warn border border-warn/15",
  bad: "bg-bad-soft text-bad border border-bad/15",
};

const pillTone = {
  neutral: "bg-surface-2 text-ink-soft border border-line",
  good: "bg-good-soft text-good border border-good/15",
  warn: "bg-warn-soft text-warn border border-warn/15",
  bad: "bg-bad-soft text-bad border border-bad/15",
  info: "bg-accent-soft text-accent border border-accent/15",
} as const;

export function Card({
  title,
  action,
  children,
  className = "",
}: {
  title?: string;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section
      className={`rounded-[var(--radius-card)] border border-line bg-surface shadow-[var(--shadow-soft)] ${className}`}
    >
      {(title || action) && (
        <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-3.5 sm:px-6">
          {title && (
            <h2 className="text-[15px] font-semibold tracking-tight text-ink">{title}</h2>
          )}
          {action}
        </header>
      )}
      <div className="p-5 sm:p-6">{children}</div>
    </section>
  );
}

export function Button({
  children,
  onClick,
  variant = "default",
  disabled,
  title,
  className = "",
}: {
  children: ReactNode;
  onClick?: () => void;
  variant?: "default" | "primary" | "danger" | "ghost";
  disabled?: boolean;
  title?: string;
  className?: string;
}) {
  const styles = {
    default:
      "bg-surface border-line text-ink hover:border-line-strong hover:bg-surface-2",
    primary:
      "bg-accent hover:bg-accent-hover text-white border-transparent shadow-[var(--shadow-glow)]",
    danger: "bg-bad-soft hover:bg-bad/15 text-bad border-bad/15",
    ghost: "bg-transparent hover:bg-surface-2 text-ink-soft border-transparent",
  }[variant];

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title}
      className={`inline-flex items-center justify-center gap-1.5 rounded-full border px-3.5 py-2 text-xs font-semibold tracking-tight transition-all duration-200 active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-40 disabled:active:scale-100 ${styles} ${className}`}
    >
      {children}
    </button>
  );
}

export function Pill({
  children,
  tone = "neutral",
  className = "",
  onClick,
  active,
  title,
}: {
  children: ReactNode;
  tone?: keyof typeof pillTone;
  className?: string;
  onClick?: () => void;
  active?: boolean;
  title?: string;
}) {
  const base = `inline-flex max-w-full items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-semibold leading-4 tracking-tight ${pillTone[tone]}`;
  const interactive = onClick
    ? `cursor-pointer transition-all hover:brightness-[0.98] ${
        active ? "ring-2 ring-accent/35 ring-offset-2 ring-offset-surface" : ""
      }`
    : "";

  if (onClick) {
    return (
      <button
        type="button"
        title={title}
        onClick={onClick}
        className={`${base} ${interactive} ${className}`}
      >
        {children}
      </button>
    );
  }

  return <span className={`${base} ${className}`}>{children}</span>;
}

export function Field({
  label,
  hint,
  children,
  accent,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
  accent?: "accent" | "teal" | "sky" | "warn" | "violet" | "bad";
}) {
  const dot = {
    accent: "bg-accent",
    teal: "bg-teal",
    sky: "bg-sky",
    warn: "bg-warn",
    violet: "bg-violet",
    bad: "bg-bad",
  } as const;

  return (
    <div className="min-w-0">
      <div className="mb-1.5 flex items-center gap-2 text-[11px] font-semibold tracking-[0.06em] text-ink-faint uppercase">
        {accent && <span className={`h-1.5 w-1.5 rounded-full ${dot[accent]}`} />}
        {label}
      </div>
      {children}
      {hint && <p className="mt-1.5 text-[11px] leading-relaxed text-ink-faint">{hint}</p>}
    </div>
  );
}

export const inputClass =
  "w-full rounded-[var(--radius-control)] border border-line bg-surface px-3.5 py-2 text-sm text-ink placeholder:text-ink-faint outline-none transition-colors focus:border-accent/40 focus:ring-4 focus:ring-accent/10";

export function Empty({ children }: { children: ReactNode }) {
  return (
    <p className="rounded-[var(--radius-control)] border border-dashed border-line-strong bg-surface-2/50 px-5 py-8 text-center text-sm text-ink-faint">
      {children}
    </p>
  );
}

export function TagList({
  items,
  tone = "neutral",
}: {
  items: string[];
  tone?: ChipTone;
}) {
  if (items.length === 0) return <span className="text-xs text-ink-faint">—</span>;

  return (
    <ul className="m-0 flex list-none flex-wrap gap-1.5 p-0">
      {items.map((item) => (
        <li key={item} className="max-w-full min-w-0">
          <span
            title={item}
            className={`inline-block max-w-full rounded-md px-2 py-1 text-[11px] leading-snug font-medium break-words ${chipStyles[tone]}`}
          >
            {item}
          </span>
        </li>
      ))}
    </ul>
  );
}

export function StackList({
  items,
  tone = "pain",
}: {
  items: string[];
  tone?: "pain" | "features" | "neutral";
}) {
  if (items.length === 0) return <span className="text-xs text-ink-faint">—</span>;

  const row =
    tone === "pain"
      ? "border-warn/20 bg-warn-soft/70"
      : tone === "features"
        ? "border-violet/15 bg-violet-soft/60"
        : "border-line bg-surface-2";

  return (
    <ul className="m-0 space-y-1.5 p-0">
      {items.map((item, index) => (
        <li
          key={item}
          className={`flex gap-2.5 rounded-xl border px-3 py-2 text-[13px] leading-relaxed text-ink ${row}`}
        >
          <span className="mt-0.5 w-5 shrink-0 text-[11px] font-semibold tabular-nums text-ink-faint">
            {String(index + 1).padStart(2, "0")}
          </span>
          <span className="min-w-0">{item}</span>
        </li>
      ))}
    </ul>
  );
}

const disclosureTone = {
  accent: "border-accent/20 bg-accent-soft/50 hover:bg-accent-soft",
  sky: "border-sky/20 bg-sky-soft/60 hover:bg-sky-soft",
  warn: "border-warn/25 bg-warn-soft/60 hover:bg-warn-soft",
  teal: "border-teal/20 bg-teal-soft/50 hover:bg-teal-soft",
} as const;

export function Disclosure({
  title,
  count,
  tone = "accent",
  children,
  defaultOpen = false,
}: {
  title: string;
  count?: number;
  tone?: keyof typeof disclosureTone;
  children: ReactNode;
  defaultOpen?: boolean;
}) {
  return (
    <details
      className={`group rounded-2xl border transition-colors ${disclosureTone[tone]}`}
      open={defaultOpen || undefined}
    >
      <summary className="flex cursor-pointer list-none items-center justify-between gap-3 px-3.5 py-2.5 marker:content-none [&::-webkit-details-marker]:hidden">
        <span className="inline-flex items-center gap-2 text-[13px] font-semibold text-ink">
          <span className="text-ink-faint transition-transform group-open:rotate-90">›</span>
          {title}
          {count != null && (
            <span className="rounded-full bg-surface/80 px-2 py-0.5 text-[11px] font-semibold tabular-nums text-ink-soft">
              {count}
            </span>
          )}
        </span>
        <span className="text-[11px] font-medium text-ink-faint group-open:hidden">Show</span>
        <span className="hidden text-[11px] font-medium text-ink-faint group-open:inline">Hide</span>
      </summary>
      <div className="border-t border-inherit px-4 pt-3 pb-4">{children}</div>
    </details>
  );
}

export function Notice({
  children,
  tone = "info",
}: {
  children: ReactNode;
  tone?: "info" | "warn" | "bad";
}) {
  const styles = {
    info: "border-accent/20 bg-accent-soft text-accent",
    warn: "border-warn/25 bg-warn-soft text-warn",
    bad: "border-bad/20 bg-bad-soft text-bad",
  }[tone];
  return (
    <div className={`rounded-[var(--radius-control)] border px-3.5 py-2.5 text-xs leading-relaxed ${styles}`}>
      {children}
    </div>
  );
}

export function StatusBar({
  live,
  fromEmail,
  sentToday,
  dailyCap,
}: {
  live?: boolean;
  fromEmail?: string;
  sentToday?: number;
  dailyCap?: number | string;
}) {
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-2xl border border-line bg-surface px-3.5 py-2 shadow-[var(--shadow-soft)]">
      {live != null && (
        <div className="flex items-center gap-2 text-xs font-semibold text-ink">
          <span
            className={`h-2 w-2 shrink-0 rounded-full ${live ? "live-dot bg-warn" : "bg-accent"}`}
          />
          <span>{live ? "Live" : "Dry run"}</span>
          {live && fromEmail && (
            <span className="hidden font-medium text-ink-soft sm:inline">as {fromEmail}</span>
          )}
        </div>
      )}
      {sentToday != null && (
        <>
          <span className="hidden h-4 w-px bg-line sm:block" />
          <div className="text-xs font-semibold tabular-nums text-ink">
            <span className="text-accent">{sentToday}</span>
            <span className="text-ink-faint"> / {dailyCap ?? "?"} </span>
            <span className="font-medium text-ink-soft">sent today</span>
          </div>
        </>
      )}
    </div>
  );
}

/** Tinted panel for analysis sections */
export function InfoPanel({
  children,
  tone = "accent",
  className = "",
}: {
  children: ReactNode;
  tone?: "accent" | "teal" | "sky" | "warn" | "violet" | "bad";
  className?: string;
}) {
  const styles = {
    accent: "border-accent/15 bg-accent-soft/40",
    teal: "border-teal/15 bg-teal-soft/40",
    sky: "border-sky/15 bg-sky-soft/40",
    warn: "border-warn/20 bg-warn-soft/40",
    violet: "border-violet/15 bg-violet-soft/40",
    bad: "border-bad/15 bg-bad-soft/40",
  }[tone];
  return <div className={`rounded-2xl border p-4 ${styles} ${className}`}>{children}</div>;
}

/** Generic modal shell (no confirm/cancel footer). */
export function Modal({
  open,
  title,
  description,
  wide = false,
  children,
  onClose,
}: {
  open: boolean;
  title: string;
  description?: ReactNode;
  wide?: boolean;
  children: ReactNode;
  onClose: () => void;
}) {
  const titleId = useId();

  useEffect(() => {
    if (!open) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => {
      document.body.style.overflow = previous;
      window.removeEventListener("keydown", onKeyDown);
    };
  }, [open, onClose]);

  if (!open || typeof document === "undefined") return null;

  return createPortal(
    <div className="fixed inset-0 z-[1000] flex items-center justify-center p-4">
      <button
        type="button"
        aria-label="Close dialog"
        className="absolute inset-0 bg-ink/40 backdrop-blur-[2px]"
        onClick={onClose}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className={`relative z-10 flex max-h-[90dvh] w-full flex-col overflow-hidden rounded-[var(--radius-card)] border border-line bg-surface shadow-[var(--shadow-lift)] ${
          wide ? "max-w-3xl" : "max-w-lg"
        }`}
      >
        <header className="flex shrink-0 items-start justify-between gap-3 border-b border-line px-5 py-4 sm:px-6">
          <div className="min-w-0">
            <h3 id={titleId} className="text-[15px] font-semibold tracking-tight text-ink">
              {title}
            </h3>
            {description && (
              <div className="mt-1 text-[12px] leading-relaxed text-ink-faint">{description}</div>
            )}
          </div>
          <button
            type="button"
            onClick={onClose}
            className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-ink-faint hover:bg-surface-2 hover:text-ink"
            aria-label="Close"
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden>
              <path
                d="M3 3l8 8M11 3l-8 8"
                stroke="currentColor"
                strokeWidth="1.6"
                strokeLinecap="round"
              />
            </svg>
          </button>
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4 sm:px-6 sm:py-5">{children}</div>
      </div>
    </div>,
    document.body,
  );
}

export type MenuItem = {
  label: string;
  onClick: () => void;
  danger?: boolean;
  disabled?: boolean;
};

export function KebabMenu({ items, align = "right" }: { items: MenuItem[]; align?: "left" | "right" }) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const onPointer = (event: MouseEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
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
    <div ref={rootRef} className="relative shrink-0">
      <button
        type="button"
        aria-label="More actions"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className="flex h-9 w-9 items-center justify-center rounded-full border border-line bg-surface text-ink-soft transition-colors hover:bg-surface-2 hover:text-ink"
      >
        <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden>
          <circle cx="8" cy="3.5" r="1.35" />
          <circle cx="8" cy="8" r="1.35" />
          <circle cx="8" cy="12.5" r="1.35" />
        </svg>
      </button>
      {open && (
        <div
          className={`absolute top-full z-40 mt-1.5 min-w-[180px] overflow-hidden rounded-xl border border-line bg-surface py-1 shadow-[var(--shadow-lift)] ${
            align === "right" ? "right-0" : "left-0"
          }`}
        >
          {items.map((item) => (
            <button
              key={item.label}
              type="button"
              disabled={item.disabled}
              onClick={() => {
                if (item.disabled) return;
                setOpen(false);
                item.onClick();
              }}
              className={`block w-full px-3.5 py-2 text-left text-[13px] font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${
                item.danger
                  ? "text-bad hover:bg-bad-soft"
                  : "text-ink hover:bg-surface-2"
              }`}
            >
              {item.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  danger = false,
  busy = false,
  confirmDisabled = false,
  children,
  onConfirm,
  onCancel,
}: {
  open: boolean;
  title: string;
  description?: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
  busy?: boolean;
  confirmDisabled?: boolean;
  children?: ReactNode;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const titleId = useId();

  useEffect(() => {
    if (!open) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busy) onCancel();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => {
      document.body.style.overflow = previous;
      window.removeEventListener("keydown", onKeyDown);
    };
  }, [open, busy, onCancel]);

  if (!open || typeof document === "undefined") return null;

  // Portal to body so fixed positioning isn't trapped by transformed/overflow
  // ancestors (e.g. main.animate-rise).
  return createPortal(
    <div className="fixed inset-0 z-[1000] flex items-center justify-center p-4">
      <button
        type="button"
        aria-label="Close dialog"
        className="absolute inset-0 bg-ink/40 backdrop-blur-[2px]"
        onClick={busy ? undefined : onCancel}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className="relative z-10 w-full max-w-lg rounded-[var(--radius-card)] border border-line bg-surface p-5 shadow-[var(--shadow-lift)] sm:p-6"
      >
        <h3 id={titleId} className="text-[15px] font-semibold tracking-tight text-ink">
          {title}
        </h3>
        {description && (
          <div className="mt-2 text-[13px] leading-relaxed text-ink-soft">{description}</div>
        )}
        {children && <div className="mt-4 space-y-3">{children}</div>}
        <div className="mt-5 flex flex-wrap justify-end gap-2">
          <Button variant="ghost" disabled={busy} onClick={onCancel}>
            {cancelLabel}
          </Button>
          <Button
            variant={danger ? "danger" : "primary"}
            disabled={busy || confirmDisabled}
            onClick={onConfirm}
          >
            {busy ? "Working…" : confirmLabel}
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

export function CheckboxRow({
  checked,
  onChange,
  label,
  hint,
  disabled,
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label: string;
  hint?: string;
  disabled?: boolean;
}) {
  return (
    <label
      className={`flex cursor-pointer gap-3 rounded-xl border border-line bg-surface-2/60 px-3.5 py-3 ${
        disabled ? "cursor-not-allowed opacity-50" : "hover:bg-surface-2"
      }`}
    >
      <input
        type="checkbox"
        className="mt-0.5 h-4 w-4 shrink-0 accent-[var(--color-accent)]"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span className="min-w-0">
        <span className="block text-[13px] font-semibold text-ink">{label}</span>
        {hint && <span className="mt-0.5 block text-[11px] leading-relaxed text-ink-faint">{hint}</span>}
      </span>
    </label>
  );
}

/** Plain “will / won’t” checklist for confirm dialogs. */
export function WhatHappensList({
  will,
  willNot,
}: {
  will: string[];
  willNot: string[];
}) {
  return (
    <div className="rounded-xl border border-line bg-surface-2/50 px-3.5 py-3">
      <p className="text-[10px] font-bold tracking-[0.08em] text-ink-faint uppercase">
        What will happen
      </p>
      <ul className="mt-2 space-y-1.5 text-[12px] leading-snug">
        {will.map((item) => (
          <li key={`will-${item}`} className="flex gap-2 text-ink">
            <span className="mt-0.5 shrink-0 font-semibold text-good" aria-hidden>
              ✓
            </span>
            <span>{item}</span>
          </li>
        ))}
        {willNot.map((item) => (
          <li key={`wont-${item}`} className="flex gap-2 text-ink-soft">
            <span className="mt-0.5 shrink-0 font-semibold text-ink-faint" aria-hidden>
              –
            </span>
            <span>{item}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
