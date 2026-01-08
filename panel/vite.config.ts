import { fileURLToPath, URL } from 'url'

import { defineConfig } from 'vite'
// @ts-ignore
import vue from '@vitejs/plugin-vue'

// https://vitejs.dev/config/
export default defineConfig({
    plugins: [vue()],
    resolve: {
        alias: {
            '@': fileURLToPath(new URL('./src', import.meta.url))
        }
    },
    define: {
        'import.meta.env.CONTROLLER_ENDPOINT': JSON.stringify(process.env.CONTROLLER_ENDPOINT || 'http://localhost:8080'),
    },
    server: {
        allowedHosts: ["localhost", "devapp.netwatcher.io", "app.netwatcher.io"],
        host: "0.0.0.0",
        port: 3000,
        hmr: {
            //clientPort: 443,
            // host: '10.0.1.2',
            // port: 8080,
            // protocol: 'wss'
        },
        strictPort: false
    }

})
