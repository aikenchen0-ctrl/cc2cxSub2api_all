"use client";
import { createContext, useContext, useEffect, useState, type ReactNode } from "react";

const UiLanguage = createContext({ language: "Chinese", setLanguage: (_value: string) => {} });
export function UiLanguageProvider({ children }: { children: ReactNode }) {
  const [language, setLanguage] = useState("Chinese");
  useEffect(() => { document.documentElement.lang = language === "English" ? "en" : "zh-CN"; }, [language]);
  return <UiLanguage.Provider value={{ language, setLanguage }}>{children}</UiLanguage.Provider>;
}
export function useUiLanguage() {
  const context = useContext(UiLanguage);
  return { ...context, t: (zh: string, en: string) => context.language === "English" ? en : zh };
}
export function UiText({ zh, en }: { zh: string; en: string }) {
  return <>{useUiLanguage().t(zh, en)}</>;
}
