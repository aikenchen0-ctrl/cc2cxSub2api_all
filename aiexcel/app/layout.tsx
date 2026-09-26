import type { Metadata } from "next";
import Script from "next/script";
import "./globals.css";

export const metadata: Metadata = {
  title: "AI表格",
  description: "用自然语言完成公式、跨表关联、格式优化与数据分析。Excel 文件在浏览器本地解析，AI 请求通过 Sub2API 转发。",
  icons: { icon: "/project-icon.jpg", shortcut: "/project-icon.jpg" },
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="zh-CN"><body>{children}<Script id="sub2api-title" strategy="afterInteractive">document.title='AI表格';</Script><Script id="sub2api-branding" strategy="afterInteractive" dangerouslySetInnerHTML={{__html: `(function(){var b=/^(localhost|127\\.0\\.0\\.1)$/i.test(location.hostname)?'http://localhost:18080':'https://api.cc2.cx';function apply(value){var u=b+'/logo.jpg';if(typeof value==='string'&&value.trim()){var v=value.trim();if(v.indexOf('data:image/')===0)u=v;else if(v.indexOf('//')!==0){try{var parsed=new URL(v,b+'/');if(parsed.protocol==='http:'||parsed.protocol==='https:')u=parsed.href}catch(_){}}}var icon=document.querySelector('link[rel~="icon"]');if(!icon){icon=document.createElement('link');icon.rel='icon';document.head.appendChild(icon)}icon.href=u;document.querySelectorAll('[data-sub2api-site-logo]').forEach(function(image){if(image instanceof HTMLImageElement)image.src=u});window.__SUB2API_SITE_LOGO__=u;window.dispatchEvent(new Event('sub2api-logo-updated'))}function load(){fetch(b+'/api/v1/settings/public',{credentials:'omit',cache:'no-store'}).then(function(r){return r.ok?r.json():null}).then(function(x){apply(x&&x.data&&x.data.site_logo)}).catch(function(){apply('')})}apply('');new MutationObserver(function(){var logo=window.__SUB2API_SITE_LOGO__;document.querySelectorAll('[data-sub2api-site-logo]').forEach(function(image){if(image instanceof HTMLImageElement&&image.src!==logo)image.src=logo})}).observe(document.documentElement,{subtree:true,childList:true,attributes:true,attributeFilter:['src']});load();window.addEventListener('focus',load);document.addEventListener('visibilitychange',function(){if(!document.hidden)load()})})();`}} /></body></html>;
}
