<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="hasHomeContent" class="min-h-screen">
    <!-- iframe mode -->
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- HTML mode - SECURITY: homeContent is admin-only setting, XSS risk is acceptable -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Compact Home Page -->
  <div
    v-else-if="compactHomeEnabled"
    data-testid="compact-home"
    class="flex min-h-screen flex-col bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white"
  >
    <header class="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-dark-800">
      <nav class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 sm:gap-4">
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <img
            :src="siteLogo || '/logo.jpg'"
            alt="Logo"
            class="h-9 w-9 shrink-0 rounded-lg object-contain"
          />
          <span class="min-w-0 truncate text-base font-semibold">{{ siteName }}</span>
        </div>
        <div class="flex max-w-full shrink-0 flex-wrap items-center justify-end gap-2">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <router-link
            v-if="showModelPlazaEntry"
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
          :src="siteLogo || '/logo.jpg'"
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
      <router-link :to="isAuthenticated ? dashboardPath : '/home'" class="lz-brand" aria-label="超参池X">
        <span class="lz-brand-mark"><img src="/logo.jpg" alt="" /></span>
        <strong class="lz-brand-name">超参池</strong>
      </router-link>
      <nav class="lz-nav" aria-label="Primary navigation">
        <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.docs') }}</a>
        <router-link to="/downloads">{{ t('downloads.nav') }}</router-link>
        <router-link v-if="showModelPlazaEntry" to="/model-plaza">{{ t('nav.modelPlaza') }}</router-link>
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
        <p class="lz-eyebrow"><span></span> CC 2 CX / SUB2API</p>
        <h1>{{ siteName }}</h1>
        <p class="lz-subtitle">{{ siteSubtitle }}</p>
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
          <span>{{ t('home.start') }}</span>
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

  <!-- Default Home Page -->
  <div
    v-if="false"
    class="relative flex min-h-screen flex-col overflow-hidden bg-gradient-to-br from-gray-50 via-primary-50/30 to-gray-100 dark:from-dark-950 dark:via-dark-900 dark:to-dark-950"
  >
    <!-- Background Decorations -->
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <div
        class="absolute -right-40 -top-40 h-96 w-96 rounded-full bg-primary-400/20 blur-3xl"
      ></div>
      <div
        class="absolute -bottom-40 -left-40 h-96 w-96 rounded-full bg-primary-500/15 blur-3xl"
      ></div>
      <div
        class="absolute left-1/3 top-1/4 h-72 w-72 rounded-full bg-primary-300/10 blur-3xl"
      ></div>
      <div
        class="absolute bottom-1/4 right-1/4 h-64 w-64 rounded-full bg-primary-400/10 blur-3xl"
      ></div>
      <div
        class="absolute inset-0 bg-[linear-gradient(rgba(20,184,166,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(20,184,166,0.03)_1px,transparent_1px)] bg-[size:64px_64px]"
      ></div>
    </div>

    <!-- Header -->
    <header class="relative z-20 px-6 py-4">
      <nav class="mx-auto flex max-w-6xl items-center justify-between">
        <!-- Logo -->
        <div class="flex items-center">
          <div class="h-10 w-10 overflow-hidden rounded-xl shadow-md">
            <img :src="siteLogo || '/logo.jpg'" alt="Logo" class="h-full w-full object-contain" />
          </div>
        </div>

        <!-- Nav Actions -->
        <div class="flex items-center gap-3">
          <!-- Language Switcher -->
          <LocaleSwitcher />

          <!-- Doc Link -->
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>

          <!-- Model Plaza Link -->
          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="inline-flex items-center gap-1.5 rounded-lg p-2 text-sm text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('nav.modelPlaza')"
          >
            <Icon name="grid" size="md" />
            <span class="hidden sm:inline">{{ t('nav.modelPlaza') }}</span>
          </router-link>

          <!-- Theme Toggle -->
          <button
            @click="toggleTheme"
            class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>

          <!-- Login / Dashboard Button -->
          <router-link
            v-if="isAuthenticated"
            :to="dashboardPath"
            class="inline-flex items-center gap-1.5 rounded-full bg-gray-900 py-1 pl-1 pr-2.5 transition-colors hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
          >
            <span
              class="flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br from-primary-400 to-primary-600 text-[10px] font-semibold text-white"
            >
              {{ userInitial }}
            </span>
            <span class="text-xs font-medium text-white">{{ t('home.dashboard') }}</span>
            <svg
              class="h-3 w-3 text-gray-400"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              stroke-width="2"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M4.5 19.5l15-15m0 0H8.25m11.25 0v11.25"
              />
            </svg>
          </router-link>
          <router-link
            v-else
            to="/login"
            class="inline-flex items-center rounded-full bg-gray-900 px-3 py-1 text-xs font-medium text-white transition-colors hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
          >
            {{ t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <!-- Main Content -->
    <main class="relative z-10 flex-1 px-6 py-16">
      <div class="mx-auto max-w-6xl">
        <!-- Hero Section - Left/Right Layout -->
        <div class="mb-12 flex flex-col items-center justify-between gap-12 lg:flex-row lg:gap-16">
          <!-- Left: Text Content -->
          <div class="flex-1 text-center lg:text-left">
            <h1
              class="mb-4 text-4xl font-bold text-gray-900 dark:text-white md:text-5xl lg:text-6xl"
            >
              {{ siteName }}
            </h1>
            <p class="mb-8 text-lg text-gray-600 dark:text-dark-300 md:text-xl">
              {{ siteSubtitle }}
            </p>

            <!-- CTA Button -->
            <div>
              <router-link
                :to="isAuthenticated ? dashboardPath : '/login'"
                class="btn btn-primary px-8 py-3 text-base shadow-lg shadow-primary-500/30"
              >
                {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
                <Icon name="arrowRight" size="md" class="ml-2" :stroke-width="2" />
              </router-link>
            </div>
          </div>

          <!-- Right: Terminal Animation -->
          <div class="flex flex-1 justify-center lg:justify-end">
            <div class="terminal-container">
              <div class="terminal-window">
                <!-- Window header -->
                <div class="terminal-header">
                  <div class="terminal-buttons">
                    <span class="btn-close"></span>
                    <span class="btn-minimize"></span>
                    <span class="btn-maximize"></span>
                  </div>
                  <span class="terminal-title">terminal</span>
                </div>
                <!-- Terminal content -->
                <div class="terminal-body">
                  <div class="code-line line-1">
                    <span class="code-prompt">$</span>
                    <span class="code-cmd">curl</span>
                    <span class="code-flag">-X POST</span>
                    <span class="code-url">/v1/messages</span>
                  </div>
                  <div class="code-line line-2">
                    <span class="code-comment"># Routing to upstream...</span>
                  </div>
                  <div class="code-line line-3">
                    <span class="code-success">200 OK</span>
                    <span class="code-response">{ "content": "Hello!" }</span>
                  </div>
                  <div class="code-line line-4">
                    <span class="code-prompt">$</span>
                    <span class="cursor"></span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Feature Tags - Centered -->
        <div class="mb-12 flex flex-wrap items-center justify-center gap-4 md:gap-6">
          <div
            class="inline-flex items-center gap-2.5 rounded-full border border-gray-200/50 bg-white/80 px-5 py-2.5 shadow-sm backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/80"
          >
            <Icon name="swap" size="sm" class="text-primary-500" />
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{
              t('home.tags.subscriptionToApi')
            }}</span>
          </div>
          <div
            class="inline-flex items-center gap-2.5 rounded-full border border-gray-200/50 bg-white/80 px-5 py-2.5 shadow-sm backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/80"
          >
            <Icon name="shield" size="sm" class="text-primary-500" />
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{
              t('home.tags.stickySession')
            }}</span>
          </div>
          <div
            class="inline-flex items-center gap-2.5 rounded-full border border-gray-200/50 bg-white/80 px-5 py-2.5 shadow-sm backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/80"
          >
            <Icon name="chart" size="sm" class="text-primary-500" />
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{
              t('home.tags.realtimeBilling')
            }}</span>
          </div>
        </div>

        <!-- Features Grid -->
        <div class="mb-12 grid gap-6 md:grid-cols-3">
          <!-- Feature 1: Unified Gateway -->
          <div
            class="group rounded-2xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-xl hover:shadow-primary-500/10 dark:border-dark-700/50 dark:bg-dark-800/60"
          >
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-blue-500 to-blue-600 shadow-lg shadow-blue-500/30 transition-transform group-hover:scale-110"
            >
              <Icon name="server" size="lg" class="text-white" />
            </div>
            <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('home.features.unifiedGateway') }}
            </h3>
            <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">
              {{ t('home.features.unifiedGatewayDesc') }}
            </p>
          </div>

          <!-- Feature 2: Account Pool -->
          <div
            class="group rounded-2xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-xl hover:shadow-primary-500/10 dark:border-dark-700/50 dark:bg-dark-800/60"
          >
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-primary-500 to-primary-600 shadow-lg shadow-primary-500/30 transition-transform group-hover:scale-110"
            >
              <svg
                class="h-6 w-6 text-white"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M18 18.72a9.094 9.094 0 003.741-.479 3 3 0 00-4.682-2.72m.94 3.198l.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0112 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 016 18.719m12 0a5.971 5.971 0 00-.941-3.197m0 0A5.995 5.995 0 0012 12.75a5.995 5.995 0 00-5.058 2.772m0 0a3 3 0 00-4.681 2.72 8.986 8.986 0 003.74.477m.94-3.197a5.971 5.971 0 00-.94 3.197M15 6.75a3 3 0 11-6 0 3 3 0 016 0zm6 3a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0zm-13.5 0a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0z"
                />
              </svg>
            </div>
            <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('home.features.multiAccount') }}
            </h3>
            <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">
              {{ t('home.features.multiAccountDesc') }}
            </p>
          </div>

          <!-- Feature 3: Billing & Quota -->
          <div
            class="group rounded-2xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-xl hover:shadow-primary-500/10 dark:border-dark-700/50 dark:bg-dark-800/60"
          >
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-purple-500 to-purple-600 shadow-lg shadow-purple-500/30 transition-transform group-hover:scale-110"
            >
              <svg
                class="h-6 w-6 text-white"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M2.25 18.75a60.07 60.07 0 0115.797 2.101c.727.198 1.453-.342 1.453-1.096V18.75M3.75 4.5v.75A.75.75 0 013 6h-.75m0 0v-.375c0-.621.504-1.125 1.125-1.125H20.25M2.25 6v9m18-10.5v.75c0 .414.336.75.75.75h.75m-1.5-1.5h.375c.621 0 1.125.504 1.125 1.125v9.75c0 .621-.504 1.125-1.125 1.125h-.375m1.5-1.5H21a.75.75 0 00-.75.75v.75m0 0H3.75m0 0h-.375a1.125 1.125 0 01-1.125-1.125V15m1.5 1.5v-.75A.75.75 0 003 15h-.75M15 10.5a3 3 0 11-6 0 3 3 0 016 0zm3 0h.008v.008H18V10.5zm-12 0h.008v.008H6V10.5z"
                />
              </svg>
            </div>
            <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('home.features.balanceQuota') }}
            </h3>
            <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">
              {{ t('home.features.balanceQuotaDesc') }}
            </p>
          </div>
        </div>

        <!-- Supported Providers -->
        <div class="mb-8 text-center">
          <h2 class="mb-3 text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('home.providers.title') }}
          </h2>
          <p class="text-sm text-gray-600 dark:text-dark-400">
            {{ t('home.providers.description') }}
          </p>
        </div>

        <div class="mb-16 flex flex-wrap items-center justify-center gap-4">
          <!-- Claude - Supported -->
          <div
            class="flex items-center gap-2 rounded-xl border border-primary-200 bg-white/60 px-5 py-3 ring-1 ring-primary-500/20 backdrop-blur-sm dark:border-primary-800 dark:bg-dark-800/60"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-orange-400 to-orange-500"
            >
              <span class="text-xs font-bold text-white">C</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.claude') }}</span>
            <span
              class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
              >{{ t('home.providers.supported') }}</span
            >
          </div>
          <!-- GPT - Supported -->
          <div
            class="flex items-center gap-2 rounded-xl border border-primary-200 bg-white/60 px-5 py-3 ring-1 ring-primary-500/20 backdrop-blur-sm dark:border-primary-800 dark:bg-dark-800/60"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-green-500 to-green-600"
            >
              <span class="text-xs font-bold text-white">G</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">GPT</span>
            <span
              class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
              >{{ t('home.providers.supported') }}</span
            >
          </div>
          <!-- Gemini - Supported -->
          <div
            class="flex items-center gap-2 rounded-xl border border-primary-200 bg-white/60 px-5 py-3 ring-1 ring-primary-500/20 backdrop-blur-sm dark:border-primary-800 dark:bg-dark-800/60"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-blue-500 to-blue-600"
            >
              <span class="text-xs font-bold text-white">G</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.gemini') }}</span>
            <span
              class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
              >{{ t('home.providers.supported') }}</span
            >
          </div>
          <!-- Antigravity - Supported -->
          <div
            class="flex items-center gap-2 rounded-xl border border-primary-200 bg-white/60 px-5 py-3 ring-1 ring-primary-500/20 backdrop-blur-sm dark:border-primary-800 dark:bg-dark-800/60"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-rose-500 to-pink-600"
            >
              <span class="text-xs font-bold text-white">A</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.antigravity') }}</span>
            <span
              class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
              >{{ t('home.providers.supported') }}</span
            >
          </div>
          <!-- More - Coming Soon -->
          <div
            class="flex items-center gap-2 rounded-xl border border-gray-200/50 bg-white/40 px-5 py-3 opacity-60 backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/40"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-gray-500 to-gray-600"
            >
              <span class="text-xs font-bold text-white">+</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.more') }}</span>
            <span
              class="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-dark-700 dark:text-dark-400"
              >{{ t('home.providers.soon') }}</span
            >
          </div>
        </div>
      </div>
    </main>

    <!-- Footer -->
    <footer class="relative z-10 border-t border-gray-200/50 px-6 py-8 dark:border-dark-800/50">
      <div
        class="mx-auto flex max-w-6xl flex-col items-center justify-center gap-4 text-center sm:flex-row sm:text-left"
      >
        <p class="text-sm text-gray-500 dark:text-dark-400">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>
        <div class="flex items-center gap-4">
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-dark-400 dark:hover:text-white"
          >
            {{ t('home.docs') }}
          </a>
          <a
            :href="githubUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-dark-400 dark:hover:text-white"
          >
            GitHub
          </a>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import LzParticleScene, { type LzMode } from '@/components/home/LzParticleScene.vue'
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

