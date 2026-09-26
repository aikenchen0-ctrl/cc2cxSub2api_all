import { useEffect, useState, type CSSProperties, type ReactNode } from "react";

import { cn } from "@/lib/utils";
import { appearanceLogoURL, useAppearanceStore } from "@/stores/use-appearance-store";
import { useThemeStore, type ThemeName } from "@/stores/use-theme-store";

type BrandLogoProps = {
    className?: string;
    fallback: ReactNode;
    alt?: string;
    theme?: ThemeName | "auto";
};

function getSub2ApiSiteLogo() {
    if (typeof window === "undefined") return "";
    return (window as Window & { __SUB2API_SITE_LOGO__?: string }).__SUB2API_SITE_LOGO__ || "";
}

export function BrandLogo({ className, fallback, alt = "", theme = "auto" }: BrandLogoProps) {
    const appearance = useAppearanceStore((state) => state.appearance);
    const currentTheme = useThemeStore((state) => state.theme);
    const [siteLogo, setSiteLogo] = useState(getSub2ApiSiteLogo);
    const source = siteLogo || (appearance.logoConfigured ? appearanceLogoURL(appearance, theme === "auto" ? currentTheme : theme) : "");
    const [failedSource, setFailedSource] = useState<string | null>(null);

    useEffect(() => {
        const syncSiteLogo = () => setSiteLogo(getSub2ApiSiteLogo());
        syncSiteLogo();
        window.addEventListener("sub2api-logo-updated", syncSiteLogo);
        return () => window.removeEventListener("sub2api-logo-updated", syncSiteLogo);
    }, []);

    if (!source) return <>{fallback}</>;
    // A configured custom logo must never fall through to the built-in brand
    // when its file becomes unavailable. Keep its footprint neutral instead.
    if (failedSource === source) return <span className={cn("block", className)} aria-hidden="true" />;
    return (
        <img
            src={source}
            data-sub2api-site-logo
            alt={alt}
            className={cn("block object-contain", className)}
            draggable={false}
            onError={(event) => {
                event.currentTarget.style.visibility = "hidden";
                setFailedSource(source);
            }}
        />
    );
}

export function BrandLogoFrame({ className, logoClassName, fallback, alt = "", theme = "auto" }: BrandLogoProps & { logoClassName?: string }) {
    const frameEnabled = useAppearanceStore((state) => state.appearance.logoFrameEnabled);
    const unframedStyle: CSSProperties | undefined = frameEnabled
        ? undefined
        : {
              background: "transparent",
              borderColor: "transparent",
              borderRadius: 0,
              boxShadow: "none",
              color: "inherit",
          };
    return (
        <span className={cn("brand-logo-frame", className)} data-logo-frame-enabled={frameEnabled} style={unframedStyle}>
            <BrandLogo className={logoClassName} fallback={fallback} alt={alt} theme={theme} />
        </span>
    );
}
