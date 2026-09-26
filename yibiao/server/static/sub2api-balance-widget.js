(() => {
  function mount() {
    const host = document.querySelector("[data-sub2api-balance-widget]") || document.createElement("div");
    if (host.shadowRoot) return;
    if (!host.hasAttribute("data-sub2api-balance-widget")) {
      host.setAttribute("data-sub2api-balance-widget", "");
      document.body.appendChild(host);
    }
    const root = host.attachShadow({ mode: "open" });
    const money = new Intl.NumberFormat("zh-CN", { style: "currency", currency: "USD", minimumFractionDigits: 2, maximumFractionDigits: 2 });
    root.innerHTML = `<style>
      :host{display:inline-flex;align-items:center;font:12px/1.2 system-ui,-apple-system,"Segoe UI",sans-serif;color:#334155}
      .pill{display:inline-flex;align-items:center;gap:6px;padding:6px 10px;border:1px solid #cbd5e1;border-radius:999px;background:#ffffffed;color:#334155;font:inherit;white-space:nowrap;cursor:pointer;box-shadow:0 4px 18px #0002}
      .amount{font-variant-numeric:tabular-nums;font-weight:700}.low{color:#dc2626}.overlay{position:fixed;inset:0;z-index:2147483000;display:grid;place-items:center;padding:20px;background:#0b122099;backdrop-filter:blur(3px)}.overlay[hidden]{display:none}
      .dialog{width:min(420px,100%);padding:22px;border:1px solid #cbd5e1;border-radius:16px;background:#fff;color:#111827;box-shadow:0 24px 80px #0004}.dialog h2{margin:0 0 8px;font-size:18px}.dialog p{margin:0 0 18px;color:#4b5563;line-height:1.6}.actions{display:flex;justify-content:flex-end;gap:8px}
      .dialog button,.dialog a{display:inline-flex;align-items:center;justify-content:center;min-height:36px;padding:0 13px;border:1px solid #d1d5db;border-radius:9px;background:#fff;color:#374151;font:inherit;text-decoration:none;cursor:pointer}.dialog a{border-color:#2563eb;background:#2563eb;color:#fff}.dialog a[aria-disabled=true]{opacity:.5;pointer-events:none}
      @media(prefers-color-scheme:dark){:host{color:#e2e8f0}.pill{background:#0f1725ed;border-color:#475569}.dialog{background:#17191d;color:#f3f4f6}.dialog p{color:#cbd5e1}.dialog button{background:#22252b;color:#f3f4f6;border-color:#454a54}}
    </style><button class="pill" type="button" aria-label="查看 Sub2API 账户余额"><span>余额</span><strong class="amount">--</strong></button>
    <div class="overlay" hidden><section class="dialog" role="dialog" aria-modal="true" aria-labelledby="balance-title"><h2 id="balance-title">Sub2API 余额不足</h2><p>当前账户余额为 <strong class="amount low"></strong>，暂时无法继续使用 AI 服务。请前往充值后重试。</p><div class="actions"><button type="button" data-close>稍后</button><a data-recharge target="_blank" rel="noopener noreferrer">前往充值</a></div></section></div>`;
    const pill = root.querySelector(".pill"), amount = root.querySelector(".pill .amount"), dialogAmount = root.querySelector(".dialog .amount"), overlay = root.querySelector(".overlay"), recharge = root.querySelector("[data-recharge]");
    pill.style.display = "none";
    let rechargeUrl = "", wasInsufficient = null;
    const close = () => { overlay.hidden = true; };
    root.querySelector("[data-close]").addEventListener("click", close);
    overlay.addEventListener("click", (event) => { if (event.target === overlay) close(); });
    root.addEventListener("keydown", (event) => { if (event.key === "Escape") close(); });
    pill.addEventListener("click", () => { if (rechargeUrl) window.open(rechargeUrl, "_blank", "noopener,noreferrer"); else if (wasInsufficient === true) overlay.hidden = false; });
    recharge.addEventListener("click", close);
    async function refresh() {
      if (document.hidden) return;
      try {
        const response = await fetch("/api/sub2api/balance", { credentials: "same-origin", cache: "no-store", headers: { Accept: "application/json" } });
        if (!response.ok) { if (response.status === 401) { pill.style.display = "none"; close(); rechargeUrl = ""; wasInsufficient = null; } return; }
        const result = await response.json(), balance = Number(result.balance);
        if (!Number.isFinite(balance)) return;
        pill.style.display = "";
        const insufficient = balance <= 0, text = money.format(balance);
        amount.textContent = text; dialogAmount.textContent = text; amount.classList.toggle("low", insufficient);
        rechargeUrl = "";
        const rawRechargeUrl = typeof result.recharge_url === "string" ? result.recharge_url.trim() : "";
        if (rawRechargeUrl) try { const url = new URL(rawRechargeUrl); if (/^https?:$/.test(url.protocol) && !url.username && !url.password) rechargeUrl = url.href; } catch {}
        recharge.href = rechargeUrl || "#"; recharge.setAttribute("aria-disabled", String(!rechargeUrl));
        if (insufficient && wasInsufficient !== true) overlay.hidden = false;
        if (!insufficient) close();
        wasInsufficient = insufficient;
      } catch {}
    }
    void refresh(); window.setInterval(refresh, 60_000); window.addEventListener("focus", refresh); document.addEventListener("visibilitychange", refresh);
  }
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", mount, { once: true }); else mount();
})();
