<script setup lang="ts">
// Ported from Sub2API HomeView; identity and branding remain tenant-local.
import { computed, ref } from 'vue'
import { useAgentSession } from '@/agent/session'
import { compactHomeEnabled, homeContent, siteDocURL, siteName, siteLogo, siteSubtitle } from '@/agent/branding'
import { isDark, toggleTheme } from '@/agent/theme'
import Icon from '@/components/icons/Icon.vue'
import LzParticleScene from '@/components/home/LzParticleScene.vue'
const session = useAgentSession()
const isAuthenticated = computed(() => session.isAuthenticated)
const dashboardPath = computed(() => session.isAgentAdmin ? '/admin/dashboard' : '/dashboard')
const customHomeURL = computed(() => {
  try {
    const url = new URL(homeContent.value)
    return ['https:', 'http:'].includes(url.protocol) && !url.username && !url.password ? url.href : ''
  } catch { return '' }
})
const customHomeHTML = computed(() => homeContent.value.startsWith('<') ? homeContent.value : '')
const hasHomeContent = computed(() => Boolean(customHomeURL.value || customHomeHTML.value))
const labels: Record<string, string> = {
 'home.docs': '文档', 'nav.modelPlaza': '模型广场',
 'home.viewDocs': '查看文档', 'downloads.nav': '客户端下载', 'home.goToDashboard': '进入控制台',
 'home.switchToLight': '切换浅色模式', 'home.switchToDark': '切换深色模式',
 'home.dashboard': '控制台', 'home.login': '登录',
 'home.heroDescription': '统一接入 AI 模型，使用一个 API 开启你的创作与开发。'
}
const t = (key: string) => labels[key] || key
type LzMode = 'AIFX' | 'CC2CX' | 'OPENAI' | 'DEEPSEEK'
const lzModes: LzMode[] = ['AIFX', 'CC2CX', 'OPENAI', 'DEEPSEEK']
const lzActiveMode = ref<LzMode>('AIFX')
function cycleLzMode() { lzActiveMode.value = lzModes[(lzModes.indexOf(lzActiveMode.value) + 1) % lzModes.length] }
const currentYear = new Date().getFullYear()
</script>

