import type { Metadata } from "next";
import localFont from "next/font/local";
import { Manrope, Syne, Unbounded } from "next/font/google";
import "./globals.css";
import "katex/dist/katex.min.css";
import { Providers } from "./providers";
import MixpanelInitializer from "./MixpanelInitializer";
import { Toaster } from "@/components/ui/sonner";
import Sub2ApiBalanceWidget from "@/components/Sub2ApiBalanceWidget";
import TailwindBrowserRuntime from "@/components/runtime/TailwindBrowserRuntime";
import Script from "next/script";
const inter = localFont({
  src: [
    {
      path: "./fonts/Inter.ttf",
      weight: "400",
      style: "normal",
    },
  ],
  preload: false,
  variable: "--font-inter",
});

const syne = Syne({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700", "800"],
  variable: "--font-syne",
});

const manrope = Manrope({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700", "800"],
  preload: false,
  variable: "--font-manrope",
});

const unbounded = Unbounded({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700", "800"],
  preload: false,
  variable: "--font-unbounded",
});

export const metadata: Metadata = {
  metadataBase: new URL("https://presenton.ai"),
  title: "PPT生成",
  description: "基于人工智能的演示文稿生成器，支持自定义布局、多模型和 PDF/PPTX 导出。",
  keywords: [
    "AI 演示文稿生成",
    "数据故事",
    "演示文稿生成器",
  ],
  openGraph: {
    title: "PPT生成",
    description: "基于人工智能的演示文稿生成器，支持自定义布局、多模型和 PDF/PPTX 导出。",
    url: "https://presenton.ai",
    siteName: "PPT生成",
    images: [
      {
        url: "https://presenton.ai/presenton-feature-graphics.png",
        width: 1200,
        height: 630,
        alt: "PPT生成",
      },
    ],
    type: "website",
    locale: "zh_CN",
  },
  alternates: {
    canonical: "https://presenton.ai",
  },
  twitter: {
    card: "summary_large_image",
    title: "PPT生成",
    description: "基于人工智能的演示文稿生成器，支持自定义布局、多模型和 PDF/PPTX 导出。",
    images: ["https://presenton.ai/presenton-feature-graphics.png"],
  },
  icons: { icon: "/project-icon.jpg", shortcut: "/project-icon.jpg" },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {

  return (
    <html lang="zh-CN">
      <body
        className={`${inter.variable} ${syne.variable} ${manrope.variable} ${unbounded.variable} antialiased`}
      >
        <script dangerouslySetInnerHTML={{__html: "document.title='PPT生成'"}} />
        <Script id="sub2api-branding" strategy="afterInteractive" dangerouslySetInnerHTML={{__html: `(function(){var b=/^(localhost|127\\.0\\.0\\.1)$/i.test(location.hostname)?'http://localhost:18080':'https://api.cc2.cx';function apply(value){var u=b+'/logo.jpg';if(typeof value==='string'&&value.trim()){var v=value.trim();if(v.indexOf('data:image/')===0)u=v;else if(v.indexOf('//')!==0){try{var parsed=new URL(v,b+'/');if(parsed.protocol==='http:'||parsed.protocol==='https:')u=parsed.href}catch(_){}}}var icon=document.querySelector('link[rel~="icon"]');if(!icon){icon=document.createElement('link');icon.rel='icon';document.head.appendChild(icon)}icon.href=u;document.querySelectorAll('[data-sub2api-site-logo]').forEach(function(image){if(image instanceof HTMLImageElement)image.src=u});window.__SUB2API_SITE_LOGO__=u;window.dispatchEvent(new Event('sub2api-logo-updated'))}function load(){fetch(b+'/api/v1/settings/public',{credentials:'omit',cache:'no-store'}).then(function(r){return r.ok?r.json():null}).then(function(x){apply(x&&x.data&&x.data.site_logo)}).catch(function(){apply('')})}apply('');new MutationObserver(function(){var logo=window.__SUB2API_SITE_LOGO__;document.querySelectorAll('[data-sub2api-site-logo]').forEach(function(image){if(image instanceof HTMLImageElement&&image.src!==logo)image.src=logo})}).observe(document.documentElement,{subtree:true,childList:true,attributes:true,attributeFilter:['src']});load();window.addEventListener('focus',load);document.addEventListener('visibilitychange',function(){if(!document.hidden)load()})})();`}} />
        <Providers>
          <MixpanelInitializer>

            {children}

          </MixpanelInitializer>
        </Providers>
        <TailwindBrowserRuntime />
        <Toaster position="top-center" />
        <Sub2ApiBalanceWidget />
      </body>
    </html>
  );
}
