/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, any>
  export default component
}

interface ImportMetaEnv {
  readonly VITE_AGENT_API_BASE_URL: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
