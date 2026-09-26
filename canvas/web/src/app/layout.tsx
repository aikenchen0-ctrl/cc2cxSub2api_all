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
                    dangerouslySetInnerHTML={{ __html: `(function(){
                        var b = /^(localhost|127\\.0\\.0\\.1)$/i.test(location.hostname) ? 'http://localhost:18080' : 'https://api.cc2.cx';
                        var logo = b + '/logo.jpg';
                        var selector = '[data-sub2api-site-logo]';
                        var iconSelector = 'link[rel~="icon"]';
                        function resolve(value) {
                            if (typeof value !== 'string' || !value.trim()) return b + '/logo.jpg';
                            var v = value.trim();
                            if (v.indexOf('data:image/') === 0) return v;
                            try {
                                var u = new URL(v, b + '/');
                                return u.protocol === 'http:' || u.protocol === 'https:' ? u.href : b + '/logo.jpg';
                            } catch (_) {
                                return b + '/logo.jpg';
                            }
                        }
                        function patch(root) {
                            if (!root || root.nodeType !== 1) return;
                            if (root.matches(iconSelector) && root.href !== logo) root.href = logo;
                            if (root.matches(selector) && root instanceof HTMLImageElement && root.src !== logo) root.src = logo;
                            root.querySelectorAll(iconSelector).forEach(function (icon) { if (icon.href !== logo) icon.href = logo; });
                            root.querySelectorAll(selector).forEach(function (image) {
                                if (image instanceof HTMLImageElement && image.src !== logo) image.src = logo;
                            });
                        }
                        function apply(value) {
                            logo = resolve(value);
                            var icons = document.querySelectorAll(iconSelector);
                            if (!icons.length) {
                                var icon = document.createElement('link');
                                icon.rel = 'icon';
                                document.head.appendChild(icon);
                                icons = document.querySelectorAll(iconSelector);
                            }
                            icons.forEach(function (icon) { if (icon.href !== logo) icon.href = logo; });
                            patch(document.documentElement);
                            window.__SUB2API_SITE_LOGO__ = logo;
                            window.dispatchEvent(new Event('sub2api-logo-updated'));
                        }
                        function load() {
                            fetch(b + '/api/v1/settings/public', {
                                credentials: 'omit',
                                cache: 'no-store',
                                headers: { Accept: 'application/json' }
                            })
                                .then(function (response) { return response.ok ? response.json() : null; })
                                .then(function (settings) { apply(settings && settings.data && settings.data.site_logo); })
                                .catch(function () { apply(''); });
                        }
                        apply('');
                        new MutationObserver(function (records) {
                            records.forEach(function (record) {
                                if (record.type === 'attributes') patch(record.target);
                                record.addedNodes.forEach(patch);
                            });
                        }).observe(document.documentElement, {
                            subtree: true,
                            childList: true,
                            attributes: true,
                            attributeFilter: ['src', 'href']
                        });
                        load();
                        window.addEventListener('focus', load);
                        document.addEventListener('visibilitychange', function () { if (!document.hidden) load(); });
                    })();` }}
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
