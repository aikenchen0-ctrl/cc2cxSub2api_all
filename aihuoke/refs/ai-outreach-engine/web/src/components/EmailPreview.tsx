import { useEffect, useState } from "react";
import { api, type EmailPreviewResult } from "../lib/api";
import { Modal } from "./ui";

export function useEmailPreview(args: {
  projectId: number;
  subject: string;
  body: string;
  fillSampleMergeFields?: boolean;
  painPoint?: string;
  valueProp?: string;
  enabled?: boolean;
}) {
  const [preview, setPreview] = useState<EmailPreviewResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (args.enabled === false || !args.body.trim()) {
      setPreview(null);
      setError(null);
      setLoading(false);
      return;
    }

    let cancelled = false;
    setLoading(true);
    const timer = window.setTimeout(() => {
      void api
        .emailPreview(args.projectId, {
          subject: args.subject,
          body: args.body,
          fillSampleMergeFields: args.fillSampleMergeFields,
          painPoint: args.painPoint,
          valueProp: args.valueProp,
        })
        .then((result) => {
          if (cancelled) return;
          setPreview(result);
          setError(null);
        })
        .catch((err: unknown) => {
          if (cancelled) return;
          setError(err instanceof Error ? err.message : "Preview failed");
        })
        .finally(() => {
          if (!cancelled) setLoading(false);
        });
    }, 220);

    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [
    args.projectId,
    args.subject,
    args.body,
    args.fillSampleMergeFields,
    args.painPoint,
    args.valueProp,
    args.enabled,
  ]);

  return { preview, error, loading };
}

export function EmailPreviewModal({
  open,
  onClose,
  projectId,
  subject,
  body,
  renderedText,
  fillSampleMergeFields = false,
  painPoint = "",
  valueProp = "",
  title = "Send preview",
}: {
  open: boolean;
  onClose: () => void;
  projectId: number;
  subject: string;
  body: string;
  htmlBody?: string;
  renderedText?: string;
  renderedHtml?: string | null;
  renderedFormat?: "html" | "plain" | "";
  fillSampleMergeFields?: boolean;
  painPoint?: string;
  valueProp?: string;
  title?: string;
}) {
  const staticPreview: EmailPreviewResult | null = renderedText
    ? {
        format: "plain",
        subject,
        text: renderedText,
        html: null,
        htmlIsCustom: false,
        brand: {
          logoUrl: null,
          productUrl: "",
          primaryColor: "",
          logoMode: "text",
        },
      }
    : null;
  const { preview: livePreview, error, loading } = useEmailPreview({
    projectId,
    subject,
    body,
    fillSampleMergeFields,
    painPoint,
    valueProp,
    enabled: open && !staticPreview,
  });
  const preview = staticPreview ?? livePreview;

  return (
    <Modal
      open={open}
      onClose={onClose}
      wide
      title={title}
      description={
        staticPreview
          ? "Exactly what was recorded at send time."
          : "How this plain-text email will look when sent."
      }
    >
      <div className="space-y-3">
        {error && <p className="text-xs text-bad">{error}</p>}
        {loading && !preview && <p className="text-xs text-ink-faint">Rendering preview…</p>}
        {preview && (
          <div className="rounded-xl border border-line bg-surface-2/60 p-4">
            {preview.subject && (
              <div className="mb-2 text-[13px] font-semibold text-ink">{preview.subject}</div>
            )}
            <pre className="font-sans text-[13px] leading-relaxed whitespace-pre-wrap text-ink-soft">
              {preview.text}
            </pre>
          </div>
        )}
      </div>
    </Modal>
  );
}

/** Plain-text subject + body editor. */
export function EmailCopyEditor({
  subject,
  body,
  onSubjectChange,
  onBodyChange,
}: {
  projectId?: number;
  subject: string;
  body: string;
  htmlBody?: string;
  onSubjectChange: (value: string) => void;
  onBodyChange: (value: string) => void;
  onHtmlBodyChange?: (value: string) => void;
  fillSampleMergeFields?: boolean;
  painPoint?: string;
  valueProp?: string;
}) {
  return (
    <div className="space-y-3">
      <input
        className="w-full rounded-[var(--radius-control)] border border-line bg-surface px-3.5 py-2.5 text-sm font-medium text-ink outline-none focus:ring-2 focus:ring-accent/20"
        value={subject}
        onChange={(event) => onSubjectChange(event.target.value)}
        placeholder="Subject"
      />
      <textarea
        className="h-44 w-full resize-y rounded-[var(--radius-control)] border border-line bg-surface px-3.5 py-2.5 font-mono text-xs leading-relaxed text-ink outline-none focus:ring-2 focus:ring-accent/20"
        value={body}
        onChange={(event) => onBodyChange(event.target.value)}
        placeholder="Plain-text body"
      />
      <p className="text-[11px] leading-relaxed text-ink-faint">
        Plain text only. Short paragraphs with blank lines work best.
      </p>
    </div>
  );
}
