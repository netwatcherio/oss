/// <reference types="vite/client" />
interface ImportMetaEnv {
    readonly NW_ENDPOINT: string
    readonly NW_GLOBAL_ENDPOINT: string
}

interface ImportMeta {
    readonly env: ImportMetaEnv
}