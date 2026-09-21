import { useEffect, useState } from "react";
import { api, type AppSettings } from "../lib/api";
import { Button, Card, CheckboxRow, Empty, Field, inputClass } from "./ui";

function joinList(values: string[]): string {
  return values.join(", ");
}

function splitList(raw: string): string[] {
  return raw
    .split(",")
    .map((part) => part.trim())
    .filter(Boolean);
}

type FormState = {
  blockedCountries: string;
  blockFreemail: boolean;
  brandLogoUrl: string;
  brandProductUrl: string;
  brandPrimaryColor: string;
  dailySendCap: string;
  minMinutesBetweenSends: string;
  sendWindowStartHour: string;
  sendWindowEndHour: string;
  sendWindowTimezone: string;
  sendOnWeekends: boolean;
};

function fromSettings(settings: AppSettings): FormState {
  return {
    blockedCountries: joinList(settings.blockedCountries),
    blockFreemail: settings.blockFreemail,
    brandLogoUrl: settings.brandLogoUrl,
    brandProductUrl: settings.brandProductUrl,
    brandPrimaryColor: settings.brandPrimaryColor,
    dailySendCap: String(settings.dailySendCap),
    minMinutesBetweenSends: String(settings.minMinutesBetweenSends),
    sendWindowStartHour: String(settings.sendWindowStartHour),
    sendWindowEndHour: String(settings.sendWindowEndHour),
    sendWindowTimezone: settings.sendWindowTimezone,
    sendOnWeekends: settings.sendOnWeekends,
  };
}

export function SettingsPanel({
  onRefresh,
  onError,
  onNotice,
}: {
  onRefresh: () => void;
  onError: (message: string) => void;
  onNotice?: (message: string) => void;
}) {
  const [form, setForm] = useState<FormState | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    void api
      .settings()
      .then((payload) => {
        if (!cancelled) setForm(fromSettings(payload.settings));
      })
      .catch((error) => {
        if (!cancelled) {
          onError(error instanceof Error ? error.message : "Failed to load settings");
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [onError]);

  function patchForm<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((current) => (current ? { ...current, [key]: value } : current));
  }

  async function saveSettings() {
    if (!form) return;
    setBusy(true);
    try {
      const result = await api.updateSettings({
        blockedCountries: splitList(form.blockedCountries).map((code) => code.toUpperCase()),
        blockFreemail: form.blockFreemail,
        brandLogoUrl: form.brandLogoUrl.trim(),
        brandProductUrl: form.brandProductUrl.trim(),
        brandPrimaryColor: form.brandPrimaryColor.trim(),
        dailySendCap: Number(form.dailySendCap),
        minMinutesBetweenSends: Number(form.minMinutesBetweenSends),
        sendWindowStartHour: Number(form.sendWindowStartHour),
        sendWindowEndHour: Number(form.sendWindowEndHour),
        sendWindowTimezone: form.sendWindowTimezone.trim(),
        sendOnWeekends: form.sendOnWeekends,
      });
      setForm(fromSettings(result.settings));
      onRefresh();
      onNotice?.("Platform settings saved.");
    } catch (error) {
      onError(error instanceof Error ? error.message : "Something went wrong");
    } finally {
      setBusy(false);
    }
  }

  if (loading || !form) {
    return <Empty>Loading settings…</Empty>;
  }

  return (
    <div className="space-y-5">
      <Card title="Brand">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Logo URL" hint="Empty → text wordmark “Observer”.">
            <input
              className={inputClass}
              value={form.brandLogoUrl}
              onChange={(event) => patchForm("brandLogoUrl", event.target.value)}
              placeholder="https://…"
            />
          </Field>
          <Field label="Product URL">
            <input
              className={inputClass}
              value={form.brandProductUrl}
              onChange={(event) => patchForm("brandProductUrl", event.target.value)}
              placeholder="https://getobserver.app"
            />
          </Field>
          <Field label="Primary color">
            <input
              className={inputClass}
              value={form.brandPrimaryColor}
              onChange={(event) => patchForm("brandPrimaryColor", event.target.value)}
              placeholder="#6ea8fe"
            />
          </Field>
        </div>
      </Card>

      <Card title="Send limits">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Daily send cap">
            <input
              className={inputClass}
              type="number"
              min={1}
              max={500}
              value={form.dailySendCap}
              onChange={(event) => patchForm("dailySendCap", event.target.value)}
            />
          </Field>
          <Field label="Minutes between sends">
            <input
              className={inputClass}
              type="number"
              min={0}
              max={120}
              value={form.minMinutesBetweenSends}
              onChange={(event) => patchForm("minMinutesBetweenSends", event.target.value)}
            />
          </Field>
          <Field label="Window start hour (0–23)">
            <input
              className={inputClass}
              type="number"
              min={0}
              max={23}
              value={form.sendWindowStartHour}
              onChange={(event) => patchForm("sendWindowStartHour", event.target.value)}
            />
          </Field>
          <Field label="Window end hour (1–24)">
            <input
              className={inputClass}
              type="number"
              min={1}
              max={24}
              value={form.sendWindowEndHour}
              onChange={(event) => patchForm("sendWindowEndHour", event.target.value)}
            />
          </Field>
          <Field label="Timezone (IANA)" hint="e.g. Asia/Kolkata, America/New_York">
            <input
              className={inputClass}
              value={form.sendWindowTimezone}
              onChange={(event) => patchForm("sendWindowTimezone", event.target.value)}
            />
          </Field>
          <div className="flex items-end">
            <CheckboxRow
              checked={form.sendOnWeekends}
              onChange={(checked) => patchForm("sendOnWeekends", checked)}
              disabled={busy}
              label="Send on weekends"
              hint="When off, Sat/Sun sends wait until Monday."
            />
          </div>
        </div>
      </Card>

      <Card title="Compliance">
        <div className="space-y-4">
          <Field
            label="Blocked countries"
            hint="ISO country codes. Prospects from these countries are skipped at draft/send."
          >
            <input
              className={inputClass}
              value={form.blockedCountries}
              onChange={(event) => patchForm("blockedCountries", event.target.value)}
              placeholder="CN, JP, AF, RU, IR, SO, …"
            />
          </Field>
          <CheckboxRow
            checked={form.blockFreemail}
            onChange={(checked) => patchForm("blockFreemail", checked)}
            disabled={busy}
            label="Block freemail addresses"
            hint="Skip Gmail/Yahoo/etc for prospected contacts. Manual/imported contacts are still allowed."
          />
        </div>
      </Card>

      <div className="flex justify-end">
        <Button variant="primary" disabled={busy} onClick={() => void saveSettings()}>
          {busy ? "Saving…" : "Save settings"}
        </Button>
      </div>
    </div>
  );
}
