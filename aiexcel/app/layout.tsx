import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "AI表格",
  description: "用自然语言完成公式、跨表关联、格式优化与数据分析。Excel 文件在浏览器本地解析，AI 请求通过 Sub2API 转发。",
  icons: { icon: "/project-icon.jpg", shortcut: "/project-icon.jpg" },
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="zh-CN"><body>{children}</body></html>;
}
