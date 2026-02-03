/// <reference types="vite/client" />

// Vue single-file component type declarations
declare module '*.vue' {
    import type { DefineComponent } from 'vue'
    const component: DefineComponent<{}, {}, any>
    export default component
}

interface ImportMetaEnv {
    readonly CONTROLLER_ENDPOINT: string
}

interface ImportMeta {
    readonly env: ImportMetaEnv
}