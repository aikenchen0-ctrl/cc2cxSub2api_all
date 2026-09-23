import type { Metadata } from "next";
import Script from "next/script";
import { AntdRegistry } from "@ant-design/nextjs-registry";
import { AppProviders } from "@/components/layout/app-providers";
import "antd/dist/reset.css";
import "./globals.css";
import React from "react";

export const metadata: Metadata = {
    title: "AI生图生视频",
    description: "AI 图片与视频创作工作台",
    icons: { icon: "/project-icon.jpg", shortcut: "/project-icon.jpg" },
};

export default function RootLayout({
    children,
}: Readonly<{
    children: React.ReactNode;
}>) {
    return (
        <html lang="zh-CN" suppressHydrationWarning className="font-sans">
            <body
                className="bg-background text-foreground antialiased"
                style={{
                    fontFamily: '"SF Pro Display","SF Pro Text","PingFang SC","Microsoft YaHei","Helvetica Neue",sans-serif',
                }}
            >
                <Script id="sub2api-title" strategy="afterInteractive">document.title='AI生图生视频';</Script>
                <Script
                    id="sub2api-branding"
                    strategy="afterInteractive"
                    dangerouslySetInnerHTML={{ __html: `(function(){var b=/^(localhost|127\\.0\\.1)$/.test(location.hostname)?'http://localhost:18080':'https://api.cc2.cx';fetch(b+'/api/v1/settings/public',{credentials:'omit'}).then(function(r){return r.ok?r.json():null}).then(function(x){var l=x&&x.data&&x.data.site_logo;if(!l)return;var e=document.querySelector('link[rel~="icon"]')||document.createElement('link');e.rel='icon';e.href=new URL(l,b+'/').href;if(!e.parentNode)document.head.appendChild(e)}).catch(function(){})})();` }}
                />
                <Script
                    id="theme-script"
                    strategy="beforeInteractive"
                    dangerouslySetInnerHTML={{
                        __html: `try{var s=JSON.parse(localStorage.getItem("infinite-canvas:theme_store")||"{}");var t=s.state&&s.state.theme==="light"?"light":"dark";document.documentElement.classList.toggle("dark",t==="dark");document.documentElement.style.colorScheme=t}catch(e){}`,
                    }}
                />
                <AntdRegistry>
                    <AppProviders>{children}</AppProviders>
                </AntdRegistry>
            </body>
        </html>
    );
}
