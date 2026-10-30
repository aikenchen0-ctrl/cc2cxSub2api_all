<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import QRCode from "qrcode";
import { useAgentSession } from "@/agent/session";
import { siteName, siteLogo, siteSubtitle, siteDocURL } from "@/agent/branding";
import { isDark, toggleTheme } from "@/agent/theme";
import Icon from "@/components/icons/Icon.vue";
import {
  ccswitchVersion,
  downloadOptions,
  recommendDownload,
} from "@/utils/ccswitch-downloads";
import modelPrices from "./reference-model-prices.json";

const session = useAgentSession();
const isAuthenticated = computed(() => session.isAuthenticated);
const dashboardPath = computed(() =>
  session.isAgentAdmin ? "/admin/dashboard" : "/dashboard",
);
const currentYear = new Date().getFullYear();
const heroSubtitle = computed(() =>
  siteSubtitle.value === "AI API Gateway Platform"
    ? "一个账号连接常用 AI 开发工具。通过本站 API 地址与个人密钥，接入兼容工具；使用 CCSwitch 管理桌面连接，在控制台查看余额、用量与订单。"
    : siteSubtitle.value,
);
const recommendation = recommendDownload();
const recommendedLabel =
  recommendation.platform === "other"
    ? "下载 CCSwitch 客户端"
    : recommendation.platform.startsWith("mac")
      ? "下载 macOS 版"
      : "下载 Windows 版";
const recommendedFilename = decodeURIComponent(
  new URL(recommendation.url).pathname.split("/").pop() || "",
);
const windowsDownloads = downloadOptions.filter((item) =>
  item.platforms.includes("windows"),
);
const macDownloads = downloadOptions.filter((item) =>
  item.platforms.some((platform) => platform.startsWith("mac")),
);
const mobileQR = ref("");
const selectedProvider = ref("全部");
const providers = [
  "全部",
  ...new Set(modelPrices.map((model) => model.provider)),
];
const modelsExpanded = ref(false);
const filteredModels = computed(() =>
  modelPrices.filter(
    (model) =>
      selectedProvider.value === "全部" ||
      model.provider === selectedProvider.value,
  ),
);
const visibleModels = computed(() =>
  modelsExpanded.value
    ? filteredModels.value
    : filteredModels.value.slice(0, 8),
);
const priceFields = [
  { key: "input", label: "输入" },
  { key: "output", label: "输出" },
  { key: "cache_write", label: "缓存写" },
  { key: "cache_read", label: "缓存读" },
] as const;
const price = (value: string) =>
  value === "-" || value === "" ? "-" : "$" + value;
function selectProvider(provider: string) {
  selectedProvider.value = provider;
  modelsExpanded.value = false;
}
const openFAQ = ref<number | null>(0);
const openHistory = ref<number | null>(null);
const mobileMenuOpen = ref(false);
const scrolled = ref(false);
const landingTheme = computed(() => (isDark.value ? "dark" : "light"));
const downloadHelp = [
  {
    title: "Windows 安装程序（EXE）",
    body: "普通用户可选 EXE 安装程序。安装后打开 CCSwitch，添加本站 API 地址和个人 API Key，再按需要配置兼容工具。",
  },
  {
    title: "Windows 安装包（MSI）",
    body: "MSI 格式适用于需要统一安装流程的设备。安装包与 EXE 使用同一 CCSwitch 版本。",
  },
  {
    title: "macOS Apple 芯片版",
    body: "搭载 Apple M 系列芯片的设备选择 Apple 芯片安装包；首次打开时按 macOS 的安全提示确认应用来源。",
  },
  {
    title: "macOS Intel 版",
    body: "搭载 Intel 处理器的设备选择 Intel 安装包。安装后可在 CCSwitch 中管理本地模型服务连接。",
  },
  {
    title: "Linux 与其他系统",
    body: "本站当前未提供 Linux 桌面安装包。可以在浏览器访问本站控制台，或使用支持兼容 API 的客户端。",
  },
];
const onScroll = () => {
  scrolled.value = window.scrollY > 12;
};
onMounted(() => {
  onScroll();
  window.addEventListener("scroll", onScroll, { passive: true });
  QRCode.toDataURL(window.location.origin + "/home", { width: 200, margin: 2 })
    .then((value) => {
      mobileQR.value = value;
    })
    .catch(() => {});
});
onUnmounted(() => {
  window.removeEventListener("scroll", onScroll);
});
</script>

