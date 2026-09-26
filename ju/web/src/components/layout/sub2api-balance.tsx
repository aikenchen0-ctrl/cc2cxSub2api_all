import { useEffect, useState } from "react";

import { ApiError, http } from "@/services/api/request";
import { useUserStore } from "@/stores/use-user-store";

type BalanceResponse = { balance: number; recharge_url?: string };

const money = new Intl.NumberFormat("zh-CN", { style: "currency", currency: "USD", minimumFractionDigits: 2, maximumFractionDigits: 2 });

export function Sub2APIBalance() {
    const user = useUserStore((state) => state.user);
    const [balance, setBalance] = useState<number | null>(null);
    const [rechargeUrl, setRechargeUrl] = useState("");
    const [visible, setVisible] = useState(false);
    const [dialogOpen, setDialogOpen] = useState(false);

    useEffect(() => {
        if (!user) {
            setVisible(false);
            setBalance(null);
            setDialogOpen(false);
            return;
        }
        let active = true;
        let wasInsufficient: boolean | null = null;
        const refresh = async () => {
            if (!active || document.hidden) return;
            try {
                const result = await http.get<BalanceResponse>("/sub2api/balance");
                if (!active) return;
                const nextBalance = Number(result.balance);
                if (!Number.isFinite(nextBalance)) return;
                const insufficient = nextBalance <= 0;
                setVisible(true);
                setBalance(nextBalance);
                let nextUrl = "";
                const rawRechargeUrl = typeof result.recharge_url === "string" ? result.recharge_url.trim() : "";
                if (rawRechargeUrl) {
                    try {
                        const candidate = new URL(rawRechargeUrl);
                        if ((candidate.protocol === "http:" || candidate.protocol === "https:") && !candidate.username && !candidate.password) nextUrl = candidate.href;
                    } catch {}
                }
                setRechargeUrl(nextUrl);
                if (insufficient && wasInsufficient !== true) setDialogOpen(true);
                if (!insufficient) setDialogOpen(false);
                wasInsufficient = insufficient;
            } catch (error) {
                if (active && error instanceof ApiError && error.status === 401) {
                    wasInsufficient = null;
                    setVisible(false);
                    setBalance(null);
                    setDialogOpen(false);
                }
            }
        };
        void refresh();
        const interval = window.setInterval(refresh, 60_000);
        const onResume = () => { if (!document.hidden) void refresh(); };
        window.addEventListener("focus", onResume);
        document.addEventListener("visibilitychange", onResume);
        return () => {
            active = false;
            window.clearInterval(interval);
            window.removeEventListener("focus", onResume);
            document.removeEventListener("visibilitychange", onResume);
        };
    }, [user]);

    if (!visible || !user) return null;
    const insufficient = balance !== null && balance <= 0;
    const openRecharge = () => {
        if (rechargeUrl) window.open(rechargeUrl, "_blank", "noopener,noreferrer");
        else if (insufficient) setDialogOpen(true);
    };

    return (
        <>
            <button type="button" onClick={openRecharge} className="inline-flex h-8 items-center gap-1.5 rounded-full border border-border/70 px-2.5 text-xs font-medium text-foreground/70 transition hover:text-foreground" aria-label="Sub2API 账户余额，点击前往充值" title="Sub2API 账户余额">
                <span>余额</span><span className={insufficient ? "font-semibold tabular-nums text-red-600 dark:text-red-400" : "font-semibold tabular-nums"}>{balance === null ? "--" : money.format(balance)}</span>
            </button>
            {dialogOpen ? (
                <div className="fixed inset-0 z-[10000] grid place-items-center bg-black/55 p-5 backdrop-blur-sm" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setDialogOpen(false); }}>
                    <section className="w-full max-w-md rounded-2xl border border-border bg-background p-6 text-foreground shadow-2xl" role="dialog" aria-modal="true" aria-labelledby="sub2api-balance-title">
                        <h2 id="sub2api-balance-title" className="text-lg font-semibold">Sub2API 余额不足</h2>
                        <p className="mt-2 text-sm leading-6 text-foreground/65">当前账户余额为 <strong className="text-red-600 dark:text-red-400">{balance === null ? "--" : money.format(balance)}</strong>，暂时无法继续使用 AI 服务。请前往充值后重试。</p>
                        <div className="mt-6 flex justify-end gap-2">
                            <button type="button" className="rounded-lg border border-border px-4 py-2 text-sm" onClick={() => setDialogOpen(false)}>稍后</button>
                            <button type="button" className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50" disabled={!rechargeUrl} onClick={openRecharge}>前往充值</button>
                        </div>
                    </section>
                </div>
            ) : null}
        </>
    );
}
