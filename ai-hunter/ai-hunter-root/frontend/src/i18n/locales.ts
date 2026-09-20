export const LOCALES = ["zh", "en", "ja", "ar", "fa"] as const;

export type Locale = (typeof LOCALES)[number];

export const DEFAULT_LOCALE: Locale = "zh";
export const STORAGE_KEY = "ai-huoke-locale";

export const LOCALE_META: Record<
  Locale,
  { label: string; htmlLang: string; dir: "ltr" | "rtl" }
> = {
  zh: { label: "中文", htmlLang: "zh-CN", dir: "ltr" },
  en: { label: "English", htmlLang: "en", dir: "ltr" },
  ja: { label: "日本語", htmlLang: "ja", dir: "ltr" },
  ar: { label: "العربية", htmlLang: "ar", dir: "rtl" },
  fa: { label: "فارسی", htmlLang: "fa", dir: "rtl" },
};

export function isLocale(value: string | null | undefined): value is Locale {
  return LOCALES.includes(value as Locale);
}