// Site settings - directly from appStore (already initialized from injected config)
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || '超参池X')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform')
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const hasHomeContent = computed(() => homeContent.value.trim().length > 0)
const compactHomeEnabled = computed(() => appStore.cachedPublicSettings?.compact_home_enabled === true)
const modelPlazaEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza))

// Check if homeContent is a URL (for iframe display)
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

// Theme
const isDark = ref(document.documentElement.classList.contains('dark'))

// GitHub URL
const githubUrl = 'https://github.com/Wei-Shaw/sub2api'

// Auth state
const isAuthenticated = computed(() => authStore.isAuthenticated)
const modelPlazaRequiresAuth = computed(
  () => appStore.cachedPublicSettings?.model_plaza_require_auth === true,
)
const showModelPlazaEntry = computed(
  () => modelPlazaEnabled.value && (isAuthenticated.value || !modelPlazaRequiresAuth.value),
)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const userInitial = computed(() => {
  const user = authStore.user
  if (!user || !user.email) return ''
  return user.email.charAt(0).toUpperCase()
})

const lzModes: LzMode[] = ['AIFX', 'CC2CX', 'OPENAI', 'DEEPSEEK']
const lzActiveMode = ref<LzMode>('AIFX')

function cycleLzMode() {
  const currentIndex = lzModes.indexOf(lzActiveMode.value)
  lzActiveMode.value = lzModes[(currentIndex + 1) % lzModes.length]
}

// Current year for footer
const currentYear = computed(() => new Date().getFullYear())

// Toggle theme
function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

// Initialize theme
function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    savedTheme !== 'light'
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

onMounted(() => {
  initTheme()

  // Check auth state
  authStore.checkAuth()

  // Ensure public settings are loaded (will use cache if already loaded from injected config)
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

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
