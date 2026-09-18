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
    editor: "Editor", versions: "Versions", newProject: "New", startNewProject: "Start a new project", clearSelection: "Clear selection", backToLatest: "Back to latest", retry: "Retry", cancelAll: "Cancel all generations", showLess: "Show less", showMore: "Show more", retryRequest: "Click Retry to run this version's request again.", switchOption: "Switch to another option above to make updates.",
    selectElement: "Select an element in the preview to target your edit", send: "Send", working: "Working…", dropImages: "Drop images here",
    uploadScreenshots: "Upload screenshots", uploadHint: "Drop screenshots or a video here, or click to browse", uploadedScreenshots: "Uploaded Screenshots",
    dropToAdd: "Drop to add", removeVideo: "Remove video", removeScreenshot: "Remove screenshot", addMoreScreenshots: "Add more screenshots", or: "or",
    previewScreenshot: "Preview screenshot", urlCapture: "Capture from URL", urlCaptureHelp: "Enter a public URL to capture a screenshot and generate code from it.", enterUrl: "Please enter a URL",
    invalidUrl: "Please enter a valid URL", captureFailed: "Failed to capture screenshot. Check the console for details.",
    designSystems: "Design Systems", addDesignSystem: "+ Add design system", manageDesignSystems: "Manage design systems", noDesignSystems: "No design systems yet",
    designSystemName: "Design system name", designSystemContent: "Design system content", designSystemNamePlaceholder: "e.g. Marketing site", designSystemContentPlaceholder: "Fill in details about colors, fonts, components, and layout preferences…",
    designSystemNameRequired: "Design system name is required.", designCreated: "Design system created.", designCreateFailed: "Could not create design system.", designSaved: "Design system saved.", designSaveFailed: "Could not save design system.", designDeleted: "Design system deleted.", designDeleteFailed: "Could not delete design system.",
    desktop: "Desktop", mobile: "Mobile", code: "Code", openNewTab: "Open in New Tab", previousVersion: "Previous version", nextVersion: "Next version", allVersions: "View all versions", downloadCode: "Download Code", refreshPreview: "Refresh Preview", scaleToFit: "Scale down to fit the screen", originalSize: "View at original size (100%)",
    history: "History", noHistory: "No history yet", clearHistory: "Clear history", version: "Version", current: "Current", commonDelete: "Delete", removeDefault: "Remove as default", setDefault: "Set as default", saveChanges: "Save changes", saved: "Saved",
    screenshotFromUrl: "Screenshot from URL", urlCaptureHelpShort: "Enter a public webpage and we’ll capture it before generating code.", websiteUrl: "Website URL", requiresScreenshotKey: "Requires a ScreenshotOne API key in Settings.", directFigmaUnsupported: "Direct Figma import isn’t supported. Export your artboards as images and use the Upload tab instead.", captureGenerate: "Capture & Generate", capturing: "Capturing…", screenshotKeyRequired: "Please add a ScreenshotOne API key in Settings. You can also upload screenshots directly in the Upload tab.", fileUrlUnsupported: "file:// URLs can't be screenshot. If you're trying to import a local file, please use the Import tab.", figmaUnsupported: "Direct Figma import is not supported. Take a screenshot of your design or export the artboards as images, then use the Upload tab.",
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
    editor: "编辑器", versions: "版本", newProject: "新建", startNewProject: "开始新项目", clearSelection: "清除选择", backToLatest: "返回最新版本", retry: "重试", cancelAll: "取消所有生成", showLess: "收起", showMore: "展开", retryRequest: "点击重试再次运行此版本的请求。", switchOption: "切换上方其他选项以继续修改。",
    selectElement: "在预览中选择元素以编辑", send: "发送", working: "处理中…", dropImages: "将图片拖放到这里",
    uploadScreenshots: "上传截图", uploadHint: "将截图或视频拖放到这里，或点击浏览", uploadedScreenshots: "已上传截图", dropToAdd: "拖放以添加", removeVideo: "移除视频", removeScreenshot: "移除截图", addMoreScreenshots: "添加更多截图", or: "或",
    previewScreenshot: "预览截图", urlCapture: "从网址捕获", urlCaptureHelp: "输入公开网址，捕获截图并根据页面生成代码。", enterUrl: "请输入网址", invalidUrl: "请输入有效网址", captureFailed: "截图失败，请查看控制台了解详情。",
    designSystems: "设计系统", addDesignSystem: "+ 添加设计系统", manageDesignSystems: "管理设计系统", noDesignSystems: "还没有设计系统", designSystemName: "设计系统名称", designSystemContent: "设计系统内容", designSystemNamePlaceholder: "例如：营销网站", designSystemContentPlaceholder: "填写颜色、字体、组件和布局偏好…", designSystemNameRequired: "请输入设计系统名称。", designCreated: "设计系统已创建。", designCreateFailed: "无法创建设计系统。", designSaved: "设计系统已保存。", designSaveFailed: "无法保存设计系统。", designDeleted: "设计系统已删除。", designDeleteFailed: "无法删除设计系统。",
    desktop: "桌面端", mobile: "移动端", code: "代码", openNewTab: "在新标签页打开", previousVersion: "上一版本", nextVersion: "下一版本", allVersions: "查看所有版本", downloadCode: "下载代码", refreshPreview: "刷新预览", scaleToFit: "缩小以适应屏幕", originalSize: "按原始尺寸查看（100%）", history: "历史记录", noHistory: "暂无历史记录", clearHistory: "清空历史记录", version: "版本", current: "当前", commonDelete: "删除", removeDefault: "取消默认", setDefault: "设为默认", saveChanges: "保存修改", saved: "已保存",
    screenshotFromUrl: "从网址捕获截图", urlCaptureHelpShort: "输入公开网页，捕获截图后生成代码。", websiteUrl: "网站网址", requiresScreenshotKey: "需要在设置中配置 ScreenshotOne API 密钥。", directFigmaUnsupported: "不支持直接导入 Figma，请先导出画板图片并使用上传功能。", captureGenerate: "捕获并生成", capturing: "捕获中…", screenshotKeyRequired: "请先在设置中添加 ScreenshotOne API 密钥，也可以直接在上传页上传截图。", fileUrlUnsupported: "无法截取 file:// 地址；如果要导入本地文件，请使用导入页。", figmaUnsupported: "不支持直接导入 Figma，请截取设计图或导出画板图片后使用上传页。",
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