<template>
  <!-- Same precedence as Sub2API: custom content, compact home, particle home. -->
  <div v-if="hasHomeContent" class="min-h-screen" data-testid="custom-home">
    <iframe v-if="customHomeURL" :src="customHomeURL" :title="`${siteName} 首页`" class="h-screen w-full border-0" sandbox="allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-top-navigation-by-user-activation" referrerpolicy="no-referrer" allowfullscreen></iframe>
    <iframe v-else :srcdoc="customHomeHTML" :title="`${siteName} 首页`" class="h-screen w-full border-0" sandbox="allow-popups allow-popups-to-escape-sandbox allow-top-navigation-by-user-activation" referrerpolicy="no-referrer"></iframe>
  </div>

  <!-- Compact Home Page: copied from Sub2API HomeView. -->

  <div
    v-else-if="compactHomeEnabled"
    data-testid="compact-home"
    class="flex min-h-screen flex-col bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white"
  >
    <header class="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-dark-800">
      <nav class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 sm:gap-4">
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <img
            :src="siteLogo || '/logo.svg'"
            alt="Logo"
            class="h-9 w-9 shrink-0 rounded-lg object-contain"
          />
          <span class="min-w-0 truncate text-base font-semibold">{{ siteName }}</span>
        </div>
        <div class="flex max-w-full shrink-0 flex-wrap items-center justify-end gap-2">
          <a
            v-if="siteDocURL"
            :href="siteDocURL"
            target="_blank"
            rel="noopener noreferrer"
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <router-link
            to="/model-plaza"
            class="flex h-10 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('nav.modelPlaza')"
          >
            <Icon name="grid" size="md" />
            <span class="hidden sm:inline">{{ t('nav.modelPlaza') }}</span>
          </router-link>
          <button
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex min-h-10 shrink-0 items-center justify-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
          >
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
          <router-link
            to="/downloads"
            class="flex h-10 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
          >
            <Icon name="download" size="md" />
            <span class="hidden sm:inline">{{ t('downloads.nav') }}</span>
          </router-link>
        </div>
      </nav>
    </header>

    <main class="flex min-w-0 flex-1 items-center justify-center px-4 py-16 sm:px-6">
      <div class="min-w-0 max-w-2xl text-center">
        <img
          :src="siteLogo || '/logo.svg'"
          alt="Logo"
          class="mx-auto mb-6 h-20 w-20 rounded-2xl object-contain"
        />
        <h1 class="[overflow-wrap:anywhere] text-3xl font-bold md:text-4xl">{{ siteName }}</h1>
        <p class="mt-4 whitespace-pre-wrap [overflow-wrap:anywhere] text-base text-gray-600 dark:text-dark-300">{{ siteSubtitle }}</p>
        <router-link
          :to="isAuthenticated ? dashboardPath : '/login'"
          class="mt-8 inline-flex min-h-10 items-center justify-center rounded-lg bg-primary-600 px-5 py-2.5 text-sm font-medium text-white hover:bg-primary-700"
        >
          {{ isAuthenticated ? t('home.goToDashboard') : t('home.login') }}
        </router-link>
      </div>
    </main>

    <footer class="min-w-0 border-t border-gray-200 px-4 py-5 text-center text-sm text-gray-500 [overflow-wrap:anywhere] sm:px-6 dark:border-dark-800 dark:text-dark-400">
      &copy; {{ currentYear }} {{ siteName }}
    </footer>
  </div>

  <!-- LZ Particle Home -->
  <div v-else class="lz-home">
    <header class="lz-header">
      <router-link :to="isAuthenticated ? dashboardPath : '/home'" class="lz-brand" :aria-label="siteName">
        <span class="lz-brand-mark"><img :src="siteLogo" alt="" /></span>
        <strong class="lz-brand-name">{{ siteName }}</strong>
      </router-link>
      <nav class="lz-nav" aria-label="Primary navigation">
        <a v-if="siteDocURL" :href="siteDocURL" target="_blank" rel="noopener noreferrer">{{ t('home.docs') }}</a>
        <router-link to="/downloads">客户端下载</router-link>
        <router-link to="/key-usage">Key 用量</router-link>
        <router-link v-if="!isAuthenticated" to="/register">注册账号</router-link>
        <router-link to="/model-plaza">{{ t('nav.modelPlaza') }}</router-link>
        <button
          type="button"
          class="lz-theme-toggle"
          :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          @click="toggleTheme"
        >
          <Icon v-if="isDark" name="sun" size="sm" />
          <Icon v-else name="moon" size="sm" />
        </button>
        <router-link v-if="isAuthenticated" :to="dashboardPath" class="lz-account">{{ t('home.dashboard') }}</router-link>
        <router-link v-else to="/login" class="lz-login">{{ t('home.login') }}</router-link>
      </nav>
    </header>

    <main class="lz-main">
      <div class="lz-copy">
        <p class="lz-eyebrow"><span></span> {{ siteName }} / SUB2API</p>
        <h1>{{ siteName }}</h1>
        <p class="lz-subtitle whitespace-pre-wrap [overflow-wrap:anywhere]">{{ siteSubtitle }}</p>
        <p class="lz-description">{{ t('home.heroDescription') }}</p>
        <div class="lz-mode-controls" aria-label="Particle modes">
          <button
            v-for="mode in lzModes"
            :key="mode"
            type="button"
            :class="['lz-mode-button', { active: lzActiveMode === mode }]"
            @click="lzActiveMode = mode"
          >
            <span></span>{{ mode }}
          </button>
        </div>
        <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="lz-start">
          <span>开始</span>
        </router-link>
      </div>
      <div class="lz-particle-stage" aria-hidden="true">
        <LzParticleScene :active-mode="lzActiveMode" @cycle="cycleLzMode" />
        <div class="lz-orbit-ring lz-ring-one"></div>
        <div class="lz-orbit-ring lz-ring-two"></div>
        <div class="lz-crosshair"></div>
      </div>
      <div class="lz-status" aria-hidden="true">
        <span><i></i> PARTICLE FIELD / {{ lzActiveMode }}</span>
        <strong>ONLINE</strong>
        <small>AUTO CYCLE</small>
      </div>
    </main>

    <footer class="lz-footer">
      <span>ONE KEY / EVERY MODEL</span>
      <span>{{ currentYear }} · {{ siteName }}</span>
    </footer>
  </div>

