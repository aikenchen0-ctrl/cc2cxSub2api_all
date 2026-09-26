"use client";

import { useEffect, useState } from "react";

import { apiGet } from "@/services/api/request";
import { useUserStore } from "@/stores/use-user-store";

type BalanceResponse = { balance: number; recharge_url?: string };

const money = new Intl.NumberFormat("zh-CN", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
});

export function Sub2APIBalance() {
    const token = useUserStore((state) => state.token);
    const user = useUserStore((state) => state.user);
    const [balance, setBalance] = useState<number | null>(null);
    const [rechargeUrl, setRechargeUrl] = useState("");
    const [dialogOpen, setDialogOpen] = useState(false);
    const [visible, setVisible] = useState(false);

    useEffect(() => {
        if (!token || !user) {
            setVisible(false);
            setBalance(null);
            setDialogOpen(false);
            return;
        }
        let previousInsufficient: boolean | null = null;
        let active = true;
        const load = async () => {
            if (!active || document.hidden) return;
            try {
                const result = await apiGet<BalanceResponse>("/api/sub2api/balance", undefined, token);
                if (!active) return;
                const nextBalance = Number(result.balance);
                if (!Number.isFinite(nextBalance)) return;
                const insufficient = nextBalance <= 0;
                setVisible(true);
                setBalance(nextBalance);
                let nextRechargeUrl = "";
                const rawRechargeUrl = typeof result.recharge_url === "string" ? result.recharge_url.trim() : "";
                if (rawRechargeUrl) {
                    try {
                        const candidate = new URL(rawRechargeUrl);
                        if ((candidate.protocol === "https:" || candidate.protocol === "http:") && !candidate.username && !candidate.password) nextRechargeUrl = candidate.href;
                    } catch {}
                }
                setRechargeUrl(nextRechargeUrl);
                if (insufficient && previousInsufficient !== true) setDialogOpen(true);
                if (!insufficient) setDialogOpen(false);
                previousInsufficient = insufficient;
            } catch (error) {
                if (active && error instanceof Error && /401|未登录|未关联 Sub2API/i.test(error.message)) {
                    previousInsufficient = null;
                    setVisible(false);
                    setBalance(null);
                    setDialogOpen(false);
                }
            }
        };
        void load();
        const interval = window.setInterval(load, 60_000);
        const onVisibility = () => { if (!document.hidden) void load(); };
        window.addEventListener("focus", onVisibility);
        document.addEventListener("visibilitychange", onVisibility);
        return () => {
            active = false;
            window.clearInterval(interval);
            window.removeEventListener("focus", onVisibility);
            document.removeEventListener("visibilitychange", onVisibility);
        };
    }, [token, user]);

    if (!visible || !user) return null;
    const insufficient = balance !== null && balance <= 0;
    const openRecharge = () => {
        if (rechargeUrl) window.open(rechargeUrl, "_blank", "noopener,noreferrer");
        else if (insufficient) setDialogOpen(true);
    };

    return (
        <>
            <button
                type="button"
                onClick={openRecharge}
                className="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-full border border-stone-300/70 px-2.5 text-xs font-medium text-stone-600 transition hover:border-stone-500 hover:text-stone-950 dark:border-stone-700 dark:text-stone-300 dark:hover:border-stone-500 dark:hover:text-white"
                aria-label="查看 Sub2API 账户余额并前往充值"
                title="Sub2API 账户余额"
            >
                <span>余额</span>
                <span className={insufficient ? "font-semibold tabular-nums text-red-600 dark:text-red-400" : "font-semibold tabular-nums"}>
                    {balance === null ? "--" : money.format(balance)}
                </span>
            </button>
            {dialogOpen ? (
                <div className="fixed inset-0 z-[10000] grid place-items-center bg-black/55 p-5 backdrop-blur-sm" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setDialogOpen(false); }}>
                    <section className="w-full max-w-md rounded-2xl border border-stone-200 bg-white p-6 text-stone-950 shadow-2xl dark:border-stone-700 dark:bg-stone-900 dark:text-stone-100" role="dialog" aria-modal="true" aria-labelledby="sub2api-balance-title">
                        <h2 id="sub2api-balance-title" className="text-lg font-semibold">Sub2API 余额不足</h2>
                        <p className="mt-2 text-sm leading-6 text-stone-600 dark:text-stone-300">
                            当前账户余额为 <strong className="text-red-600 dark:text-red-400">{balance === null ? "--" : money.format(balance)}</strong>，暂时无法继续使用 AI 服务。请前往充值后重试。
                        </p>
                        <div className="mt-6 flex justify-end gap-2">
                            <button type="button" className="rounded-lg border border-stone-300 px-4 py-2 text-sm dark:border-stone-600" onClick={() => setDialogOpen(false)}>稍后</button>
                            <button type="button" className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white disabled:opacity-50" disabled={!rechargeUrl} onClick={openRecharge}>前往充值</button>
                        </div>
                    </section>
                </div>
            ) : null}
        </>
    );
}
