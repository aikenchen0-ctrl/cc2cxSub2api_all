import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "格知 · AI Excel Copilot",
  description: "用自然语言完成公式、跨表关联、格式优化与数据分析。Excel 文件仅在本地浏览器处理。",
  icons: { icon: "/favicon.svg", shortcut: "/favicon.svg" },
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="zh-CN"><body>{children}</body></html>;
}
