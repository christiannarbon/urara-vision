/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, unknown>
  export default component
}

declare module 'cytoscape-fcose' {
  import type { Ext } from 'cytoscape'
  const ext: Ext
  export default ext
}

declare module 'cytoscape-dagre' {
  import type { Ext } from 'cytoscape'
  const ext: Ext
  export default ext
}

interface ImportMetaEnv {
  readonly VITE_API_BASE?: string
  // The chat service, proxied at its own prefix. Separate from VITE_API_BASE
  // because it is a separate service behind a separate nginx location.
  readonly VITE_CHAT_BASE?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
