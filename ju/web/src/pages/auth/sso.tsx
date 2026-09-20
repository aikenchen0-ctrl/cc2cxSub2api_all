import { useEffect, useRef } from "react";
import { FullScreenLoader } from "@/components/ui/aceternity/full-screen-loader";
import { exchangeSub2APISSO } from "@/services/api/auth";

export default function SSOPage() {
    const started = useRef(false);
    useEffect(() => {
        if (started.current) return;
        started.current = true;
        void exchangeSub2APISSO().then(({ next }) => {
            // Reload so identity hydration and user-scoped caches start together.
            window.location.replace(next);
        }).catch(() => window.location.replace("/login?sso_error=1"));
    }, []);
    return <FullScreenLoader label="正在登录" detail="" />;
}
