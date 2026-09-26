(() => {
  const mounted = new WeakSet();
  const money = new Intl.NumberFormat("zh-CN", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });

  function mount(host) {
    if (!(host instanceof HTMLElement) || mounted.has(host)) return;
    mounted.add(host);
    const root = host.attachShadow({ mode: "open" });
    root.innerHTML = `<style>
      :host{display:inline-flex;align-items:center;font:12px/1.2 system-ui,-apple-system,"Segoe UI",sans-serif;color:inherit}
      .pill{display:inline-flex;align-items:center;gap:6px;padding:6px 10px;border:1px solid #9ca3af66;border-radius:999px;background:#ffffffd9;color:inherit;font:inherit;white-space:nowrap;cursor:pointer;box-shadow:0 4px 18px #00000012}
      .amount{font-variant-numeric:tabular-nums;font-weight:700}.amount.low{color:#dc2626}
      .overlay{position:fixed;inset:0;z-index:2147483000;display:grid;place-items:center;padding:20px;background:#0b122099;backdrop-filter:blur(3px)}
      .overlay[hidden]{display:none}.dialog{width:min(420px,100%);padding:22px;border:1px solid #9ca3af55;border-radius:16px;background:#fff;color:#111827;box-shadow:0 24px 80px #0004}
      .dialog h2{margin:0 0 8px;font-size:18px}.dialog p{margin:0 0 18px;color:#4b5563;line-height:1.6}.actions{display:flex;justify-content:flex-end;gap:8px}
      .dialog button,.dialog a{display:inline-flex;align-items:center;justify-content:center;min-height:36px;padding:0 13px;border:1px solid #d1d5db;border-radius:9px;background:#fff;color:#374151;font:inherit;text-decoration:none;cursor:pointer}
      .dialog a{border-color:#2563eb;background:#2563eb;color:#fff}.dialog a[aria-disabled=true]{opacity:.5;pointer-events:none}
      @media(prefers-color-scheme:dark){.pill{background:#17191ddd}.dialog{background:#17191d;color:#f3f4f6}.dialog p{color:#cbd5e1}.dialog button{background:#22252b;color:#f3f4f6;border-color:#454a54}}
    </style>
    <button class="pill" type="button" aria-label="查看账户余额"><span>余额</span><strong class="amount">--</strong></button>
    <div class="overlay" hidden><section class="dialog" role="dialog" aria-modal="true" aria-labelledby="balance-title">
      <h2 id="balance-title">余额不足</h2><p>当前账户余额为 <strong class="amount low"></strong>，暂时无法继续使用 AI 服务。请前往充值后重试。</p>
      <div class="actions"><button type="button" data-close>稍后</button><a data-recharge target="_blank" rel="noopener noreferrer">前往充值</a></div>
    </section></div>`;

    const pill = root.querySelector(".pill");
    const amount = root.querySelector(".pill .amount");
    const dialogAmount = root.querySelector(".dialog .amount");
    const overlay = root.querySelector(".overlay");
    const recharge = root.querySelector("[data-recharge]");
    pill.style.display = "none";
    let rechargeUrl = "";
    let lastInsufficient = null;
    const close = () => { overlay.hidden = true; };
    const show = () => { overlay.hidden = false; };

    root.querySelector("[data-close]").addEventListener("click", close);
    overlay.addEventListener("click", (event) => { if (event.target === overlay) close(); });
    root.addEventListener("keydown", (event) => { if (event.key === "Escape") close(); });
    pill.addEventListener("click", () => { if (rechargeUrl) window.open(rechargeUrl, "_blank", "noopener,noreferrer"); else if (lastInsufficient === true) show(); });
    recharge.addEventListener("click", close);

    async function refresh() {
      if (document.hidden) return;
      try {
        const response = await fetch("/api/sub2api/balance", {
          credentials: "same-origin",
          cache: "no-store",
          headers: { Accept: "application/json" },
        });
        if (!response.ok) { if (response.status === 401) { pill.style.display = "none"; close(); rechargeUrl = ""; lastInsufficient = null; } return; }
        const result = await response.json();
        const balance = Number(result.balance);
        if (!Number.isFinite(balance)) return;
        pill.style.display = "";
        const insufficient = balance <= 0;
        const formatted = money.format(balance);
        amount.textContent = formatted;
        dialogAmount.textContent = formatted;
        amount.classList.toggle("low", insufficient);
        rechargeUrl = "";
        const rawRechargeUrl = typeof result.recharge_url === "string" ? result.recharge_url.trim() : "";
        if (rawRechargeUrl) {
          try {
            const candidate = new URL(rawRechargeUrl);
            if ((candidate.protocol === "https:" || candidate.protocol === "http:") && !candidate.username && !candidate.password) rechargeUrl = candidate.href;
          } catch {}
        }
        recharge.href = rechargeUrl || "#";
        recharge.setAttribute("aria-disabled", String(!rechargeUrl));
        if (insufficient && lastInsufficient !== true) show();
        if (!insufficient) close();
        lastInsufficient = insufficient;
      } catch {}
    }

    void refresh();
    window.setInterval(refresh, 60_000);
    window.addEventListener("focus", refresh);
    document.addEventListener("visibilitychange", refresh);
  }

  function scan(node = document) {
    if (node instanceof Element && node.hasAttribute("data-sub2api-balance-widget")) mount(node);
    node.querySelectorAll?.("[data-sub2api-balance-widget]").forEach(mount);
  }

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", () => scan(), { once: true });
  else scan();
  new MutationObserver((records) => records.forEach((record) => record.addedNodes.forEach((node) => {
    if (node instanceof Element) scan(node);
  }))).observe(document.documentElement, { childList: true, subtree: true });
})();
