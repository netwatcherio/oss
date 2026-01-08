/// <reference types="vite/client" />
interface ImportMetaEnv {
    readonly CONTROLLER_ENDPOINT: string
}

interface ImportMeta {
    readonly env: ImportMetaEnv
}