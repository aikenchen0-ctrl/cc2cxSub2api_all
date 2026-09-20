import { LOCALES, LOCALE_META } from "./locales";
import { useI18n } from "./I18nProvider";

export function LanguageSwitcher() {
  const { locale, setLocale, t } = useI18n();

  return (
    <label className="flex items-center gap-1.5 text-sm text-muted-foreground">
      <span className="sr-only">{t("lang.label")}</span>
      <select
        aria-label={t("lang.label")}
        value={locale}
        onChange={(e) => setLocale(e.target.value as typeof locale)}
        className="h-8 rounded-md border bg-background px-2 text-xs text-foreground"
      >
        {LOCALES.map((code) => (
          <option key={code} value={code}>
            {LOCALE_META[code].label}
          </option>
        ))}
      </select>
    </label>
  );
}
