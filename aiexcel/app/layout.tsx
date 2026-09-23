import type { Metadata } from "next";
import Script from "next/script";
import "./globals.css";

export const metadata: Metadata = {
  title: "AI表格",
  description: "用自然语言完成公式、跨表关联、格式优化与数据分析。Excel 文件在浏览器本地解析，AI 请求通过 Sub2API 转发。",
  icons: { icon: "/project-icon.jpg", shortcut: "/project-icon.jpg" },
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="zh-CN"><body>{children}<Script id="sub2api-title" strategy="afterInteractive">document.title='AI表格';</Script><Script id="sub2api-branding" strategy="afterInteractive" dangerouslySetInnerHTML={{__html: `(function(){var b=/^(localhost|127\\.0\\.0\\.1)$/.test(location.hostname)?'http://localhost:18080':'https://api.cc2.cx';fetch(b+'/api/v1/settings/public',{credentials:'omit'}).then(function(r){return r.ok?r.json():null}).then(function(x){var l=x&&x.data&&x.data.site_logo;if(!l)return;var e=document.querySelector('link[rel~="icon"]')||document.createElement('link');e.rel='icon';e.href=new URL(l,b+'/').href;if(!e.parentNode)document.head.appendChild(e)}).catch(function(){})})();`}} /></body></html>;
}
