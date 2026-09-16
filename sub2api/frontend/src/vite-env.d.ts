/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL: string
  /** Public URL of the separately deployed infinite-canvas frontend. */
  readonly VITE_CANVAS_URL?: string
  /** Public URL of the separately deployed smart short-drama frontend. */
  readonly VITE_SHORT_DRAMA_URL?: string
  readonly VITE_SHORT_DRAMA_SSO_URL?: string
  readonly BASE_URL: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}

declare module '*.md?raw' {
  const content: string
  export default content
}
