import { createContext, useContext, useMemo, useState, type ReactNode } from "react";

export type Language = "en" | "zh-CN";

const messages = {
  en: {
    language: "Language", english: "English", chinese: "简体中文",
    settings: "Settings", theme: "Theme", appTheme: "App Theme", appThemeHelp: "System default, with optional light/dark override", system: "System", light: "Light", dark: "Dark",
    languageHelp: "Choose the language used by the interface", apiKeys: "API Keys", editorTheme: "Code Editor Theme",
    upload: "Upload", url: "URL", text: "Text", import: "Import", generateFromText: "Generate from Text",
    describeUi: "Describe the UI you want to create…", tryExample: "Try an example:", generate: "Generate",
    generateShortcut: "Press Cmd/Ctrl + Enter to generate", importCode: "Import Existing Code",
    pasteCode: "Paste your HTML code here or drag/drop a .html file…", importButton: "Import Code",
    importShortcut: "Press Cmd/Ctrl + Enter to import", descriptionRequired: "Please enter a description",
    codeRequired: "Please paste in some code", stackRequired: "Please select your stack", stack: "Stack:",
    selectStack: "Select a stack", instructions: "Additional instructions", addInstructions: "Add instructions",
    optional: "Optional", instructionsPlaceholder: "Describe anything you want changed or emphasized…",
    instructionsAria: "Instructions for generated code", extractAssets: "Extract image assets from original",
    designSystem: "Design system", selectDesignSystem: "Select a design system", noDesignSystem: "No design system",
  },
  "zh-CN": {
    language: "语言", english: "English", chinese: "简体中文", settings: "设置", theme: "主题", appTheme: "应用主题",
    system: "跟随系统", light: "浅色", dark: "深色", appThemeHelp: "跟随系统，也可以单独选择浅色或深色", languageHelp: "选择界面使用的语言", apiKeys: "API 密钥",
    editorTheme: "代码编辑器主题", upload: "上传", url: "网址", text: "文本", import: "导入",
    generateFromText: "根据文本生成", describeUi: "描述你想创建的界面…", tryExample: "试试示例：", generate: "生成",
    generateShortcut: "按 Cmd/Ctrl + Enter 生成", importCode: "导入现有代码", pasteCode: "粘贴 HTML 代码，或拖放 .html 文件…",
    importButton: "导入代码", importShortcut: "按 Cmd/Ctrl + Enter 导入", descriptionRequired: "请输入描述",
    codeRequired: "请先粘贴代码", stackRequired: "请选择技术栈", stack: "技术栈：", selectStack: "选择技术栈",
    instructions: "补充说明", addInstructions: "添加说明", optional: "可选", instructionsPlaceholder: "描述你希望修改或强调的内容…",
    instructionsAria: "生成代码的补充说明", extractAssets: "从原图提取图片资源", designSystem: "设计系统",
    selectDesignSystem: "选择设计系统", noDesignSystem: "不使用设计系统",
  },
} as const;

type Key = keyof typeof messages.en;
interface ContextValue { language: Language; setLanguage: (language: Language) => void; t: (key: Key) => string; }
const Context = createContext<ContextValue | null>(null);
const STORAGE_KEY = "screen2code-language";

function initialLanguage(): Language {
  if (typeof window === "undefined") return "en";
  const stored = window.localStorage.getItem(STORAGE_KEY);
  if (stored === "en" || stored === "zh-CN") return stored;
  return navigator.language.toLowerCase().startsWith("zh") ? "zh-CN" : "en";
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [language, setLanguageState] = useState<Language>(initialLanguage);
  const setLanguage = (next: Language) => { setLanguageState(next); window.localStorage.setItem(STORAGE_KEY, next); };
  const value = useMemo(() => ({ language, setLanguage, t: (key: Key) => messages[language][key] ?? messages.en[key] }), [language]);
  return <Context.Provider value={value}>{children}</Context.Provider>;
}

export function useI18n() {
  const value = useContext(Context);
  if (!value) throw new Error("useI18n must be used inside I18nProvider");
  return value;
}