<template>
  <div
    :data-theme="landingTheme"
    @keydown.esc="mobileMenuOpen = false"
    class="site"
    data-testid="reference-home"
  >
    <div aria-hidden="true" class="bg-decor">
      <span class="blob b1"></span><span class="blob b2"></span
      ><span class="blob b3"></span>
    </div>
    <div aria-hidden="true" class="bg-grid"></div>
    <div class="promo">
      <div class="container">
        <span class="dot"></span
        ><span
          >{{ siteName }} · <b>统一模型接入</b> · CCSwitch 桌面客户端 ·
          <b>用量与账单可查</b></span
        >
      </div>
    </div>
    <nav :class="{ scrolled }" class="nav">
      <div class="container">
        <a class="brand" href="#top"
          ><img :src="siteLogo" alt="" class="logo" /><span>{{
            siteName
          }}</span></a
        >
        <div class="nav-links">
          <a href="#features">核心能力</a><a href="#billing">怎么计费</a
          ><a href="#pricing">定价</a><a href="#download">下载</a
          ><a href="#trust">安心保障</a><a href="#faq">FAQ</a
          ><a
            :href="siteDocURL || '#faq'"
            :target="siteDocURL ? '_blank' : undefined"
            rel="noopener"
            >教程文档</a
          >
        </div>
        <div class="nav-cta">
          <div class="tt desktop-only" data-theme-toggle-root="">
            <button
              :title="isDark ? '切换浅色模式' : '切换深色模式'"
              @click="toggleTheme"
              aria-label="切换主题"
              class="tt-btn"
              data-testid="landing-theme-toggle"
              type="button"
            >
              <Icon :name="isDark ? 'sun' : 'moon'" size="sm" />
            </button>
          </div>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="btn btn-ghost btn-sm desktop-only"
            >{{ isAuthenticated ? "控制台" : "登录" }}</router-link
          ><a class="btn btn-primary btn-sm" href="#download">免费下载</a
          ><button
            :aria-expanded="mobileMenuOpen"
            @click="mobileMenuOpen = !mobileMenuOpen"
            aria-controls="landing-mobile-menu"
            aria-label="打开菜单"
            class="hamburger"
            type="button"
          >
            <svg
              fill="none"
              stroke="currentColor"
              stroke-width="1.8"
              viewBox="0 0 24 24"
            >
              <path d="M4 6h16M4 12h16M4 18h16"></path>
            </svg>
          </button>
        </div>
      </div>
      <div class="mobile-menu" id="landing-mobile-menu" v-show="mobileMenuOpen">
        <a @click="mobileMenuOpen = false" href="#features">核心能力</a
        ><a @click="mobileMenuOpen = false" href="#billing">怎么计费</a
        ><a @click="mobileMenuOpen = false" href="#pricing">定价</a
        ><a @click="mobileMenuOpen = false" href="#download">下载</a
        ><a @click="mobileMenuOpen = false" href="#trust">安心保障</a
        ><a @click="mobileMenuOpen = false" href="#faq">FAQ</a
        ><a
          :href="siteDocURL || '#faq'"
          :target="siteDocURL ? '_blank' : undefined"
          @click="mobileMenuOpen = false"
          rel="noopener"
          >教程文档</a
        >
        <div class="mm-row">
          <div class="tt" data-theme-toggle-root="">
            <button
              :title="isDark ? '切换浅色模式' : '切换深色模式'"
              @click="toggleTheme"
              aria-label="切换主题"
              class="tt-btn"
              data-testid="landing-theme-toggle"
              type="button"
            >
              <Icon :name="isDark ? 'sun' : 'moon'" size="sm" />
            </button>
          </div>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/register'"
            @click="mobileMenuOpen = false"
            class=""
            >登录 / 注册 →</router-link
          >
        </div>
      </div>
    </nav>
    <header class="hero" id="top">
      <div class="container">
        <span class="badge badge-brand">统一模型接入工作空间</span>
        <h1>
          <span class="ac-hot-text">一个入口</span>连接 AI<br /><span
            class="ac-grad-text"
            >让灵感即刻落地</span
          >
        </h1>
        <p class="lede">{{ heroSubtitle }}</p>
        <div class="hero-cta">
          <a
            :href="recommendation.url"
            class="btn btn-primary btn-lg"
            rel="noopener noreferrer"
            target="_blank"
            >{{ recommendedLabel }}</a
          ><a class="btn btn-ghost btn-lg" href="#billing">看计费怎么算 →</a>
        </div>
        <div class="hero-trust">
          <span
            ><svg
              fill="none"
              stroke="currentColor"
              stroke-width="2.2"
              viewBox="0 0 24 24"
            >
              <path d="M20 6L9 17l-5-5"></path></svg
            >独立账户管理</span
          ><span
            ><svg
              fill="none"
              stroke="currentColor"
              stroke-width="2.2"
              viewBox="0 0 24 24"
            >
              <path d="M20 6L9 17l-5-5"></path></svg
            >用量明细可查</span
          ><span
            ><svg
              fill="none"
              stroke="currentColor"
              stroke-width="2.2"
              viewBox="0 0 24 24"
            >
              <path d="M20 6L9 17l-5-5"></path></svg
            >Windows / macOS</span
          ><span
            ><svg
              fill="none"
              stroke="currentColor"
              stroke-width="2.2"
              viewBox="0 0 24 24"
            >
              <path d="M20 6L9 17l-5-5"></path></svg
            >多工具兼容接入</span
          >
        </div>
        <div class="stat-row">
          <div class="stat">
            <div class="v tabular-nums live">API</div>
            <div class="l">统一接入入口</div>
          </div>
          <div class="stat">
            <div class="v tabular-nums">CCSwitch</div>
            <div class="l">桌面配置管理</div>
          </div>
          <div class="stat">
            <div class="v tabular-nums">多工具</div>
            <div class="l">按需选择模型</div>
          </div>
          <div class="stat">
            <div class="v tabular-nums">可追溯</div>
            <div class="l">用量与订单记录</div>
          </div>
        </div>
        <div class="showcase">
          <div class="win">
            <div class="win-bar">
              <span class="wd r"></span><span class="wd y"></span
              ><span class="wd g"></span
              ><span class="wt">{{ siteName }} — 工作空间预览</span>
            </div>
            <div class="win-body">
              <div class="win-side">
                <div class="item on">
                  <svg
                    class="ic"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="1.6"
                    viewBox="0 0 24 24"
                  >
                    <path d="M3 12l9-9 9 9M5 10v10h14V10"></path></svg
                  >API 密钥
                </div>
                <div class="item">
                  <svg
                    class="ic"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="1.6"
                    viewBox="0 0 24 24"
                  >
                    <path d="M8 9l3 3-3 3M5 4h14v16H5z"></path></svg
                  >工具接入
                </div>
                <div class="item">
                  <svg
                    class="ic"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="1.6"
                    viewBox="0 0 24 24"
                  >
                    <path d="M12 18h.01M7 3h10v18H7z"></path></svg
                  >用量记录
                </div>
                <div class="item">
                  <svg
                    class="ic"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="1.6"
                    viewBox="0 0 24 24"
                  >
                    <path d="M3 3v18h18M7 14l3-3 3 3 5-5"></path></svg
                  >账单明细
                </div>
              </div>
              <div class="win-main">
                <div class="mini-row">
                  <div class="mini-title">统一入口，开始你的工作流</div>
                  <span class="badge badge-brand tabular-nums">功能示意</span>
                </div>
                <div class="route-card">
                  <div class="rc-top">
                    <span class="rc-name">统一 API 网关</span
                    ><span class="badge badge-ok">API 连接</span>
                  </div>
                  <div class="route-meta">
                    <span class="k">本站 API 地址</span
                    ><span class="k">个人专属 Key</span
                    ><span class="k tabular-nums">按实际用量记录</span>
                  </div>
                  <div class="switch-flow">
                    <span class="sf-step active">创建密钥</span
                    ><span class="arr">→</span
                    ><span class="sf-step active">配置工具</span
                    ><span class="arr">→</span
                    ><span class="sf-step">查看用量</span>
                  </div>
                </div>
                <div class="route-card dim">
                  <div class="rc-top">
                    <span class="rc-name">CCSwitch · 桌面配置管理</span
                    ><span class="badge badge-cyan">客户端配置</span>
                  </div>
                  <div class="route-meta">
                    <span class="k">Windows / macOS</span
                    ><span class="k tabular-nums">多工具配置</span
                    ><span class="k">本地配置文件</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div class="phone">
            <div class="pbar"></div>
            <div class="pbody">
              <div class="plabel">随时查看 · 工作空间</div>
              <div class="chat me">查看我的模型用量</div>
              <div class="chat ai">请求记录、Token 与费用在控制台查看。</div>
              <div class="chat ai">订单与余额，随时掌握</div>
            </div>
          </div>
        </div>
      </div>
    </header>
    <div class="band">
      <div class="container">
        <div class="band-title">
          一个网关地址 · 一个 API Key · 打通你惯用的每个工具
        </div>
        <div class="logos">
          <span class="logo-chip"
            ><svg
              class="ic"
              fill="none"
              stroke="currentColor"
              stroke-width="1.6"
              viewBox="0 0 24 24"
            >
              <path d="M12 2 3 7v10l9 5 9-5V7z"></path></svg
            >Cursor</span
          ><span class="logo-chip"
            ><svg
              class="ic"
              fill="none"
              stroke="currentColor"
              stroke-width="1.6"
              viewBox="0 0 24 24"
            >
              <path d="M8 9l3 3-3 3m5 0h3M4 4h16v16H4z"></path></svg
            >Codex CLI</span
          ><span class="logo-chip"
            ><svg
              class="ic"
              fill="none"
              stroke="currentColor"
              stroke-width="1.6"
              viewBox="0 0 24 24"
            >
              <path d="M12 3l9 5v8l-9 5-9-5V8z"></path></svg
            >Claude Code</span
          ><span class="logo-chip">OpenAI 兼容</span
          ><span class="logo-chip">Anthropic 兼容</span>
        </div>
        <div class="band-sub">
          连接常用 AI 开发工具与兼容 API。可用模型以本站控制台和账户权限为准。
        </div>
      </div>
    </div>
    <section id="features">
      <div class="container">
        <div class="sec-head">
          <span class="eyebrow">Why {{ siteName }}</span>
          <h2>多工具，一个入口</h2>
          <p>从模型接入到桌面配置、用量查询和账户管理，让日常工作更顺手。</p>
        </div>
        <div class="feat-grid">
          <div class="feat">
            <div class="ico">
              <svg
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                viewBox="0 0 24 24"
              >
                <path
                  d="M12 1v22M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"
                ></path>
              </svg>
            </div>
            <h3>用量清晰 · 记录可查</h3>
            <p>
              请求用量和费用以控制台实际记录为准，充值金额、币种和支付方式以本站收银台为准。
            </p>
            <div class="demo">
              <div class="calc-mini">
                <span class="chip">请求用量</span><span class="op">×</span
                ><span class="chip">模型计费</span><span class="op">×</span
                ><span class="chip">账户配置</span><span class="op">=</span
                ><span class="res">实际费用</span>
              </div>
            </div>
          </div>
          <div class="feat">
            <div class="ico">
              <svg
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                viewBox="0 0 24 24"
              >
                <path d="M8 9l3 3-3 3m5 0h3M4 4h16v16H4z"></path>
              </svg>
            </div>
            <h3>一个入口，连接常用工具</h3>
            <p>
              通过本站 API 地址与个人密钥，配置兼容工具；CCSwitch
              帮你在桌面管理服务连接。
            </p>
            <div class="demo">
              <div class="tool-mini">
                <span class="t">Cursor</span><span class="t">Codex CLI</span
                ><span class="t">Claude Code</span><span class="t">Python</span
                ><span class="t">curl</span>
              </div>
            </div>
          </div>
          <div class="feat">
            <div class="ico">
              <svg
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                viewBox="0 0 24 24"
              >
                <path
                  d="M17 1l4 4-4 4M3 11V9a4 4 0 0 1 4-4h14M7 23l-4-4 4-4M21 13v2a4 4 0 0 1-4 4H3"
                ></path>
              </svg>
            </div>
            <h3>个人密钥 · 独立管理</h3>
            <p>
              在控制台创建和管理个人 API
              Key，按工具需要配置，不依赖共享账号注入。
            </p>
            <div class="demo">
              <div class="switch-flow flat">
                <span class="sf-step active">创建密钥</span
                ><span class="arr">→</span
                ><span class="sf-step active">配置工具</span
                ><span class="arr">→</span><span class="sf-step">调用模型</span
                ><span class="arr">→</span
                ><span class="sf-step active">查看用量</span>
              </div>
            </div>
          </div>
          <div class="feat">
            <div class="ico">
              <svg
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                viewBox="0 0 24 24"
              >
                <rect height="20" rx="2" width="10" x="7" y="2"></rect>
                <path d="M11 18h2"></path>
              </svg>
            </div>
            <h3>随时掌握你的工作空间</h3>
            <p>
              通过浏览器查看余额、订单与调用记录，桌面客户端负责本地工具的连接配置。
            </p>
            <div class="demo">
              <div class="tool-mini">
                <span class="t">余额查询</span><span class="t">用量记录</span
                ><span class="t">订单管理</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
    <section class="alt" id="billing">
      <div class="container">
        <div class="sec-head">
          <span class="eyebrow">Transparent Billing</span>
          <h2>怎么用，算得清</h2>
          <p>计费规则以当前账户和本站配置为准；首页的官方模型价格仅供参考。</p>
        </div>
        <div class="billing-wrap">
          <div class="card billing-card">
            <div class="badge badge-brand mb">按量计费公式</div>
            <div class="formula">
              <span class="box a">请求用量</span><span class="fop">×</span
              ><span class="box b">模型计费</span><span class="fop">×</span
              ><span class="box c">账户配置</span><span class="fop">=</span
              ><span class="box r">实际费用</span>
            </div>
            <ul class="bill-list">
              <li>
                <svg
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.2"
                  viewBox="0 0 24 24"
                >
                  <path d="M20 6L9 17l-5-5"></path></svg
                >用量明细可查：在控制台查看请求、Token 与对应费用。
              </li>
              <li>
                <svg
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.2"
                  viewBox="0 0 24 24"
                >
                  <path d="M20 6L9 17l-5-5"></path></svg
                >本站独立配置：支付方式、充值限额和币种以本站收银台为准。
              </li>
              <li>
                <svg
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.2"
                  viewBox="0 0 24 24"
                >
                  <path d="M20 6L9 17l-5-5"></path></svg
                >余额不足提醒：遇到额度不足时，请先检查账户余额和调用权限。
              </li>
              <li>
                <svg
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.2"
                  viewBox="0 0 24 24"
                >
                  <path d="M20 6L9 17l-5-5"></path></svg
                >价格参考公开：下方提供官方 API
                价格快照，实际费用以本站账单为准。
              </li>
            </ul>
          </div>
          <div class="card compare">
            <h4>从接入到查询，一条清晰的路径</h4>
            <div class="bar official">
              <div class="lab">
                <span>创建个人密钥</span><span class="amt">控制台</span>
              </div>
              <div class="track"><div class="fill"></div></div>
            </div>
            <div class="bar ours">
              <div class="lab">
                <span class="ours-name">查看用量与费用</span
                ><span class="amt ours-amt">调用记录</span>
              </div>
              <div class="track">
                <div class="fill" style="width: 100%"></div>
              </div>
            </div>
            <div class="save-tag">每一次调用，<b>有据可查</b></div>
            <div class="disclaimer">
              实际扣费、充值与退款规则以本站配置和订单页面为准。
            </div>
          </div>
        </div>
      </div>
    </section>
    <section id="how">
      <div class="container">
        <div class="sec-head">
          <span class="eyebrow">Getting Started</span>
          <h2>4 步开始，接入你的工作流</h2>
          <p>注册、下载、配置、调用，按步骤即可开始。</p>
        </div>
        <div class="steps">
          <div class="step-c">
            <div class="n tabular-nums">01</div>
            <h4>注册账号</h4>
            <p>
              进入本站注册页，按页面提示完成注册。是否验证邮箱由本站配置决定。
            </p>
            <svg
              class="arrow"
              fill="none"
              stroke="currentColor"
              stroke-width="1.6"
              viewBox="0 0 24 24"
            >
              <path d="M5 12h14M13 6l6 6-6 6"></path>
            </svg>
          </div>
          <div class="step-c">
            <div class="n tabular-nums">02</div>
            <h4>下载客户端</h4>
            <p>根据当前系统下载 CCSwitch，安装后管理本地模型服务连接。</p>
            <svg
              class="arrow"
              fill="none"
              stroke="currentColor"
              stroke-width="1.6"
              viewBox="0 0 24 24"
            >
              <path d="M5 12h14M13 6l6 6-6 6"></path>
            </svg>
          </div>
          <div class="step-c">
            <div class="n tabular-nums">03</div>
            <h4>配置连接</h4>
            <p>在本站控制台创建 API Key，将本站地址与密钥填写到兼容工具中。</p>
            <svg
              class="arrow"
              fill="none"
              stroke="currentColor"
              stroke-width="1.6"
              viewBox="0 0 24 24"
            >
              <path d="M5 12h14M13 6l6 6-6 6"></path>
            </svg>
          </div>
          <div class="step-c">
            <div class="n tabular-nums">04</div>
            <h4>开始使用</h4>
            <p>在工具中调用模型，在控制台查看余额、用量和账单记录。</p>
          </div>
        </div>
      </div>
    </section>
    <section class="alt" id="pricing">
      <div class="container">
        <div class="sec-head">
          <span class="eyebrow">Pricing</span>
          <h2>简单透明的定价</h2>
          <p>模型价格可参考，实际扣费和可用范围以本站账户及配置为准。</p>
        </div>
        <div class="price-top">
          <div class="price-card">
            <div class="price-head">
              <span class="badge badge-brand">本站账户 · 用量可查</span>
              <div class="price-amt">
                <span class="ac-grad-text big tabular-nums">按量</span
                ><span class="unit">计费 · 以实际账单为准</span>
              </div>
              <p class="price-sub">
                充值方式、到账金额和退款规则以本站收银台为准
              </p>
            </div>
            <ul class="price-feats">
              <li>
                <svg
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.4"
                  viewBox="0 0 24 24"
                >
                  <path d="M20 6L9 17l-5-5"></path></svg
                >个人 API 密钥
              </li>
              <li>
                <svg
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.4"
                  viewBox="0 0 24 24"
                >
                  <path d="M20 6L9 17l-5-5"></path></svg
                >调用用量可查
              </li>
              <li>
                <svg
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.4"
                  viewBox="0 0 24 24"
                >
                  <path d="M20 6L9 17l-5-5"></path></svg
                >多工具兼容接入
              </li>
              <li>
                <svg
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.4"
                  viewBox="0 0 24 24"
                >
                  <path d="M20 6L9 17l-5-5"></path></svg
                >CCSwitch 客户端
              </li>
              <li>
                <svg
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.4"
                  viewBox="0 0 24 24"
                >
                  <path d="M20 6L9 17l-5-5"></path></svg
                >独立账户管理
              </li>
              <li>
                <svg
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.4"
                  viewBox="0 0 24 24"
                >
                  <path d="M20 6L9 17l-5-5"></path></svg
                >余额与订单记录
              </li>
              <li>
                <svg
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.4"
                  viewBox="0 0 24 24"
                >
                  <path d="M20 6L9 17l-5-5"></path></svg
                >本站支付配置
              </li>
              <li>
                <svg
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2.4"
                  viewBox="0 0 24 24"
                >
                  <path d="M20 6L9 17l-5-5"></path></svg
                >站长文档支持
              </li>
            </ul>
            <a class="btn btn-primary btn-block btn-lg" href="#download"
              >免费下载，配置即用</a
            >
          </div>
        </div>
        <div class="price-table-wrap">
          <div class="pt-head">
            <h3>模型价格参考</h3>
            <a
              class="pt-src"
              href="https://cursor.com/docs/models-and-pricing#model-pricing"
              rel="noopener noreferrer"
              target="_blank"
            >
              Cursor 官方价格
              <svg
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                viewBox="0 0 24 24"
              >
                <path
                  d="M10 6H6a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-4M14 4h6v6M20 4L10 14"
                ></path></svg
            ></a>
          </div>
          <p class="pt-desc">
            以下为官方 API 价格参考快照（每百万
            tokens，2026-10-21）；不代表本站售价，也不承诺全部模型可用。实际费用以本站账单为准。
          </p>
          <div class="filters">
            <button
              :aria-pressed="selectedProvider === provider"
              :class="{ on: selectedProvider === provider }"
              :key="provider"
              @click="selectProvider(provider)"
              type="button"
              v-for="provider in providers"
            >
              {{ provider }}
            </button>
          </div>
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>模型</th>
                  <th>提供商</th>
                  <th class="r">输入</th>
                  <th class="r">缓存写入</th>
                  <th class="r">缓存读取</th>
                  <th class="r">输出</th>
                  <th>备注</th>
                </tr>
              </thead>
              <tbody>
                <tr :key="model.name" v-for="model in visibleModels">
                  <td>
                    <a
                      :href="model.link"
                      class="mn link"
                      rel="noopener noreferrer"
                      target="_blank"
                      v-if="model.link"
                      >{{ model.name }}</a
                    ><span class="mn" v-if="!model.link">{{ model.name }}</span>
                  </td>
                  <td>
                    <span class="pv">{{ model.provider }}</span>
                  </td>
                  <td class="r mono">{{ price(model.input) }}</td>
                  <td class="r mono pv">{{ price(model.cache_write) }}</td>
                  <td class="r mono pv">{{ price(model.cache_read) }}</td>
                  <td class="r mono out">{{ price(model.output) }}</td>
                  <td class="note">{{ model.notes }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="price-cards">
            <div :key="model.name" class="pc" v-for="model in visibleModels">
              <div class="pc-top">
                <a
                  :href="model.link"
                  class="mn link"
                  rel="noopener noreferrer"
                  target="_blank"
                  v-if="model.link"
                  >{{ model.name }}</a
                ><span class="mn" v-if="!model.link">{{ model.name }}</span
                ><span class="pv">{{ model.provider }}</span>
              </div>
              <div class="pc-grid">
                <div :key="field.key" v-for="field in priceFields">
                  <span class="pv">{{ field.label }}</span
                  ><span
                    :class="{ out: field.key === 'output' }"
                    class="mono"
                    >{{ price(model[field.key]) }}</span
                  >
                </div>
              </div>
              <div class="pc-note">{{ model.notes }}</div>
            </div>
          </div>
          <div class="pt-expand">
            <button
              :aria-expanded="modelsExpanded"
              @click="modelsExpanded = !modelsExpanded"
              class="btn btn-ghost btn-sm"
              type="button"
              v-if="filteredModels.length &gt; 8"
            >
              <span>{{
                modelsExpanded
                  ? "收起模型列表"
                  : "展开全部 " +
                    filteredModels.length +
                    " 个模型（还有 " +
                    (filteredModels.length - 8) +
                    " 个）"
              }}</span
              ><svg
                class="chev"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                viewBox="0 0 24 24"
              >
                <path d="M19 9l-7 7-7-7"></path>
              </svg>
            </button>
          </div>
          <p class="pt-foot">
            价格参考快照 · 2026-10-21 · 不作为本站扣费报价。
          </p>
        </div>
      </div>
    </section>
    <section id="download">
      <div class="container">
        <div class="sec-head">
          <span class="eyebrow">Download</span>
          <h2>下载客户端</h2>
          <p>CCSwitch v{{ ccswitchVersion }} · 官方发布渠道</p>
        </div>
        <div class="dl-hero">
          <a
            :href="recommendation.url"
            class="btn btn-primary btn-lg"
            rel="noopener noreferrer"
            target="_blank"
            >{{ recommendedLabel }}</a
          >
          <div class="dl-file tabular-nums">
            {{ recommendedFilename }} · CCSwitch v{{ ccswitchVersion }}
          </div>
          <div class="oneclick">
            <svg
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              viewBox="0 0 24 24"
            >
              <path d="M12 3v18M3 12h18"></path>
            </svg>
            安装后添加本站 API 地址和个人密钥，按需管理桌面连接。
          </div>
          <div class="dl-tut">
            不会用？
            <a
              :href="siteDocURL || '#faq'"
              :target="siteDocURL ? '_blank' : undefined"
              rel="noopener noreferrer"
              >查看本站接入说明 →</a
            >
          </div>
        </div>
        <div class="dl-area">
          <div class="dl-grid">
            <div class="dl-card">
              <div class="dl-card-head">
                <div class="os">
                  <svg fill="currentColor" viewBox="0 0 24 24">
                    <path
                      d="M3 12V6.75l8-1.25V12H3zm0 .5h8v6.5l-8-1.25V12.5zM11.5 12V5.35l9.5-1.6V12h-9.5zm0 .5h9.5v8.25l-9.5-1.6V12.5z"
                    ></path>
                  </svg>
                </div>
                <div class="os-meta">
                  <div class="os-name">Windows</div>
                  <div class="os-arch">x86_64</div>
                </div>
              </div>
              <div class="dl-card-body">
                <a
                  :href="item.url"
                  :key="item.key"
                  class="dl-variant"
                  rel="noopener noreferrer"
                  target="_blank"
                  v-for="item in windowsDownloads"
                  ><div class="dv-meta">
                    <div class="dv-row1">
                      <span class="dv-label">{{ item.title }}</span
                      ><span class="dv-size">v{{ ccswitchVersion }}</span>
                    </div>
                    <div class="dv-desc">{{ item.description }}</div>
                  </div>
                  <svg
                    class="dv-arrow"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="1.5"
                    viewBox="0 0 24 24"
                  >
                    <path
                      d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"
                    ></path></svg
                ></a>
              </div>
            </div>
            <div class="dl-card">
              <div class="dl-card-head">
                <div class="os">
                  <svg fill="currentColor" viewBox="0 0 24 24">
                    <path
                      d="M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47-1.34.03-1.77-.79-3.29-.79-1.53 0-2 .77-3.27.82-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51 1.28-.02 2.5.87 3.29.87.78 0 2.26-1.07 3.8-.91.65.03 2.47.26 3.64 1.98-.09.06-2.17 1.28-2.15 3.81.03 3.02 2.65 4.03 2.68 4.04-.03.07-.42 1.44-1.38 2.83M13 3.5c.73-.83 1.94-1.46 2.94-1.5.13 1.17-.34 2.35-1.04 3.19-.69.85-1.83 1.51-2.95 1.42-.15-1.15.41-2.35 1.05-3.11z"
                    ></path>
                  </svg>
                </div>
                <div class="os-meta">
                  <div class="os-name">macOS</div>
                  <div class="os-arch">Apple Silicon / Intel</div>
                </div>
              </div>
              <div class="dl-card-body">
                <a
                  :href="item.url"
                  :key="item.key"
                  class="dl-variant"
                  rel="noopener noreferrer"
                  target="_blank"
                  v-for="item in macDownloads"
                  ><div class="dv-meta">
                    <div class="dv-row1">
                      <span class="dv-label">{{ item.title }}</span
                      ><span class="dv-size">v{{ ccswitchVersion }}</span>
                    </div>
                    <div class="dv-desc">{{ item.description }}</div>
                  </div>
                  <svg
                    class="dv-arrow"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="1.5"
                    viewBox="0 0 24 24"
                  >
                    <path
                      d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"
                    ></path></svg
                ></a>
              </div>
            </div>
            <div class="dl-card">
              <div class="dl-card-head">
                <div class="os">
                  <svg fill="currentColor" viewBox="0 0 24 24">
                    <path
                      d="M14.62 8.35c-.42.28-1.75.86-1.75.86-.28.14-.28.42 0 .56 0 0 1.33.58 1.75.86.42.28.7 1.4.7 1.4s.28-1.12.7-1.4c.42-.28 1.75-.86 1.75-.86.28-.14.28-.42 0-.56 0 0-1.33-.58-1.75-.86-.42-.28-.7-1.4-.7-1.4s-.28 1.12-.7 1.4M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2m0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8"
                    ></path>
                  </svg>
                </div>
                <div class="os-meta">
                  <div class="os-name">其他设备</div>
                  <div class="os-arch">浏览器 / API</div>
                </div>
              </div>
              <div class="dl-card-body">
                <router-link class="dl-variant" to="/downloads"
                  ><div class="dv-meta">
                    <div class="dv-row1">
                      <span class="dv-label">查看全部下载</span>
                    </div>
                    <div class="dv-desc">
                      本站当前提供 Windows 与 macOS 安装包。
                    </div>
                  </div></router-link
                ><router-link :to="dashboardPath" class="dl-variant"
                  ><div class="dv-meta">
                    <div class="dv-row1">
                      <span class="dv-label">访问控制台</span>
                    </div>
                    <div class="dv-desc">
                      其他系统可通过兼容 API 工具连接本站。
                    </div>
                  </div></router-link
                >
                <div class="m-desk-card">
                  <div class="m-desk-title">Linux 与其他兼容工具</div>
                  <p>
                    本站未提供 Linux
                    桌面安装包。可使用命令行或开发工具，通过兼容 API 连接本站。
                  </p>
                  <div class="m-desk-btns">
                    <a
                      class="btn btn-ghost btn-sm"
                      :href="siteDocURL || '#faq'"
                      :target="siteDocURL ? '_blank' : undefined"
                      rel="noopener noreferrer"
                      >接入说明</a
                    >
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div class="dl-extra">
            <router-link to="/downloads">查看全部安装包 →</router-link> ·
            <a :href="siteDocURL || '#faq'">不会用？查看接入说明 →</a>
          </div>
          <div class="notes-card">
            <h3>CCSwitch · 连接配置说明</h3>
            <div class="release-notes notes-body">
              <p>
                安装后添加本站 API 地址和个人密钥，在兼容工具中选择对应模型。
              </p>
              <ul>
                <li>个人密钥可在本站控制台创建和管理。</li>
                <li>下载链接沿用本站当前版本，安装包来自官方发布渠道。</li>
                <li>Windows 提供 EXE 和 MSI 两种安装格式。</li>
                <li>macOS 按 Apple 芯片或 Intel 处理器选择对应安装包。</li>
                <li>其他系统可以在浏览器访问控制台，或使用兼容 API 工具。</li>
              </ul>
            </div>
          </div>
        </div>
        <div class="history" id="release-notes">
          <div class="hist-title">安装与接入说明</div>
          <div class="hist-list">
            <div
              :class="{ open: openHistory === index }"
              :key="item.title"
              class="hist-item"
              v-for="(item, index) in downloadHelp"
            >
              <button
                :aria-expanded="openHistory === index"
                @click="openHistory = openHistory === index ? null : index"
                class="hist-q"
                type="button"
              >
                <span class="hist-ver">{{ item.title }}</span
                ><span class="hist-date">本站指南</span
                ><svg
                  class="chev"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  viewBox="0 0 24 24"
                >
                  <path d="M19 9l-7 7-7-7"></path>
                </svg>
              </button>
              <div class="hist-body" v-show="openHistory === index">
                {{ item.body }}
              </div>
            </div>
          </div>
        </div>
        <div class="app-box glass">
          <div class="app-visual">
            <div class="app-qr">
              <img
                :src="mobileQR"
                alt="扫码访问本站"
                height="112"
                v-if="mobileQR"
                width="112"
              />
            </div>
          </div>
          <div class="app-info">
            <h4>手机用户？浏览器也能查看</h4>
            <p>
              扫码打开本站，查看账户、余额、用量和订单，无需安装其他站点 App。
            </p>
            <div class="app-btns">
              <router-link :to="dashboardPath" class="btn btn-ghost btn-sm"
                >进入工作空间 →</router-link
              ><span class="app-scan">用手机扫码访问本站</span>
            </div>
          </div>
        </div>
      </div>
    </section>
    <section id="docs">
      <div class="container">
        <div class="sec-head">
          <span class="eyebrow">Docs &amp; Guides</span>
          <h2>看文档，少走弯路</h2>
          <p>
            从注册账号到配置工具、查看用量，按本站指南开始使用。未配置文档时，可先查看下方常见问题。
          </p>
        </div>
        <div class="docs-grid">
          <a
            :href="siteDocURL || '#faq'"
            :target="siteDocURL ? '_blank' : undefined"
            class="doc-card"
            rel="noopener"
            ><div class="dc-ico">
              <svg
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                viewBox="0 0 24 24"
              >
                <path
                  d="M4 5a2 2 0 0 1 2-2h8l6 6v10a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2z"
                ></path>
                <path d="M14 3v6h6"></path>
              </svg>
            </div>
            <h3>新手接入指南</h3>
            <p>从创建账号到发起第一条请求。</p>
            <span class="dc-link">阅读 →</span></a
          ><a
            :href="siteDocURL || '#faq'"
            :target="siteDocURL ? '_blank' : undefined"
            class="doc-card"
            rel="noopener"
            ><div class="dc-ico">
              <svg
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                viewBox="0 0 24 24"
              >
                <rect height="20" rx="2" width="10" x="7" y="2"></rect>
                <path d="M11 18h2"></path>
              </svg>
            </div>
            <h3>客户端配置</h3>
            <p>下载 CCSwitch，配置本站连接。</p>
            <span class="dc-link">阅读 →</span></a
          ><a
            :href="siteDocURL || '#faq'"
            :target="siteDocURL ? '_blank' : undefined"
            class="doc-card"
            rel="noopener"
            ><div class="dc-ico">
              <svg
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                viewBox="0 0 24 24"
              >
                <circle cx="12" cy="12" r="9"></circle>
                <path d="M9.5 9a2.5 2.5 0 0 1 4.5 1.5c0 1.5-2 2-2 3"></path>
                <path d="M12 17h.01"></path>
              </svg>
            </div>
            <h3>常见问题 FAQ</h3>
            <p>接入、账户和费用问题。</p>
            <span class="dc-link">阅读 →</span></a
          ><a
            :href="siteDocURL || '#faq'"
            :target="siteDocURL ? '_blank' : undefined"
            class="doc-card"
            rel="noopener"
            ><div class="dc-ico">
              <svg
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                viewBox="0 0 24 24"
              >
                <path d="M3 3v18h18"></path>
                <path d="M7 14l3-3 3 3 5-5"></path>
              </svg>
            </div>
            <h3>用量与账单</h3>
            <p>了解用量记录和账户余额。</p>
            <span class="dc-link">阅读 →</span></a
          >
        </div>
        <div class="docs-cta">
          <a
            :href="siteDocURL || '#faq'"
            :target="siteDocURL ? '_blank' : undefined"
            class="btn btn-primary btn-lg"
            rel="noopener"
            ><svg
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              viewBox="0 0 24 24"
            >
              <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"></path>
              <path
                d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"
              ></path>
            </svg>
            查看完整文档
          </a>
        </div>
      </div>
    </section>
    <section class="alt" id="trust">
      <div class="container">
        <div class="sec-head">
          <span class="eyebrow">Trust &amp; Safety</span>
          <h2>放心用的底气</h2>
          <p>把风险降到最低，把安全握在手里。</p>
        </div>
        <div class="promise-grid">
          <div class="promise">
            <div class="pic">
              <svg
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                viewBox="0 0 24 24"
              >
                <path d="M12 2l7 4v6c0 5-3.5 8-7 10-3.5-2-7-5-7-10V6z"></path>
                <path d="M9 12l2 2 4-4"></path>
              </svg>
            </div>
            <h4>独立账户</h4>
            <p>每位用户使用自己的账户与工作空间。</p>
          </div>
          <div class="promise">
            <div class="pic">
              <svg
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                viewBox="0 0 24 24"
              >
                <path d="M3 12a9 9 0 1 0 9-9 9 9 0 0 0-6 2.3L3 8"></path>
                <path d="M3 3v5h5"></path>
              </svg>
            </div>
            <h4>订单可查</h4>
            <p>支付方式和售后规则以本站订单为准。</p>
          </div>
          <div class="promise">
            <div class="pic">
              <svg
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                viewBox="0 0 24 24"
              >
                <rect height="10" rx="2" width="16" x="4" y="10"></rect>
                <path d="M8 10V7a4 4 0 0 1 8 0v3"></path>
              </svg>
            </div>
            <h4>个人密钥</h4>
            <p>在控制台创建和管理自己的 API Key。</p>
          </div>
          <div class="promise">
            <div class="pic">
              <svg
                fill="none"
                stroke="currentColor"
                stroke-width="1.8"
                viewBox="0 0 24 24"
              >
                <path d="M22 12h-4l-3 9L9 3l-3 9H2"></path>
              </svg>
            </div>
            <h4>规则透明</h4>
            <p>可用模型、价格和额度以实际配置为准。</p>
          </div>
        </div>
        <div class="reviews">
          <div class="review">
            <div class="stars">开发工作流</div>
            <p>
              用你熟悉的编辑器和命令行工具开始工作，CCSwitch
              帮助管理本地模型服务配置。
            </p>
            <div class="who">
              <div class="av">D</div>
              <div>
                <div class="nm">桌面开发</div>
                <div class="rl">Windows / macOS</div>
              </div>
            </div>
          </div>
          <div class="review">
            <div class="stars">兼容 API</div>
            <p>
              使用本站地址和个人密钥对接兼容工具，可用协议和模型以实际账户权限为准。
            </p>
            <div class="who">
              <div class="av">A</div>
              <div>
                <div class="nm">工具接入</div>
                <div class="rl">API 地址 + 个人密钥</div>
              </div>
            </div>
          </div>
          <div class="review">
            <div class="stars">账户管理</div>
            <p>
              余额、调用用量和订单集中在一个工作空间，按本站实际规则充值与使用。
            </p>
            <div class="who">
              <div class="av">U</div>
              <div>
                <div class="nm">个人控制台</div>
                <div class="rl">余额 / 用量 / 订单</div>
              </div>
            </div>
          </div>
        </div>
        <div class="ph-note">
          功能说明展示；页面不使用虚构用户评价或经营数据。
        </div>
      </div>
    </section>
    <section id="faq">
      <div class="container">
        <div class="sec-head">
          <span class="eyebrow">FAQ</span>
          <h2>常见问题</h2>
        </div>
        <div class="faq">
          <div :class="{ open: openFAQ === 0 }" class="faq-item">
            <button
              :aria-expanded="openFAQ === 0"
              @click="openFAQ = openFAQ === 0 ? null : 0"
              aria-controls="faq-answer-0"
              class="faq-q"
              type="button"
            >
              <span>如何开始使用？</span
              ><svg
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                viewBox="0 0 24 24"
              >
                <path d="M6 9l6 6 6-6"></path>
              </svg>
            </button>
            <div :aria-hidden="openFAQ !== 0" class="faq-a" id="faq-answer-0">
              <div>
                进入本站注册页，按页面提示完成注册。下载 CCSwitch
                后，在控制台创建个人 API Key，将本站地址与密钥配置到兼容工具。
              </div>
            </div>
          </div>
          <div :class="{ open: openFAQ === 1 }" class="faq-item">
            <button
              :aria-expanded="openFAQ === 1"
              @click="openFAQ = openFAQ === 1 ? null : 1"
              aria-controls="faq-answer-1"
              class="faq-q"
              type="button"
            >
              <span>按量计费是怎么算的？</span
              ><svg
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                viewBox="0 0 24 24"
              >
                <path d="M6 9l6 6 6-6"></path>
              </svg>
            </button>
            <div :aria-hidden="openFAQ !== 1" class="faq-a" id="faq-answer-1">
              <div>
                实际费用以本站配置和账户账单为准。模型表是官方 API
                价格参考快照，不代表本站售价或可用模型承诺。
              </div>
            </div>
          </div>
          <div :class="{ open: openFAQ === 2 }" class="faq-item">
            <button
              :aria-expanded="openFAQ === 2"
              @click="openFAQ = openFAQ === 2 ? null : 2"
              aria-controls="faq-answer-2"
              class="faq-q"
              type="button"
            >
              <span>余额不足会怎样？会影响我本地的 Cursor 吗？</span
              ><svg
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                viewBox="0 0 24 24"
              >
                <path d="M6 9l6 6 6-6"></path>
              </svg>
            </button>
            <div :aria-hidden="openFAQ !== 2" class="faq-a" id="faq-answer-2">
              <div>
                余额不足时请在控制台查看账户状态，并按本站开放的支付方式充值。本站使用
                API 连接，不注入或恢复本地 Cursor 共享账号。
              </div>
            </div>
          </div>
          <div :class="{ open: openFAQ === 3 }" class="faq-item">
            <button
              :aria-expanded="openFAQ === 3"
              @click="openFAQ = openFAQ === 3 ? null : 3"
              aria-controls="faq-answer-3"
              class="faq-q"
              type="button"
            >
              <span>除了 Cursor，还能接入哪些工具？</span
              ><svg
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                viewBox="0 0 24 24"
              >
                <path d="M6 9l6 6 6-6"></path>
              </svg>
            </button>
            <div :aria-hidden="openFAQ !== 3" class="faq-a" id="faq-answer-3">
              <div>
                根据工具支持的协议，可使用本站地址与个人 API Key 配置 Codex
                CLI、Claude Code，以及兼容 API 的工具。
              </div>
            </div>
          </div>
          <div :class="{ open: openFAQ === 4 }" class="faq-item">
            <button
              :aria-expanded="openFAQ === 4"
              @click="openFAQ = openFAQ === 4 ? null : 4"
              aria-controls="faq-answer-4"
              class="faq-q"
              type="button"
            >
              <span>支持哪些操作系统与模型？</span
              ><svg
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                viewBox="0 0 24 24"
              >
                <path d="M6 9l6 6 6-6"></path>
              </svg>
            </button>
            <div :aria-hidden="openFAQ !== 4" class="faq-a" id="faq-answer-4">
              <div>
                本站下载页目前提供 CCSwitch 的 Windows 与 macOS
                安装包。其他系统可通过兼容 API
                工具接入；可用模型以账户权限为准。
              </div>
            </div>
          </div>
          <div :class="{ open: openFAQ === 5 }" class="faq-item">
            <button
              :aria-expanded="openFAQ === 5"
              @click="openFAQ = openFAQ === 5 ? null : 5"
              aria-controls="faq-answer-5"
              class="faq-q"
              type="button"
            >
              <span>是否需要押金？可以退款吗？</span
              ><svg
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                viewBox="0 0 24 24"
              >
                <path d="M6 9l6 6 6-6"></path>
              </svg>
            </button>
            <div :aria-hidden="openFAQ !== 5" class="faq-a" id="faq-answer-5">
              <div>
                充值金额、到账额度、支付方式和退款条件由本站配置决定，请以收银台、订单页面和站长说明为准。
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
    <section>
      <div class="container">
        <div class="final">
          <h2>准备好把编程效率拉满了吗？</h2>
          <p>连接常用 AI 工具，从创建本站账号和配置个人密钥开始。</p>
          <div class="hero-cta center">
            <a
              :href="recommendation.url"
              class="btn btn-primary btn-lg"
              rel="noopener noreferrer"
              target="_blank"
              ><svg fill="currentColor" viewBox="0 0 24 24">
                <path
                  d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"
                ></path>
              </svg>
              免费下载客户端 </a
            ><router-link
              :to="isAuthenticated ? dashboardPath : '/register'"
              class="btn btn-ghost btn-lg"
              >注册账号</router-link
            >
          </div>
        </div>
      </div>
    </section>
    <footer>
      <div class="container">
        <div class="foot-grid">
          <div class="foot-brand">
            <a class="brand" href="#top"
              ><img :src="siteLogo" alt="" class="logo" /><span>{{
                siteName
              }}</span></a
            >
            <p>{{ siteSubtitle }}</p>
          </div>
          <div class="foot-links">
            <div class="foot-col">
              <h5>产品</h5>
              <a href="#features">核心能力</a><a href="#pricing">定价</a
              ><a href="#download">下载客户端</a
              ><router-link :to="dashboardPath" class=""
                >移动端控制台</router-link
              >
            </div>
            <div class="foot-col">
              <h5>资源</h5>
              <a
                :href="siteDocURL || '#faq'"
                :target="siteDocURL ? '_blank' : undefined"
                rel="noopener"
                >帮助文档</a
              ><a
                :href="siteDocURL || '#faq'"
                :target="siteDocURL ? '_blank' : undefined"
                rel="noopener"
                >新手教程</a
              ><a
                :href="siteDocURL || '#faq'"
                :target="siteDocURL ? '_blank' : undefined"
                rel="noopener"
                >常见问题</a
              ><a
                :href="siteDocURL || '#faq'"
                :target="siteDocURL ? '_blank' : undefined"
                rel="noopener"
                >更新日志</a
              ><a href="#billing">计费说明</a>
            </div>
            <div class="foot-col">
              <h5>账户</h5>
              <router-link rel="noopener" to="/register">注册账号</router-link
              ><router-link rel="noopener" to="/profile">账户设置</router-link>
            </div>
          </div>
        </div>
        <div class="foot-bottom">
          <span>© {{ currentYear }} {{ siteName }}. 保留所有权利。</span
          ><span>统一模型接入 · 用量记录可查</span>
        </div>
      </div>
    </footer>
    <div class="mobile-cta">
      <router-link
        :to="isAuthenticated ? dashboardPath : '/register'"
        class="btn btn-ghost mc-1"
        >注册</router-link
      ><a
        :href="recommendation.url"
        class="btn btn-primary mc-2"
        rel="noopener noreferrer"
        target="_blank"
        >免费下载客户端</a
      >
    </div>
  </div>
</template>

<style scoped>
.site[data-theme="dark"] {
  --brand-1: #e8d29a;
  --brand-2: #d4ad5a;
  --cyan: #f0dfb2;
  --brand-soft: #d4ad5a24;
  --cyan-soft: #f0dfb21a;
  --grad-brand: linear-gradient(105deg, #f0dfb2, #d4ad5a 52%, #b3872c);
  --grad-brand-cyan: linear-gradient(105deg, #fbf1d5, #e2c273 55%, #c99f37);
  --bg: #0e0c09;
  --surface-1: #17130f;
  --surface-2: #221c16;
  --surface-3: #2e271e;
  --card: #221c16c7;
  --card-solid: #191510;
  --ink: #f6f0e2;
  --ink-2: #c4b7a0;
  --ink-3: #92856f;
  --line: #4a3f2d5c;
  --line-strong: #d4ad5a5c;
  --ok: #79c395;
  --ok-soft: #79c3951f;
  --warn: #e2c273;
  --warn-soft: #e2c27324;
  --danger: #e07068;
  --danger-soft: #e0706824;
  --info: #d4ad5a;
  --shadow-sm: 0 1px 2px #00000066;
  --shadow: 0 10px 30px -10px #000000b3, 0 2px 8px #00000080;
  --shadow-lg: 0 30px 70px -20px #000000d9;
  --glow: 0 0 0 1px #d4ad5a3d, 0 20px 60px -20px #c99f3752;
  --nav-bg: #0e0c09cc;
  --radius-card: 16px;
  --radius-btn: 12px;
  --radius-input: 10px;
  --radius-sm: 8px;
  --radius-pill: 999px;
  --dur-fast: 0.15s;
  --dur: 0.2s;
  --dur-slow: 0.25s;
  --ease: cubic-bezier(0.4, 0, 0.2, 1);
  --t-surface: #17130f;
  --t-surface-secondary: #221c16;
  --t-surface-card: #2a231b;
  --t-surface-hover: #352c22;
  --t-on-surface: #f6f0e2;
  --t-on-surface-secondary: #c4b7a0;
  --t-on-surface-muted: #92856f;
  --t-accent: #e2c273;
  --t-accent-hover: #f0dfb2;
  --t-accent-secondary: #d4ad5a;
  --t-accent-glow: #e2c2731f;
  --t-border: #4a3f2d99;
  --t-border-accent: #d4ad5a4d;
  --t-gradient-from: #f0dfb2;
  --t-gradient-to: #c99f37;
  color-scheme: dark;
}
.site[data-theme="light"] {
  --brand-1: #b3872c;
  --brand-2: #9a7018;
  --cyan: #c99f37;
  --brand-soft: #c99f3718;
  --cyan-soft: #e2c27318;
  --grad-brand: linear-gradient(105deg, #d4ad5a, #b3872c);
  --grad-brand-cyan: linear-gradient(
    105deg,
    #b3872c,
    #c99f37 55%,
    #e2c273
  );
  --bg: #fbfaf7;
  --surface-1: #ffffff;
  --surface-2: #f7f4ed;
  --surface-3: #eee8dc;
  --card: #fffffff2;
  --card-solid: #fff;
  --ink: #2a241c;
  --ink-2: #6b6152;
  --ink-3: #92856f;
  --line: #e6dfd1;
  --line-strong: #cdbb95;
  --ok: #3f7a58;
  --ok-soft: #3f7a5818;
  --warn: #a5761d;
  --warn-soft: #a5761d18;
  --danger: #b3413a;
  --danger-soft: #b3413a1f;
  --info: #8c681f;
  --shadow-sm: 0 1px 2px #3a2e1614;
  --shadow: 0 10px 30px -12px #3a2e1624, 0 2px 6px #3a2e160d;
  --shadow-lg: 0 30px 60px -20px #3a2e1630;
  --glow: 0 0 0 1px #c99f3738, 0 20px 50px -20px #c99f3724;
  --nav-bg: #fbfaf7f2;
  --radius-card: 16px;
  --radius-btn: 12px;
  --radius-input: 10px;
  --radius-sm: 8px;
  --radius-pill: 999px;
  --dur-fast: 0.15s;
  --dur: 0.2s;
  --dur-slow: 0.25s;
  --ease: cubic-bezier(0.4, 0, 0.2, 1);
  --t-surface: #fbfaf7;
  --t-surface-secondary: #ffffff;
  --t-surface-card: #f7f4ed;
  --t-surface-hover: #f0ebdf;
  --t-on-surface: #2a241c;
  --t-on-surface-secondary: #6b6152;
  --t-on-surface-muted: #92856f;
  --t-accent: #9a7018;
  --t-accent-hover: #7d5a13;
  --t-accent-secondary: #b3872c;
  --t-accent-glow: #c99f3718;
  --t-border: #e6dfd1;
  --t-border-accent: #c99f3766;
  --t-gradient-from: #d4ad5a;
  --t-gradient-to: #b3872c;
  color-scheme: light;
}
.site[data-theme="light"] .bg-decor .blob {
  display: none;
}
.site[data-theme="light"] .bg-grid {
  opacity: 0.24;
}
.site {
  position: relative;
  min-height: 100vh;
  font-size: 15px;
  line-height: 1.6;
  overflow-x: clip;
}
.site a {
  color: inherit;
  text-decoration: none;
}
.site h1,
.site h2,
.site h3,
.site h4 {
  margin: 0;
  letter-spacing: -0.02em;
  font-weight: 700;
}
.site p {
  margin: 0;
}
.container {
  max-width: 1120px;
  margin: 0 auto;
  padding: 0 24px;
}
.eyebrow {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 12.5px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--brand-2);
}
.eyebrow:before {
  content: "";
  width: 22px;
  height: 2px;
  border-radius: 2px;
  background: var(--grad-brand);
}
section {
  padding: 84px 0;
  position: relative;
  scroll-margin-top: 90px;
}
section.alt {
  background: var(--surface-1);
}
.sec-head {
  text-align: center;
  max-width: 660px;
  margin: 0 auto 48px;
}
.sec-head h2 {
  font-size: clamp(1.6rem, 3.4vw, 2.4rem);
  margin: 14px 0 12px;
  line-height: 1.15;
}
.sec-head p {
  color: var(--ink-2);
  font-size: 1.02rem;
}
.bg-decor {
  position: fixed;
  inset: 0;
  z-index: -1;
  pointer-events: none;
  overflow: hidden;
}
.bg-decor .blob {
  position: absolute;
  border-radius: 50%;
  filter: blur(130px);
  opacity: 0.5;
}
.bg-decor .b1 {
  top: -160px;
  left: 50%;
  transform: translate(-50%);
  width: 760px;
  height: 520px;
  background: radial-gradient(circle, rgba(201, 159, 55, 0.34), transparent 70%);
}
.bg-decor .b2 {
  top: 520px;
  right: -160px;
  width: 520px;
  height: 520px;
  background: radial-gradient(
    circle,
    rgba(226, 194, 115, 0.24),
    transparent 70%
  );
}
.bg-decor .b3 {
  top: 1400px;
  left: -160px;
  width: 480px;
  height: 480px;
  background: radial-gradient(circle, rgba(212, 173, 90, 0.16), transparent 70%);
}
.bg-grid {
  position: fixed;
  inset: 0;
  z-index: -1;
  pointer-events: none;
  opacity: 0.5;
  background-image:
    linear-gradient(var(--line) 1px, transparent 1px),
    linear-gradient(90deg, var(--line) 1px, transparent 1px);
  background-size: 56px 56px;
  -webkit-mask-image: radial-gradient(
    ellipse 80% 50% at 50% 0%,
    #000 40%,
    transparent 100%
  );
  mask-image: radial-gradient(
    ellipse 80% 50% at 50% 0%,
    #000 40%,
    transparent 100%
  );
}
.promo {
  background: linear-gradient(90deg, var(--brand-soft), var(--cyan-soft));
  border-bottom: 1px solid var(--line);
  font-size: 13px;
  color: var(--ink);
}
.promo .container {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  min-height: 38px;
  padding: 6px 24px;
  text-align: center;
}
.promo b {
  color: var(--brand-2);
}
.promo .dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--brand-2);
  box-shadow: 0 0 0 3px var(--brand-soft);
  flex-shrink: 0;
}
.nav {
  position: sticky;
  top: 0;
  z-index: 60;
  -webkit-backdrop-filter: blur(16px);
  backdrop-filter: blur(16px);
  background: var(--nav-bg);
  border-bottom: 1px solid transparent;
  transition:
    border-color var(--dur) var(--ease),
    background var(--dur) var(--ease);
}
.nav.scrolled {
  border-color: var(--line);
}
.nav .container {
  height: 64px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  font-weight: 800;
  font-size: 17px;
  color: var(--ink);
}
.brand .logo {
  width: 30px;
  height: 30px;
  flex-shrink: 0;
}
.nav-links {
  display: flex;
  align-items: center;
  gap: 28px;
}
.nav-links a {
  font-size: 14px;
  color: var(--ink-2);
  font-weight: 500;
  transition: color var(--dur-fast) var(--ease);
}
.nav-links a:hover {
  color: var(--ink);
}
.nav-cta {
  display: flex;
  align-items: center;
  gap: 10px;
}
.hamburger {
  display: none;
  width: 44px;
  height: 44px;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--line);
  border-radius: 12px;
  background: var(--card);
  color: var(--ink);
  cursor: pointer;
}
.hamburger svg {
  width: 22px;
  height: 22px;
}
.mobile-menu {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 12px 24px 18px;
  background: var(--nav-bg);
  -webkit-backdrop-filter: blur(16px);
  backdrop-filter: blur(16px);
  border-bottom: 1px solid var(--line);
}
.mobile-menu a {
  padding: 12px 8px;
  color: var(--ink-2);
  font-size: 15px;
  border-radius: 10px;
}
.mobile-menu a:hover {
  background: var(--surface-2);
  color: var(--ink);
}
.mm-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 8px 2px;
  margin-top: 4px;
  border-top: 1px solid var(--line);
}
.mm-row a {
  flex: 1;
  color: var(--brand-2);
  font-weight: 650;
  padding: 8px;
}
.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  font-family: inherit;
  font-size: 14px;
  font-weight: 650;
  border-radius: var(--radius-btn);
  padding: 11px 20px;
  cursor: pointer;
  border: 1px solid transparent;
  transition:
    transform var(--dur-fast) var(--ease),
    box-shadow var(--dur) var(--ease),
    background var(--dur) var(--ease),
    border-color var(--dur) var(--ease),
    color var(--dur) var(--ease);
  white-space: nowrap;
  min-height: 44px;
}
.btn svg {
  width: 17px;
  height: 17px;
  flex-shrink: 0;
}
.btn:active {
  transform: scale(0.97);
}
.site .btn-primary {
  background: var(--grad-brand);
  color: #1e1a15;
  box-shadow: 0 10px 24px -10px #c99f3780;
}
.btn-primary:hover {
  transform: translateY(-1px);
  box-shadow: 0 16px 32px -10px #c99f37a6;
}
.btn-ghost {
  background: var(--card);
  border-color: var(--line);
  color: var(--ink);
}
.btn-ghost:hover {
  border-color: var(--line-strong);
  color: var(--ink);
}
.btn-sm {
  padding: 8px 14px;
  font-size: 13px;
  min-height: 38px;
  border-radius: 10px;
}
.btn-lg {
  padding: 15px 28px;
  font-size: 15.5px;
  border-radius: 14px;
  min-height: 52px;
}
.btn-block {
  width: 100%;
}
.badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 11px;
  border-radius: var(--radius-pill);
  font-size: 12px;
  font-weight: 650;
  border: 1px solid var(--line);
}
.badge .d {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
}
.badge-brand {
  background: var(--brand-soft);
  color: var(--brand-2);
  border-color: var(--line-strong);
}
.badge-ok {
  background: var(--brand-soft);
  color: var(--brand-2);
  border-color: transparent;
}
.badge-cyan {
  background: var(--cyan-soft);
  color: var(--cyan);
  border-color: transparent;
}
.card {
  background: var(--card);
  border: 1px solid var(--line);
  border-radius: var(--radius-card);
  -webkit-backdrop-filter: blur(10px);
  backdrop-filter: blur(10px);
  box-shadow: var(--shadow);
}
.glass {
  background: var(--card);
  border: 1px solid var(--line);
  border-radius: var(--radius-card);
  -webkit-backdrop-filter: blur(14px);
  backdrop-filter: blur(14px);
}
.hero {
  padding: 72px 0 60px;
  text-align: center;
  position: relative;
  scroll-margin-top: 0;
}
.hero h1 {
  font-size: clamp(2.1rem, 6vw, 4.1rem);
  line-height: 1.12;
  margin: 20px auto;
  white-space: nowrap;
}
.hero .lede {
  color: var(--ink-2);
  font-size: clamp(1rem, 2.2vw, 1.22rem);
  max-width: 60ch;
  margin: 0 auto 30px;
}
.hero .lede b {
  color: var(--ink);
  font-weight: 700;
}
.hero-cta {
  display: flex;
  gap: 14px;
  justify-content: center;
  flex-wrap: wrap;
  margin-bottom: 18px;
}
.hero-cta.center {
  margin-bottom: 0;
}
.hero-cta-mobile {
  flex-direction: column;
  align-items: center;
  gap: 8px;
}
.hero-cta-mobile > .btn {
  width: 100%;
  max-width: 360px;
}
.hero-cta-mobile > .btn svg {
  width: 20px;
  height: 20px;
}
.hero-app-meta {
  font-size: 12.5px;
  color: var(--ink-3);
}
.hero-app-meta a {
  color: var(--brand-2);
  font-weight: 650;
}
.hero-links {
  display: flex;
  gap: 18px;
  justify-content: center;
  flex-wrap: wrap;
  margin-top: 4px;
  font-size: 13.5px;
  font-weight: 650;
}
.hero-links a,
.hero-links button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--brand-2);
  background: none;
  border: 0;
  padding: 6px 0;
  font: inherit;
  cursor: pointer;
}
.hero-links svg {
  width: 16px;
  height: 16px;
}
.hero-trust {
  display: flex;
  gap: 10px 20px;
  justify-content: center;
  flex-wrap: wrap;
  color: var(--ink-2);
  font-size: 13px;
  margin-bottom: 44px;
}
.hero-trust span {
  display: inline-flex;
  align-items: center;
  gap: 7px;
}
.hero-trust svg {
  width: 15px;
  height: 15px;
  color: var(--brand-2);
  flex-shrink: 0;
}
.stat-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 14px;
  max-width: 820px;
  margin: 0 auto;
}
.stat {
  padding: 18px 12px;
  text-align: center;
  border-radius: var(--radius-card);
  border: 1px solid var(--line);
  background: var(--card);
}
.stat .v {
  font-size: clamp(1.4rem, 3vw, 2rem);
  font-weight: 800;
}
.stat .v.live {
  color: var(--brand-2);
}
.stat .l {
  font-size: 12px;
  color: var(--ink-3);
  margin-top: 4px;
  letter-spacing: 0.03em;
}
.showcase {
  margin: 56px auto 0;
  max-width: 960px;
  position: relative;
}
.win {
  border-radius: 18px;
  overflow: hidden;
  border: 1px solid var(--line-strong);
  background: var(--card-solid);
  box-shadow: var(--shadow-lg);
}
.win-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  background: var(--surface-2);
  border-bottom: 1px solid var(--line);
}
.win-bar .wd {
  width: 11px;
  height: 11px;
  border-radius: 50%;
}
.win-bar .r {
  background: #ff5f57;
}
.win-bar .y {
  background: #febc2e;
}
.win-bar .g {
  background: #28c840;
}
.win-bar .wt {
  margin-left: 10px;
  font-size: 12px;
  color: var(--ink-3);
  font-family: ui-monospace, Menlo, Consolas, monospace;
}
.win-body {
  display: grid;
  grid-template-columns: 200px 1fr;
  min-height: 340px;
  text-align: left;
}
.win-side {
  background: var(--surface-1);
  border-right: 1px solid var(--line);
  padding: 16px 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.win-side .item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 11px;
  border-radius: 10px;
  font-size: 13px;
  color: var(--ink-2);
  font-weight: 500;
}
.win-side .item.on {
  background: var(--brand-soft);
  color: var(--brand-2);
  font-weight: 650;
}
.win-side .item .ic {
  width: 16px;
  height: 16px;
  flex-shrink: 0;
}
.win-main {
  padding: 20px;
  position: relative;
}
.mini-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.mini-title {
  font-size: 15px;
  font-weight: 700;
}
.route-card {
  border: 1px solid var(--line);
  border-radius: 14px;
  padding: 14px;
  margin-bottom: 12px;
  background: var(--surface-1);
}
.route-card.dim {
  opacity: 0.75;
}
.rc-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}
.rc-name {
  font-weight: 700;
  font-size: 14px;
}
.route-meta {
  display: flex;
  gap: 14px;
  font-size: 12px;
  color: var(--ink-2);
  flex-wrap: wrap;
}
.route-meta .k {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.route-meta .d {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}
.pulse {
  animation: acpulse-b4b35bfc 1.8s ease-in-out infinite;
}
@keyframes acpulse-b4b35bfc {
  0%,
  to {
    opacity: 1;
  }
  50% {
    opacity: 0.35;
  }
}
.switch-flow {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--ink-2);
  margin-top: 6px;
  flex-wrap: wrap;
}
.switch-flow.flat {
  margin-top: 0;
}
.switch-flow .sf-step {
  padding: 5px 10px;
  border-radius: 8px;
  background: var(--surface-2);
  border: 1px solid var(--line);
}
.switch-flow .sf-step.active {
  background: var(--brand-soft);
  color: var(--brand-2);
  border-color: transparent;
}
.switch-flow .arr {
  color: var(--ink-3);
}
.phone {
  position: absolute;
  right: -18px;
  bottom: -26px;
  width: 150px;
  height: 302px;
  border-radius: 26px;
  border: 6px solid var(--surface-3);
  background: var(--surface-1);
  box-shadow: var(--shadow-lg);
  overflow: hidden;
  display: none;
}
.phone .pbar {
  height: 26px;
  background: var(--surface-2);
  display: flex;
  align-items: center;
  justify-content: center;
}
.phone .pbar:after {
  content: "";
  width: 44px;
  height: 5px;
  border-radius: 3px;
  background: var(--surface-3);
}
.phone .pbody {
  padding: 12px;
}
.phone .chat {
  font-size: 9.5px;
  border-radius: 8px;
  padding: 7px 9px;
  margin-bottom: 7px;
  line-height: 1.5;
}
.phone .chat.ai {
  background: var(--brand-soft);
  color: var(--ink);
}
.phone .chat.me {
  background: var(--surface-2);
  color: var(--ink-2);
}
.phone .plabel {
  font-size: 9px;
  color: var(--cyan);
  font-weight: 700;
  margin-bottom: 8px;
  display: flex;
  align-items: center;
  gap: 5px;
}
.band {
  padding: 40px 0;
  border-top: 1px solid var(--line);
  border-bottom: 1px solid var(--line);
}
.band-title {
  text-align: center;
  color: var(--ink-3);
  font-size: 12.5px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  margin-bottom: 20px;
}
.logos {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 14px;
}
.logo-chip {
  display: inline-flex;
  align-items: center;
  gap: 9px;
  padding: 10px 18px;
  border-radius: 12px;
  border: 1px solid var(--line);
  background: var(--card);
  font-weight: 650;
  font-size: 14px;
  color: var(--ink);
}
.logo-chip .ic {
  width: 20px;
  height: 20px;
}
.band-sub {
  text-align: center;
  color: var(--ink-3);
  font-size: 13px;
  margin-top: 22px;
}
.band-sub b {
  color: var(--ink);
}
.feat-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 18px;
}
.feat {
  padding: 26px;
  border-radius: var(--radius-card);
  border: 1px solid var(--line);
  background: var(--card);
  position: relative;
  overflow: hidden;
  transition:
    transform var(--dur) var(--ease),
    border-color var(--dur) var(--ease),
    box-shadow var(--dur) var(--ease);
}
.feat:hover {
  transform: translateY(-3px);
  border-color: var(--line-strong);
  box-shadow: var(--glow);
}
.feat .ico {
  width: 48px;
  height: 48px;
  border-radius: 13px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 16px;
  background: var(--grad-brand);
  color: #1e1a15;
  box-shadow: 0 8px 20px -8px #c99f3780;
}
.feat .ico svg {
  width: 24px;
  height: 24px;
}
.feat h3 {
  font-size: 1.15rem;
  margin-bottom: 8px;
}
.feat p {
  color: var(--ink-2);
  font-size: 14px;
}
.feat .demo {
  margin-top: 16px;
  border-top: 1px dashed var(--line);
  padding-top: 14px;
}
.tool-mini {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.tool-mini .t {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 11px;
  border-radius: 9px;
  background: var(--surface-1);
  border: 1px solid var(--line);
  font-size: 12.5px;
  font-weight: 600;
}
.tool-mini .cy-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--cyan);
  display: inline-block;
}
.calc-mini {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 13px;
}
.calc-mini .chip {
  padding: 5px 10px;
  border-radius: 8px;
  background: var(--surface-1);
  border: 1px solid var(--line);
  font-weight: 650;
  font-variant-numeric: tabular-nums;
}
.calc-mini .op {
  color: var(--ink-3);
  font-weight: 700;
}
.calc-mini .res {
  padding: 5px 10px;
  border-radius: 8px;
  background: var(--brand-soft);
  color: var(--brand-2);
  font-weight: 750;
  font-variant-numeric: tabular-nums;
}
.billing-wrap {
  display: grid;
  grid-template-columns: 1.05fr 0.95fr;
  gap: 24px;
  align-items: stretch;
}
.billing-card {
  padding: 28px;
}
.billing-card .mb {
  margin-bottom: 6px;
}
.formula {
  font-size: clamp(1rem, 2.2vw, 1.35rem);
  font-weight: 750;
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin: 6px 0 20px;
}
.formula .box {
  padding: 8px 14px;
  border-radius: 12px;
  border: 1px solid var(--line);
  background: var(--surface-1);
}
.formula .box.a {
  color: var(--danger);
  border-color: var(--danger-soft);
}
.formula .box.b {
  color: var(--cyan);
}
.formula .box.c {
  color: var(--warn);
}
.formula .box.r {
  color: #1e1a15;
  background: var(--grad-brand);
  border-color: transparent;
}
.formula .fop {
  color: var(--ink-3);
}
.bill-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.bill-list li {
  display: flex;
  gap: 11px;
  align-items: flex-start;
  font-size: 14px;
  color: var(--ink-2);
}
.bill-list li svg {
  width: 20px;
  height: 20px;
  color: var(--brand-2);
  flex-shrink: 0;
  margin-top: 1px;
}
.bill-list li b {
  color: var(--ink);
}
.compare {
  padding: 24px;
}
.compare h4 {
  font-size: 14px;
  color: var(--ink-2);
  font-weight: 600;
  margin-bottom: 18px;
  text-align: center;
}
.bar {
  margin-bottom: 18px;
}
.bar .lab {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  margin-bottom: 7px;
}
.bar .lab .amt {
  font-weight: 750;
  font-variant-numeric: tabular-nums;
}
.bar .ours-name {
  color: var(--brand-2);
  font-weight: 700;
}
.bar .ours-amt {
  color: var(--brand-2);
}
.bar .track {
  height: 16px;
  border-radius: 999px;
  background: var(--surface-2);
  overflow: hidden;
}
.bar .fill {
  height: 100%;
  border-radius: 999px;
}
.bar.official .fill {
  width: 100%;
  background: linear-gradient(90deg, #a99d8a, #d3c9b8);
}
.bar.ours .fill {
  width: 15%;
  background: var(--grad-brand);
  box-shadow: 0 0 14px #c99f3780;
}
.save-tag {
  text-align: center;
  margin-top: 8px;
  font-size: 13px;
  color: var(--ink-2);
}
.save-tag b {
  color: var(--brand-2);
  font-size: 1.3rem;
  font-variant-numeric: tabular-nums;
}
.disclaimer {
  font-size: 12px;
  color: var(--ink-3);
  margin-top: 14px;
  text-align: center;
}
.steps {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
}
.step-c {
  padding: 22px;
  border-radius: var(--radius-card);
  border: 1px solid var(--line);
  background: var(--card);
  position: relative;
}
.step-c .n {
  font-size: 2rem;
  font-weight: 800;
  color: var(--line-strong);
  line-height: 1;
}
.step-c h4 {
  margin: 12px 0 8px;
  font-size: 1.02rem;
}
.step-c p {
  color: var(--ink-2);
  font-size: 13.5px;
}
.step-c .arrow {
  position: absolute;
  right: -11px;
  top: 34px;
  width: 22px;
  height: 22px;
  color: var(--ink-3);
  z-index: 2;
}
.price-top {
  display: grid;
  grid-template-columns: 1fr;
  max-width: 460px;
  margin: 0 auto 30px;
}
.price-card {
  padding: 30px;
  border-radius: 20px;
  border: 1.5px solid var(--line-strong);
  background: var(--card);
  position: relative;
  box-shadow: var(--glow);
  overflow: hidden;
}
.price-card:before {
  content: "";
  position: absolute;
  inset: 0;
  background: radial-gradient(
    circle at 50% -20%,
    var(--brand-soft),
    transparent 60%
  );
  pointer-events: none;
}
.price-head {
  text-align: center;
  margin-bottom: 22px;
  position: relative;
}
.price-amt {
  display: flex;
  align-items: baseline;
  justify-content: center;
  gap: 4px;
  margin: 12px 0 6px;
}
.price-amt .big {
  font-size: 3rem;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
}
.price-amt .unit {
  color: var(--ink-2);
  font-size: 14px;
}
.price-sub {
  color: var(--ink-2);
  font-size: 13.5px;
}
.price-feats {
  list-style: none;
  padding: 0;
  margin: 0 0 24px;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 11px;
  position: relative;
}
.price-feats li {
  display: flex;
  gap: 8px;
  align-items: center;
  font-size: 13.5px;
  color: var(--ink-2);
}
.price-feats li svg {
  width: 16px;
  height: 16px;
  color: var(--brand-2);
  flex-shrink: 0;
}
.price-table-wrap {
  max-width: 920px;
  margin: 0 auto;
}
.pt-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 14px;
}
.pt-head h3 {
  font-size: 1.15rem;
}
.pt-src {
  font-size: 13px;
  color: var(--ink-2);
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.pt-src svg {
  width: 14px;
  height: 14px;
}
.pt-src:hover {
  color: var(--ink);
}
.pt-desc {
  color: var(--ink-2);
  font-size: 13.5px;
  margin-bottom: 16px;
}
.filters {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.filters button {
  padding: 7px 15px;
  border-radius: 999px;
  font-size: 12.5px;
  font-weight: 600;
  border: 1px solid var(--line);
  background: var(--card);
  color: var(--ink-2);
  cursor: pointer;
  transition: all var(--dur-fast) var(--ease);
  font-family: inherit;
}
.filters button:hover {
  color: var(--ink);
  border-color: var(--line-strong);
}
.filters button.on {
  background: var(--grad-brand);
  color: #1e1a15;
  border-color: transparent;
}
.table-wrap {
  border: 1px solid var(--line);
  border-radius: var(--radius-card);
  overflow: hidden;
  overflow-x: auto;
}
table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13.5px;
}
thead tr {
  background: var(--surface-2);
}
th {
  text-align: left;
  padding: 13px 16px;
  font-weight: 600;
  color: var(--ink-2);
  font-size: 12.5px;
  white-space: nowrap;
}
th.r,
td.r {
  text-align: right;
}
td {
  padding: 13px 16px;
  border-top: 1px solid var(--line);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
tbody tr:hover {
  background: var(--brand-soft);
}
td .mn {
  font-weight: 650;
  color: var(--ink);
}
td .mn.link {
  color: var(--brand-2);
}
td .mn.link:hover {
  text-decoration: underline;
}
td .pv,
.pc .pv {
  color: var(--ink-3);
  font-size: 12.5px;
}
td.out {
  font-weight: 650;
  color: var(--ink);
}
td.note {
  color: var(--ink-3);
  font-size: 12px;
  white-space: normal;
  max-width: 260px;
}
.mono {
  font-family: ui-monospace, Menlo, Consolas, monospace;
}
.price-cards {
  display: none;
  flex-direction: column;
  gap: 10px;
}
.pc {
  border: 1px solid var(--line);
  border-radius: 14px;
  padding: 14px;
  background: var(--card);
}
.pc-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}
.pc-top .mn {
  font-weight: 650;
  font-size: 14px;
}
.pc-top .mn.link {
  color: var(--brand-2);
}
.pc-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px 16px;
  font-size: 12.5px;
}
.pc-grid > div {
  display: flex;
  justify-content: space-between;
}
.pc-grid .mono {
  font-variant-numeric: tabular-nums;
}
.pc-grid .out {
  font-weight: 650;
  color: var(--ink);
}
.pc-note {
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px solid var(--line);
  font-size: 12px;
  color: var(--ink-3);
}
.pt-expand {
  display: flex;
  justify-content: center;
  margin-top: 16px;
}
.chev {
  width: 16px;
  height: 16px;
  transition: transform var(--dur) var(--ease);
}
.chev.up {
  transform: rotate(180deg);
}
.pt-foot {
  color: var(--ink-3);
  font-size: 12px;
  margin-top: 12px;
  text-align: right;
}
.pt-foot a {
  color: var(--ink-2);
}
.pt-foot a:hover {
  color: var(--ink);
}
.dl-hero {
  text-align: center;
  margin-bottom: 34px;
}
.dl-file {
  font-size: 12.5px;
  color: var(--ink-3);
  margin-top: 10px;
}
.oneclick {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 10px 16px;
  border-radius: 12px;
  background: var(--cyan-soft);
  color: var(--cyan);
  font-size: 13px;
  font-weight: 650;
  border: 1px solid transparent;
  margin-top: 14px;
}
.oneclick svg {
  width: 15px;
  height: 15px;
}
.dl-tut {
  margin-top: 14px;
  font-size: 13.5px;
  color: var(--ink-2);
}
.dl-tut a {
  color: var(--brand-2);
}
.dl-tut a:hover {
  text-decoration: underline;
}
.dl-area {
  max-width: 900px;
  margin: 0 auto;
}
.dl-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
  margin-bottom: 20px;
  align-items: start;
}
.dl-grid.one {
  grid-template-columns: 1fr;
  max-width: 420px;
  margin-left: auto;
  margin-right: auto;
}
.dl-grid.two {
  grid-template-columns: repeat(2, 1fr);
  max-width: 640px;
  margin-left: auto;
  margin-right: auto;
}
.dl-card {
  border-radius: var(--radius-card);
  border: 1px solid var(--line);
  background: var(--card);
  overflow: hidden;
  transition: border-color var(--dur) var(--ease);
}
.dl-card:hover {
  border-color: var(--line-strong);
}
.dl-card-head {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 18px 18px 14px;
  border-bottom: 1px solid var(--line);
}
.dl-card-head .os {
  width: 40px;
  height: 40px;
  border-radius: 11px;
  background: var(--surface-2);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.dl-card-head .os svg {
  width: 22px;
  height: 22px;
  color: var(--ink);
}
.os-name {
  font-weight: 700;
  font-size: 15px;
}
.os-arch {
  font-size: 12px;
  color: var(--ink-3);
  margin-top: 2px;
}
.dl-card-body {
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.dl-pickers {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 4px 4px 8px;
}
.dl-picker {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}
.dl-picker button {
  flex: 1;
  min-width: 0;
  padding: 6px 8px;
  border-radius: 8px;
  font-size: 12px;
  font-weight: 600;
  border: 1px solid var(--line);
  background: var(--card);
  color: var(--ink-2);
  cursor: pointer;
  font-family: inherit;
  min-height: 36px;
  white-space: nowrap;
  transition:
    color var(--dur-fast) var(--ease),
    border-color var(--dur-fast) var(--ease),
    background var(--dur-fast) var(--ease);
}
.dl-picker button:hover {
  color: var(--ink);
  border-color: var(--line-strong);
}
.dl-picker button.on {
  background: var(--grad-brand);
  color: #1e1a15;
  border-color: transparent;
}
.dl-empty-combo {
  font-size: 12px;
  color: var(--ink-3);
  padding: 10px 12px;
  margin: 0;
}
.dl-variant {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 11px 12px;
  border-radius: 10px;
  transition: background var(--dur-fast) var(--ease);
}
.dl-variant:hover {
  background: var(--surface-2);
}
.dv-meta {
  flex: 1;
  min-width: 0;
}
.dv-row1 {
  display: flex;
  align-items: center;
  gap: 8px;
}
.dv-label {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--ink);
}
.dv-size {
  font-size: 10.5px;
  padding: 1px 7px;
  border-radius: 6px;
  background: var(--surface-1);
  color: var(--ink-3);
  border: 1px solid var(--line);
}
.dv-desc {
  font-size: 12px;
  color: var(--ink-3);
  margin-top: 3px;
}
.dv-arrow {
  width: 16px;
  height: 16px;
  color: var(--ink-3);
  flex-shrink: 0;
}
.dl-variant:hover .dv-arrow {
  color: var(--brand-2);
}
.dl-script {
  margin: 6px 6px 2px;
  padding-top: 10px;
  border-top: 1px solid var(--line);
}
.ds-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  padding: 0 2px;
}
.ds-head span {
  font-size: 12px;
  color: var(--ink-3);
}
.ds-head button {
  font-size: 11px;
  padding: 2px 10px;
  border-radius: 7px;
  border: 1px solid var(--line);
  background: var(--card);
  color: var(--ink-2);
  cursor: pointer;
  font-family: inherit;
  transition:
    color var(--dur-fast) var(--ease),
    border-color var(--dur-fast) var(--ease);
}
.ds-head button:hover {
  color: var(--ink);
  border-color: var(--line-strong);
}
.dl-script pre {
  margin: 0;
  font-size: 11px;
  line-height: 1.6;
  color: var(--cyan);
  background: var(--surface-1);
  border: 1px solid var(--line);
  border-radius: 10px;
  padding: 10px 12px;
  overflow-x: auto;
  font-family: ui-monospace, Menlo, Consolas, monospace;
}
.dl-extra {
  display: flex;
  gap: 12px;
  justify-content: center;
  flex-wrap: wrap;
  align-items: center;
  margin: 8px 0 22px;
  font-size: 13.5px;
}
.dl-extra a {
  color: var(--ink-2);
}
.dl-extra a:hover {
  color: var(--brand-2);
}
.dl-extra .sep {
  color: var(--ink-3);
}
.notes-card {
  max-width: 720px;
  margin: 0 auto;
  padding: 22px;
  border-radius: var(--radius-card);
  border: 1px solid var(--line);
  background: var(--card);
}
.notes-card h3 {
  font-size: 14px;
  font-weight: 600;
  color: var(--ink-2);
  margin-bottom: 12px;
  display: flex;
  align-items: center;
  gap: 8px;
}
.notes-card h3 svg {
  width: 16px;
  height: 16px;
}
.notes-body {
  font-size: 14px;
  color: var(--ink-2);
  line-height: 1.7;
}
.notes-more {
  margin-top: 12px;
  font-size: 12.5px;
  color: var(--ink-3);
  background: none;
  border: none;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-family: inherit;
}
.notes-more:hover {
  color: var(--ink);
}
.dl-empty {
  text-align: center;
  color: var(--ink-3);
  font-size: 14px;
  padding: 40px 0;
}
.history {
  max-width: 720px;
  margin: 32px auto 0;
  scroll-margin-top: 90px;
}
.hist-title {
  text-align: center;
  font-size: 14px;
  color: var(--ink-2);
  font-weight: 600;
  margin-bottom: 16px;
}
.hist-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.hist-item {
  border: 1px solid var(--line);
  border-radius: 14px;
  overflow: hidden;
  background: var(--card);
  transition: border-color var(--dur) var(--ease);
}
.hist-item.open {
  border-color: var(--line-strong);
}
.hist-q {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  background: none;
  border: none;
  cursor: pointer;
  text-align: left;
  font-family: inherit;
  color: var(--ink);
}
.hist-ver {
  font-size: 13.5px;
  font-weight: 650;
}
.hist-date {
  font-size: 12px;
  color: var(--ink-3);
  flex: 1;
}
.hist-body {
  padding: 0 16px 16px;
  font-size: 13.5px;
  color: var(--ink-2);
}
.hist-assets {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 12px;
}
.hist-asset {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: 9px;
  border: 1px solid var(--line);
  font-size: 12px;
  color: var(--ink-2);
  transition:
    color var(--dur-fast) var(--ease),
    border-color var(--dur-fast) var(--ease);
}
.hist-asset:hover {
  color: var(--ink);
  border-color: var(--line-strong);
}
.hist-asset svg {
  width: 14px;
  height: 14px;
}
.hist-asset .cap {
  text-transform: capitalize;
}
.hist-asset .dim {
  opacity: 0.6;
}
.pager {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  margin-top: 18px;
  font-size: 12.5px;
  color: var(--ink-3);
}
.pager button {
  padding: 6px 14px;
  border-radius: 9px;
  border: 1px solid var(--line);
  background: var(--card);
  color: var(--ink-2);
  cursor: pointer;
  font-family: inherit;
  transition:
    color var(--dur-fast) var(--ease),
    border-color var(--dur-fast) var(--ease);
}
.pager button:hover:not(:disabled) {
  color: var(--ink);
  border-color: var(--line-strong);
}
.pager button:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}
.app-box {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 20px;
  align-items: center;
  max-width: 640px;
  margin: 34px auto 0;
  padding: 22px;
}
.app-visual {
  display: block;
  flex-shrink: 0;
}
.app-qr {
  width: 112px;
  height: 112px;
  border-radius: 14px;
  background: #fff;
  border: 1px solid var(--line);
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  box-shadow: var(--shadow-sm);
}
.app-qr img {
  width: 100%;
  height: 100%;
  display: block;
}
.app-qr-ph {
  font-size: 11px;
  color: #817665;
  text-align: center;
  line-height: 1.5;
}
.app-info h4 {
  font-size: 1.05rem;
  margin-bottom: 6px;
}
.app-info p {
  color: var(--ink-2);
  font-size: 14px;
  margin-bottom: 12px;
}
.app-btns {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  align-items: center;
}
.app-scan {
  font-size: 12.5px;
  color: var(--ink-3);
}
.dl-seg {
  display: flex;
  max-width: 420px;
  margin: 0 auto 18px;
  padding: 4px;
  border-radius: 12px;
  background: var(--surface-2);
  border: 1px solid var(--line);
}
.dl-seg button {
  flex: 1;
  min-height: 40px;
  border: 0;
  border-radius: 9px;
  background: none;
  color: var(--ink-2);
  font: inherit;
  font-size: 14px;
  font-weight: 650;
  cursor: pointer;
}
.dl-seg button.on {
  background: var(--card);
  color: var(--ink);
  box-shadow: var(--shadow-sm);
}
.m-dl {
  max-width: 420px;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.m-app-card {
  padding: 20px;
}
.m-app-head {
  display: flex;
  align-items: center;
  gap: 12px;
}
.m-app-ico {
  width: 48px;
  height: 48px;
  flex: none;
  border-radius: 13px;
  background: var(--grad-brand);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 10px 22px -10px #c99f3780;
}
.m-app-ico svg {
  width: 26px;
  height: 26px;
  color: #1e1a15;
}
.m-app-title {
  font-weight: 800;
  font-size: 16px;
}
.m-app-ver {
  font-size: 12px;
  color: var(--ink-3);
}
.m-app-desc {
  color: var(--ink-2);
  font-size: 13.5px;
  margin: 12px 0 16px;
}
.m-app-foot {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
  margin-top: 12px;
  font-size: 12.5px;
  color: var(--ink-3);
}
.m-app-foot a {
  color: var(--brand-2);
  font-weight: 650;
}
.m-desk-card {
  padding: 18px 20px;
  border-radius: var(--radius-card);
  border: 1px dashed var(--line-strong);
  background: var(--surface-2);
}
.m-desk-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 750;
  font-size: 15px;
}
.m-desk-title svg {
  width: 18px;
  height: 18px;
  color: var(--brand-2);
}
.m-desk-card p {
  color: var(--ink-2);
  font-size: 13px;
  margin: 8px 0 14px;
}
.m-desk-btns {
  display: flex;
  gap: 10px;
}
.m-desk-btns .btn {
  flex: 1;
}
.toast {
  position: fixed;
  left: 50%;
  bottom: calc(96px + env(safe-area-inset-bottom));
  transform: translate(-50%);
  z-index: 90;
  max-width: calc(100vw - 40px);
  padding: 10px 16px;
  border-radius: 12px;
  background: #191510f2;
  color: #f6f0e2;
  font-size: 13px;
  text-align: center;
}
.docs-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
}
.doc-card {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  padding: 22px;
  border-radius: var(--radius-card);
  border: 1px solid var(--line);
  background: var(--card);
  transition:
    transform var(--dur) var(--ease),
    border-color var(--dur) var(--ease),
    box-shadow var(--dur) var(--ease);
}
.doc-card:hover {
  transform: translateY(-3px);
  border-color: var(--line-strong);
  box-shadow: var(--glow);
}
.doc-card .dc-ico {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 14px;
  background: var(--grad-brand);
  color: #1e1a15;
  box-shadow: 0 8px 20px -8px #c99f3780;
}
.doc-card .dc-ico svg {
  width: 22px;
  height: 22px;
}
.doc-card h3 {
  font-size: 1.02rem;
  margin-bottom: 6px;
  display: flex;
  align-items: center;
  gap: 8px;
}
.doc-card p {
  color: var(--ink-2);
  font-size: 13.5px;
  flex: 1;
}
.doc-card .dc-link {
  margin-top: 14px;
  font-size: 13px;
  font-weight: 650;
  color: var(--brand-2);
}
.doc-card:hover .dc-link {
  text-decoration: underline;
}
.dc-new {
  font-size: 11px;
  font-weight: 650;
  padding: 1px 8px;
  border-radius: var(--radius-pill);
  background: var(--cyan-soft);
  color: var(--cyan);
}
.docs-cta {
  display: flex;
  justify-content: center;
  margin-top: 28px;
}
.promise-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 36px;
}
.promise {
  padding: 22px;
  border-radius: var(--radius-card);
  border: 1px solid var(--line);
  background: var(--card);
  text-align: center;
}
.promise .pic {
  width: 44px;
  height: 44px;
  margin: 0 auto 12px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--brand-soft);
  color: var(--brand-2);
}
.promise .pic svg {
  width: 22px;
  height: 22px;
}
.promise h4 {
  font-size: 1rem;
  margin-bottom: 6px;
}
.promise p {
  font-size: 13px;
  color: var(--ink-2);
}
.reviews {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}
.review {
  padding: 22px;
  border-radius: var(--radius-card);
  border: 1px solid var(--line);
  background: var(--card);
}
.review .stars {
  color: var(--warn);
  font-size: 14px;
  letter-spacing: 2px;
  margin-bottom: 10px;
}
.review p {
  font-size: 14px;
  color: var(--ink);
  margin-bottom: 14px;
  line-height: 1.6;
}
.review .who {
  display: flex;
  align-items: center;
  gap: 10px;
}
.review .av {
  width: 34px;
  height: 34px;
  border-radius: 50%;
  background: var(--grad-brand);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #1e1a15;
  font-weight: 700;
  font-size: 14px;
}
.review .nm {
  font-size: 13px;
  font-weight: 650;
}
.review .rl {
  font-size: 12px;
  color: var(--ink-3);
}
.ph-note {
  text-align: center;
  font-size: 12px;
  color: var(--ink-3);
  margin-top: 16px;
}
.faq {
  max-width: 760px;
  margin: 0 auto;
}
.faq-item {
  border: 1px solid var(--line);
  border-radius: 14px;
  background: var(--card);
  margin-bottom: 12px;
  overflow: hidden;
}
.faq-q {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 18px 20px;
  background: none;
  border: none;
  color: var(--ink);
  font-size: 15px;
  font-weight: 600;
  cursor: pointer;
  text-align: left;
  font-family: inherit;
}
.faq-q svg {
  width: 18px;
  height: 18px;
  color: var(--ink-3);
  flex-shrink: 0;
  transition: transform var(--dur) var(--ease);
}
.faq-item.open .faq-q svg {
  transform: rotate(180deg);
}
.faq-a {
  max-height: 0;
  overflow: hidden;
  transition: max-height var(--dur-slow) var(--ease);
  color: var(--ink-2);
  font-size: 14px;
}
.faq-item.open .faq-a {
  max-height: 260px;
}
.faq-a div {
  padding: 0 20px 18px;
}
.final {
  text-align: center;
  padding: 64px 32px;
  border-radius: 24px;
  position: relative;
  overflow: hidden;
  border: 1px solid var(--line-strong);
  background: linear-gradient(135deg, #e2c27324, #c99f371a 50%, #f0dfb214);
}
.final h2 {
  font-size: clamp(1.7rem, 3.6vw, 2.5rem);
  margin-bottom: 14px;
}
.final p {
  color: var(--ink-2);
  margin-bottom: 28px;
  font-size: 1.05rem;
}
footer {
  border-top: 1px solid var(--line);
  padding: 44px 0 40px;
  color: var(--ink-2);
}
.foot-grid {
  display: flex;
  justify-content: space-between;
  gap: 30px;
  flex-wrap: wrap;
  align-items: flex-start;
}
.foot-brand {
  max-width: 280px;
}
.foot-brand .brand {
  margin-bottom: 12px;
}
.foot-brand p {
  font-size: 13.5px;
  color: var(--ink-2);
}
.foot-links {
  display: flex;
  gap: 40px;
  flex-wrap: wrap;
}
.foot-col h5 {
  font-size: 12.5px;
  color: var(--ink-3);
  text-transform: uppercase;
  letter-spacing: 0.08em;
  margin: 0 0 12px;
  font-weight: 700;
}
.foot-col a {
  display: block;
  font-size: 14px;
  color: var(--ink-2);
  margin-bottom: 9px;
  transition: color var(--dur-fast) var(--ease);
}
.foot-col a:hover {
  color: var(--ink);
}
.foot-bottom {
  margin-top: 34px;
  padding-top: 22px;
  border-top: 1px solid var(--line);
  display: flex;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  font-size: 13px;
  color: var(--ink-3);
}
.mobile-cta {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  z-index: 70;
  display: none;
  gap: 10px;
  padding: 12px 16px;
  padding-bottom: calc(12px + env(safe-area-inset-bottom));
  background: var(--nav-bg);
  -webkit-backdrop-filter: blur(16px);
  backdrop-filter: blur(16px);
  border-top: 1px solid var(--line);
}
.mobile-cta .mc-1 {
  flex: 1;
}
.mobile-cta .mc-2 {
  flex: 2;
}
@media (max-width: 900px) {
  .nav-links,
  .desktop-only {
    display: none !important;
  }
  .hamburger {
    display: flex;
  }
  .win-body {
    grid-template-columns: 1fr;
  }
  .win-side {
    flex-direction: row;
    overflow-x: auto;
    border-right: none;
    border-bottom: 1px solid var(--line);
  }
  .win-side .item {
    white-space: nowrap;
  }
  .billing-wrap {
    grid-template-columns: 1fr;
  }
  .steps {
    grid-template-columns: repeat(2, 1fr);
  }
  .step-c .arrow {
    display: none;
  }
  .promise-grid {
    grid-template-columns: repeat(2, 1fr);
  }
  .reviews {
    grid-template-columns: 1fr;
  }
  .dl-grid,
  .dl-grid.two {
    grid-template-columns: 1fr;
    max-width: 420px;
    margin-left: auto;
    margin-right: auto;
  }
  .price-feats {
    grid-template-columns: 1fr;
  }
  .docs-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}
@media (max-width: 768px) {
  .table-wrap {
    display: none;
  }
  .price-cards {
    display: flex;
  }
}
@media (max-width: 640px) {
  .container {
    padding: 0 18px;
  }
  section {
    padding: 60px 0;
  }
  .feat-grid {
    grid-template-columns: 1fr;
  }
  .stat-row {
    grid-template-columns: repeat(2, 1fr);
  }
  .docs-grid {
    grid-template-columns: 1fr;
  }
  .logos {
    gap: 10px;
  }
  .app-box {
    grid-template-columns: 1fr;
    text-align: center;
    justify-items: center;
  }
  .foot-bottom {
    flex-direction: column;
  }
  .mobile-cta {
    display: flex;
  }
  .site {
    padding-bottom: 76px;
  }
}
@media (min-width: 1024px) {
  .phone {
    display: block;
  }
}
@media (prefers-reduced-motion: reduce) {
  .pulse {
    animation: none;
  }
}
.tt {
  position: relative;
  display: inline-flex;
}
.tt-btn {
  width: 40px;
  height: 40px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-btn);
  border: 1px solid var(--line);
  background: var(--card);
  color: var(--ink);
  cursor: pointer;
  transition:
    border-color var(--dur-fast) var(--ease),
    color var(--dur-fast) var(--ease),
    background var(--dur-fast) var(--ease);
}
.tt-btn:hover {
  border-color: var(--line-strong);
  color: var(--brand-2);
}
.tt-btn svg {
  width: 18px;
  height: 18px;
}
.tt-menu {
  position: absolute;
  right: 0;
  top: calc(100% + 8px);
  z-index: 80;
  min-width: 168px;
  padding: 6px;
  border-radius: var(--radius-card);
  border: 1px solid var(--line);
  background: var(--card-solid);
  box-shadow: var(--shadow-lg);
}
.tt-item {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 10px;
  border: none;
  border-radius: var(--radius-input);
  background: none;
  color: var(--ink-2);
  font-size: 13.5px;
  font-weight: 550;
  cursor: pointer;
  text-align: left;
  font-family: inherit;
  transition:
    background var(--dur-fast) var(--ease),
    color var(--dur-fast) var(--ease);
}
.tt-item:hover {
  background: var(--surface-2);
  color: var(--ink);
}
.tt-item.on {
  color: var(--brand-2);
  background: var(--brand-soft);
}
.tt-item .ic {
  width: 17px;
  height: 17px;
  flex-shrink: 0;
}
.tt-item .lb {
  flex: 1;
}
.tt-item .hint {
  font-size: 11px;
  color: var(--ink-3);
  font-variant-numeric: tabular-nums;
}
.tt-item .ck {
  width: 15px;
  height: 15px;
  color: var(--brand-2);
  flex-shrink: 0;
}
.tt-fade-enter-active,
.tt-fade-leave-active {
  transition:
    opacity var(--dur-fast) var(--ease),
    transform var(--dur-fast) var(--ease);
}
.tt-fade-enter-from,
.tt-fade-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
.site {
  isolation: isolate;
  background: var(--bg);
  color: var(--ink);
  font-family:
    -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC",
    "Microsoft YaHei", "Helvetica Neue", sans-serif;
  -webkit-font-smoothing: antialiased;
  --default-font-family: inherit;
}
.site *,
.site *::before,
.site *::after {
  box-sizing: border-box;
}
.site .container {
  width: 100%;
  max-width: 1120px;
}
.site svg {
  display: block;
  vertical-align: middle;
}
.site button {
  font-family: inherit;
}
.site .logo {
  object-fit: contain;
}
.site .hero .lede {
  white-space: pre-line;
}
.site .bg-decor,
.site .bg-grid {
  z-index: -1;
}
.site .hero h1 {
  white-space: normal;
}
.site .tabular-nums {
  font-variant-numeric: tabular-nums;
}
.site .ac-grad-text {
  background: var(--grad-brand-cyan);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
}
.site .ac-hot-text {
  color: var(--ink);
}
.site .my-1 {
  margin: 4px 0;
}
.site .notes-body ul {
  padding-left: 20px;
  list-style: disc;
}
.site a:focus-visible,
.site button:focus-visible {
  outline: 2px solid var(--brand-2);
  outline-offset: 4px;
}
.site .policy-backdrop {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: grid;
  place-items: center;
  padding: 24px;
  background: #0e0c09aa;
  backdrop-filter: blur(8px);
}
.site .policy-dialog {
  width: 100%;
  max-width: 520px;
  padding: 30px;
  border: 1px solid var(--line);
  border-radius: 20px;
  background: var(--card-solid);
  box-shadow: var(--shadow-lg);
}
.site .policy-dialog h2 {
  font-size: 22px;
}
.site .policy-dialog p {
  margin: 18px 0;
  color: var(--ink-2);
}
.site .policy-dialog > .btn {
  display: flex;
  margin-top: 24px;
}
.site .faq-item.open .faq-a {
  max-height: 500px;
}
.site [aria-hidden="true"].faq-a {
  visibility: hidden;
}
.site .app-qr img {
  width: 112px;
  height: 112px;
}
@media (max-width: 640px) {
  .site .hero h1 {
    font-size: clamp(1.9rem, 6vw, 4.1rem);
  }
  .site .hero .lede {
    font-size: 1rem;
  }
  .site .nav .container {
    gap: 10px;
  }
  .site .brand {
    font-size: 15px;
  }
  .site .nav-cta {
    gap: 8px;
  }
}
@media (prefers-reduced-motion: reduce) {
  .site * {
    scroll-behavior: auto !important;
    transition: none !important;
    animation: none !important;
  }
}
</style>
