/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL?: string
  readonly VITE_CANVAS_URL?: string
  readonly VITE_QRCODE_URL?: string
  readonly VITE_PUBLIC_API_URL?: string
  readonly VITE_LINK?: string
  readonly VITE_AICUT_LINK?: string
  readonly VITE_AIEXCEL_LINK?: string
  readonly VITE_CANVAS_LINK?: string
  readonly VITE_JU_LINK?: string
  readonly VITE_LIVART_LINK?: string
  readonly VITE_PPT_LINK?: string
  readonly VITE_QRCODE_LINK?: string
  readonly VITE_SCREEN2CODE_LINK?: string
  readonly VITE_CANVAS_SSO_URL?: string
  readonly VITE_SHORT_DRAMA_SSO_URL?: string
  readonly VITE_SUPER_CANVAS_SSO_URL?: string
  readonly VITE_SCREEN2CODE_SSO_URL?: string
  readonly VITE_PPT_SSO_URL?: string
  readonly VITE_AICUT_SSO_URL?: string
  readonly VITE_AIEXCEL_SSO_URL?: string
  readonly VITE_QRCODE_SSO_URL?: string
  readonly VITE_YIBIAO_SSO_URL?: string
  readonly VITE_WS_BASE_URL?: string
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
