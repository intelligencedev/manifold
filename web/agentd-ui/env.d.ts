/// <reference types="vite/client" />

declare module "*.vue" {
  import type { DefineComponent } from "vue";
  const component: DefineComponent<
    Record<string, unknown>,
    Record<string, unknown>,
    any
  >;
  export default component;
}

interface ImportMetaEnv {
  readonly VITE_AGENT_API_BASE_URL?: string;
  readonly VITE_AGENTD_BASE_URL?: string;
  readonly VITE_DEBUG_SSE?: string;
  readonly VITE_MANIFOLD_FEATURE_GATE?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