</template>
<style scoped>
/* Terminal Container */
.terminal-container {
  position: relative;
  display: inline-block;
}

/* Terminal Window */
.terminal-window {
  width: 420px;
  background: linear-gradient(145deg, #1e293b 0%, #0f172a 100%);
  border-radius: 14px;
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.4),
    0 0 0 1px rgba(255, 255, 255, 0.1),
    inset 0 1px 0 rgba(255, 255, 255, 0.1);
  overflow: hidden;
  transform: perspective(1000px) rotateX(2deg) rotateY(-2deg);
  transition: transform 0.3s ease;
}

.terminal-window:hover {
  transform: perspective(1000px) rotateX(0deg) rotateY(0deg) translateY(-4px);
}

/* Terminal Header */
.terminal-header {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  background: rgba(30, 41, 59, 0.8);
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.terminal-buttons {
  display: flex;
  gap: 8px;
}

.terminal-buttons span {
  width: 12px;
  height: 12px;
  border-radius: 50%;
}

.btn-close {
  background: #ef4444;
}
.btn-minimize {
  background: #eab308;
}
.btn-maximize {
  background: #22c55e;
}

.terminal-title {
  flex: 1;
  text-align: center;
  font-size: 12px;
  font-family: ui-monospace, monospace;
  color: #64748b;
  margin-right: 52px;
}

/* Terminal Body */
.terminal-body {
  padding: 20px 24px;
  font-family: ui-monospace, 'Fira Code', monospace;
  font-size: 14px;
  line-height: 2;
}

.code-line {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  opacity: 0;
  animation: line-appear 0.5s ease forwards;
}

.line-1 {
  animation-delay: 0.3s;
}
.line-2 {
  animation-delay: 1s;
}
.line-3 {
  animation-delay: 1.8s;
}
.line-4 {
  animation-delay: 2.5s;
}

@keyframes line-appear {
  from {
    opacity: 0;
    transform: translateY(5px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.code-prompt {
  color: #22c55e;
  font-weight: bold;
}
.code-cmd {
  color: #38bdf8;
}
.code-flag {
  color: #a78bfa;
}
.code-url {
  color: #14b8a6;
}
.code-comment {
  color: #64748b;
  font-style: italic;
}
.code-success {
  color: #22c55e;
  background: rgba(34, 197, 94, 0.15);
  padding: 2px 8px;
  border-radius: 4px;
  font-weight: 600;
}
.code-response {
  color: #fbbf24;
}

/* Blinking Cursor */
.cursor {
  display: inline-block;
  width: 8px;
  height: 16px;
  background: #22c55e;
  animation: blink 1s step-end infinite;
}

@keyframes blink {
  0%,
  50% {
    opacity: 1;
  }
  51%,
  100% {
    opacity: 0;
  }
}

/* Dark mode adjustments */
:deep(.dark) .terminal-window {
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.6),
    0 0 0 1px rgba(20, 184, 166, 0.2),
    0 0 40px rgba(20, 184, 166, 0.1),
    inset 0 1px 0 rgba(255, 255, 255, 0.1);
}

.lz-home {
  position: relative;
  min-height: 100svh;
  overflow: hidden;
  isolation: isolate;
  background: #050814;
  color: #edf2ff;
}

.lz-home::before {
  position: absolute;
  inset: 0;
  z-index: -2;
  background:
    radial-gradient(circle at 67% 48%, rgba(96, 91, 255, 0.18), transparent 24%),
    radial-gradient(circle at 80% 62%, rgba(36, 211, 238, 0.1), transparent 18%),
    linear-gradient(135deg, #050814 0%, #080d1f 52%, #070b16 100%);
  content: '';
}

.lz-home-grid {
  position: absolute;
  inset: 0;
  z-index: -1;
  opacity: 0.34;
  background-image: linear-gradient(rgba(135, 153, 205, 0.07) 1px, transparent 1px), linear-gradient(90deg, rgba(135, 153, 205, 0.07) 1px, transparent 1px);
  background-size: 68px 68px;
  mask-image: linear-gradient(to bottom, transparent, black 18%, black 82%, transparent);
}

.lz-header,
.lz-main,
.lz-footer {
  position: relative;
  z-index: 1;
}

.lz-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 28px clamp(24px, 5vw, 76px);
}

.lz-brand {
  display: inline-flex;
  align-items: center;
  gap: 12px;
  color: inherit;
  text-decoration: none;
}

.lz-brand-mark {
  display: grid;
  height: 38px;
  width: 38px;
  place-items: center;
  overflow: hidden;
  border: 1px solid rgba(161, 173, 255, 0.34);
  border-radius: 11px;
  background: rgba(120, 130, 255, 0.12);
  box-shadow: 0 0 24px rgba(90, 100, 255, 0.28);
}

.lz-brand-mark img {
  height: 100%;
  width: 100%;
  object-fit: contain;
}

.lz-brand-copy {
  display: grid;
  gap: 3px;
}

.lz-brand-copy strong {
  font-size: 14px;
  font-weight: 700;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.lz-brand-copy small,
.lz-status,
.lz-footer,
.lz-nav {
  color: #9da8c5;
  font-size: 10px;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.lz-nav {
  display: flex;
  align-items: center;
  gap: 22px;
}

.lz-nav a {
  color: #aeb8d3;
  text-decoration: none;
  transition: color 180ms ease;
}

.lz-nav a:hover {
  color: #f3f6ff;
}

.lz-theme-toggle {
  display: inline-grid;
  height: 30px;
  width: 30px;
  place-items: center;
  border: 1px solid rgba(164, 177, 219, 0.24);
  border-radius: 50%;
  background: rgba(145, 157, 207, 0.08);
  color: #cbd4ed;
  cursor: pointer;
}

.lz-account,
.lz-login {
  border: 1px solid rgba(171, 181, 225, 0.3);
  border-radius: 999px;
  padding: 9px 14px;
  color: #e7ebff !important;
  background: rgba(152, 160, 231, 0.12);
}

.lz-main {
  display: flex;
  min-height: calc(100svh - 142px);
  align-items: center;
  padding: 0 clamp(24px, 9vw, 140px) 72px;
}

.lz-copy {
  max-width: 630px;
  transform: translateY(-2vh);
}

.lz-eyebrow {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 0 0 24px;
  color: #a8b4d9;
  font-size: 11px;
  letter-spacing: 0.24em;
}

.lz-eyebrow span {
  height: 5px;
  width: 5px;
  border-radius: 50%;
  background: #5eead4;
  box-shadow: 0 0 14px #5eead4;
}

.lz-copy h1 {
  max-width: 10ch;
  margin: 0;
  color: #f5f6ff;
  font-size: clamp(56px, 9vw, 132px);
  font-weight: 650;
  letter-spacing: -0.04em;
  line-height: 0.92;
}

.lz-subtitle {
  margin: 28px 0 0;
  color: #d5dcf4;
  font-size: clamp(18px, 2.2vw, 28px);
  letter-spacing: 0.02em;
}

.lz-description {
  max-width: 52ch;
  margin: 14px 0 0;
  color: #8e9abb;
  font-size: 15px;
  line-height: 1.8;
}

.lz-start {
  display: inline-flex;
  align-items: center;
  gap: 14px;
  margin-top: 34px;
  border: 1px solid rgba(207, 215, 255, 0.46);
  border-radius: 999px;
  padding: 13px 18px 13px 22px;
  color: #f5f7ff;
  background: rgba(124, 133, 255, 0.2);
  box-shadow: 0 0 34px rgba(97, 106, 255, 0.22);
  font-size: 13px;
  font-weight: 650;
  letter-spacing: 0.12em;
  text-decoration: none;
  transition: transform 180ms ease, background 180ms ease, box-shadow 180ms ease;
}

.lz-start:hover {
  transform: translateY(-2px);
  background: rgba(124, 133, 255, 0.32);
  box-shadow: 0 0 42px rgba(97, 106, 255, 0.34);
}

.lz-status {
  position: absolute;
  right: clamp(24px, 8vw, 126px);
  bottom: 12vh;
  display: grid;
  gap: 9px;
  text-align: right;
}

.lz-status strong {
  color: #d8e2ff;
  font-size: 12px;
  font-weight: 600;
}

.lz-status i {
  display: inline-block;
  height: 6px;
  width: 6px;
  margin-right: 7px;
  border-radius: 50%;
  background: #5eead4;
  box-shadow: 0 0 12px #5eead4;
}

.lz-status small {
  color: #7784a8;
  font-size: 9px;
}

.lz-footer {
  display: flex;
  justify-content: space-between;
  padding: 0 clamp(24px, 5vw, 76px) 25px;
  color: #6c7899;
}

@media (max-width: 700px) {
  .lz-header {
    align-items: flex-start;
    padding-top: 20px;
  }

  .lz-nav {
    gap: 10px;
  }

  .lz-nav > a:not(.lz-login):not(.lz-account) {
    display: none;
  }

  .lz-main {
    min-height: calc(100svh - 110px);
    padding-bottom: 110px;
  }

  .lz-copy h1 {
    font-size: clamp(54px, 17vw, 84px);
  }

  .lz-description {
    max-width: 34ch;
    font-size: 14px;
  }

  .lz-status {
    right: 24px;
    bottom: 74px;
  }

  .lz-footer {
    gap: 12px;
    font-size: 8px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .lz-start {
    transition: none;
  }
}

/* Keep the latest lz composition and palette aligned with lz/src/style.css. */
.lz-home {
  min-height: 100vh;
  background: radial-gradient(ellipse at 50% 48%, #121520 0%, #090b11 45%, #07080b 100%);
  color: #f0f1f5;
  font-family: Manrope, system-ui, sans-serif;
}

.lz-home::before {
  z-index: 0;
  opacity: 0.26;
  background-image: linear-gradient(rgba(255, 255, 255, 0.025) 1px, transparent 1px), linear-gradient(90deg, rgba(255, 255, 255, 0.025) 1px, transparent 1px);
  background-size: 54px 54px;
  mask-image: linear-gradient(to bottom, transparent 0%, black 22%, black 76%, transparent 100%);
}

.lz-header {
  height: 78px;
  padding: 0 52px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.07);
}

.lz-brand {
  font-size: 20px;
  font-weight: 700;
  letter-spacing: 0.04em;
}

.lz-brand-mark,
.lz-brand-copy {
  display: none;
}

.lz-brand::after {
  content: '';
}

.lz-nav {
  gap: 16px;
  font: 500 11px 'DM Mono', monospace;
  letter-spacing: 0.08em;
}

.lz-nav > a:not(.lz-login) {
  color: #959ba9;
}

.lz-theme-toggle {
  height: auto;
  width: auto;
  border: 0;
  border-radius: 0;
  background: none;
  color: #959ba9;
}

.lz-login {
  border: 1px solid rgba(165, 245, 224, 0.65);
  border-radius: 0;
  padding: 10px 17px;
  background: transparent;
  color: #b9f8e8 !important;
}

.lz-main {
  min-height: calc(100vh - 78px);
  align-items: center;
  justify-content: center;
  padding: 78px 24px 72px;
  text-align: center;
}

.lz-copy {
  max-width: 900px;
  transform: none;
}

.lz-eyebrow {
  justify-content: center;
  margin-bottom: 23px;
  font: 500 10px 'DM Mono', monospace;
  letter-spacing: 0.19em;
  color: #737b8c;
}

.lz-eyebrow span {
  width: 5px;
  height: 5px;
  background: #8cf1d7;
  box-shadow: none;
}

.lz-copy h1 {
  max-width: none;
  margin: 23px 0 13px;
  font-size: clamp(54px, 7vw, 96px);
  line-height: 0.82;
  letter-spacing: -0.07em;
  font-weight: 800;
  color: #f8f8fb;
}

.lz-subtitle {
  margin: 0;
  font: 500 13px 'DM Mono', monospace;
  letter-spacing: 0.12em;
  color: #a5abb9;
}

.lz-description {
  max-width: 65ch;
  margin: 27px auto 0;
  color: #d5d9e2;
  font-size: 14px;
  line-height: 1.6;
}

.lz-mode-controls {
  display: flex;
  justify-content: center;
  gap: 10px;
  margin-top: 25px;
}

.lz-mode-button {
  min-width: 112px;
  height: 40px;
  border: 1px solid rgba(183, 191, 208, 0.35);
  background: rgba(11, 13, 18, 0.45);
  color: #aeb5c4;
  font: 500 11px 'DM Mono', monospace;
  letter-spacing: 0.14em;
  cursor: pointer;
  transition: all 0.25s;
}

.lz-mode-button:hover,
.lz-mode-button.active {
  border-color: #9cf1dd;
  color: #d8fff4;
  background: rgba(130, 238, 214, 0.08);
  box-shadow: 0 0 18px rgba(115, 237, 209, 0.1);
}

.lz-mode-button span,
.lz-status > span:first-child::before {
  display: inline-block;
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: currentColor;
  content: '';
}

.lz-mode-button span {
  margin-right: 9px;
  vertical-align: 2px;
}

.lz-start {
  margin-top: 0;
  border-radius: 0;
  padding: 10px 18px;
  background: transparent;
  box-shadow: none;
  color: #b9f8e8;
  font: 700 11px 'DM Mono', monospace;
  letter-spacing: 0.12em;
}

.lz-start:hover {
  background: rgba(130, 238, 214, 0.08);
  box-shadow: 0 0 18px rgba(115, 237, 209, 0.15);
}

.lz-status {
  right: 52px;
  bottom: 24px;
  display: flex;
  align-items: center;
  gap: 10px;
  color: #687183;
  font: 500 9px 'DM Mono', monospace;
  letter-spacing: 0.19em;
}

.lz-status strong,
.lz-status small {
  color: inherit;
  font-size: inherit;
  font-weight: inherit;
}

.lz-status i {
  width: 28px;
  height: 1px;
  margin: 0;
  border-radius: 0;
  background: #323846;
  box-shadow: none;
}

.lz-footer {
  position: absolute;
  bottom: 24px;
  left: 52px;
  right: 52px;
  padding: 0;
  color: #555d6c;
  font: 500 9px 'DM Mono', monospace;
  letter-spacing: 0.1em;
}

@media (max-width: 900px) {
  .lz-header { padding: 0 22px; }
  .lz-mode-button { min-width: 84px; }
  .lz-status { right: 22px; }
  .lz-footer { left: 22px; right: 22px; }
}

@media (max-width: 700px) {
  .lz-header { height: 66px; padding: 0 5vw; }
  .lz-nav > a:not(.lz-login):not(.lz-account) { display: none; }
  .lz-main { min-height: calc(100vh - 66px); padding: 52px 16px 90px; }
  .lz-mode-controls { display: grid; width: 100%; grid-template-columns: 1fr 1fr; gap: 8px; }
  .lz-mode-button { width: 100%; }
  .lz-status { right: 16px; bottom: 58px; font-size: 8px; letter-spacing: 0.11em; }
  .lz-status i { width: 16px; }
  .lz-footer { bottom: 15px; font-size: 8px; }
}

/* Latest lz layout: brand at top-left, copy and particle field occupy separate regions. */
.lz-header {
  position: relative;
  z-index: 4;
}

.lz-brand-mark {
  display: grid;
  height: 34px;
  width: 34px;
  place-items: center;
  overflow: hidden;
  border: 1px solid rgba(165, 245, 224, 0.55);
  border-radius: 8px;
  background: rgba(11, 13, 18, 0.72);
  box-shadow: 0 0 18px rgba(115, 237, 209, 0.12);
}

.lz-brand-mark img {
  height: 100%;
  width: 100%;
  object-fit: cover;
}

.lz-brand-name {
  color: #f8f8fb;
  font-size: 18px;
  font-weight: 700;
  letter-spacing: 0.12em;
}

.lz-main {
  position: relative;
  display: grid;
  min-height: calc(100vh - 78px);
  grid-template-columns: minmax(0, 0.9fr) minmax(420px, 1.1fr);
  align-items: center;
  gap: clamp(24px, 5vw, 80px);
  padding: 32px clamp(24px, 6vw, 92px) 86px;
  text-align: left;
}

.lz-copy {
  position: relative;
  z-index: 2;
  max-width: 600px;
}

.lz-eyebrow {
  justify-content: flex-start;
}

.lz-copy h1 {
  font-size: clamp(62px, 8vw, 118px);
}

.lz-description {
  margin-left: 0;
  margin-right: 0;
}

.lz-mode-controls {
  justify-content: flex-start;
  flex-wrap: wrap;
}

.lz-particle-stage {
  position: relative;
  z-index: 1;
  width: 100%;
  min-height: min(58vw, 620px);
  overflow: hidden;
  border: 1px solid rgba(165, 245, 224, 0.12);
  border-radius: 50%;
  background: radial-gradient(circle, rgba(21, 28, 44, 0.2), transparent 65%);
  box-shadow: 0 0 80px rgba(86, 129, 255, 0.08), inset 0 0 90px rgba(116, 237, 210, 0.04);
}

.lz-orbit-ring {
  position: absolute;
  top: 50%;
  left: 50%;
  z-index: 1;
  width: 88%;
  height: 44%;
  border: 1px solid rgba(166, 245, 225, 0.2);
  border-radius: 50%;
  pointer-events: none;
  transform: translate(-50%, -50%) rotate(-8deg);
  animation: lz-orbit-spin 18s linear infinite;
}

.lz-ring-two {
  width: 58%;
  height: 88%;
  border-color: rgba(153, 180, 255, 0.18);
  transform: translate(-50%, -50%) rotate(63deg);
  animation-direction: reverse;
  animation-duration: 24s;
}

.lz-crosshair {
  position: absolute;
  inset: 25% 8%;
  z-index: 1;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  pointer-events: none;
}

@keyframes lz-orbit-spin {
  to { transform: translate(-50%, -50%) rotate(352deg); }
}

.lz-status {
  z-index: 3;
  right: clamp(24px, 6vw, 92px);
  bottom: 28px;
}

@media (max-width: 900px) {
  .lz-main {
    grid-template-columns: minmax(0, 1fr) minmax(300px, 0.9fr);
    gap: 24px;
    padding-inline: 24px;
  }
  .lz-particle-stage { min-height: 420px; }
}

@media (max-width: 700px) {
  .lz-header { padding-inline: 20px; }
  .lz-brand-name { font-size: 16px; }
  .lz-main {
    display: flex;
    min-height: auto;
    flex-direction: column;
    align-items: stretch;
    gap: 24px;
    padding: 38px 20px 100px;
  }
  .lz-copy { max-width: none; }
  .lz-eyebrow { justify-content: flex-start; }
  .lz-copy h1 { font-size: clamp(58px, 18vw, 84px); }
  .lz-particle-stage {
    order: 2;
    min-height: min(86vw, 420px);
    border-radius: 50%;
  }
  .lz-status { right: 20px; bottom: 56px; }
}

/* Keep the requested hierarchy after the responsive layout overrides above. */
.lz-copy h1 { font-size: clamp(46px, 5.5vw, 78px); }
.lz-subtitle {
  margin-top: 22px;
  font-size: clamp(20px, 2.5vw, 32px);
  letter-spacing: 0.04em;
}
.lz-start { margin-top: 12px; }

@media (max-width: 700px) {
  .lz-copy h1 { font-size: clamp(44px, 14vw, 68px); }
  .lz-subtitle { font-size: clamp(18px, 5vw, 24px); }
}
</style>
