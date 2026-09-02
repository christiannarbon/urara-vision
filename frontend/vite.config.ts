import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// The compose backend's token, which the dev proxy presents the way nginx does
// in a built image. Override it for a backend running with a different one, or
// set it empty for one running with none.
const apiToken = process.env.VITE_PROXY_TOKEN ?? 'relviz-dev-token-not-for-production'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    // In dev the app talks to the backend on its own port; proxying keeps the
    // API same-origin so the client can use a relative base everywhere.
    proxy: {
      '/api': {
        target: process.env.VITE_PROXY_TARGET ?? 'http://localhost:8080',
        changeOrigin: true,
        // Same rule the built image's nginx follows: fill in the credential
        // only when the browser sent none of its own.
        configure: (proxy) => {
          proxy.on('proxyReq', (req) => {
            if (apiToken && !req.getHeader('authorization')) {
              req.setHeader('Authorization', `Bearer ${apiToken}`)
            }
          })
        },
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    rollupOptions: {
      output: {
        // Cytoscape is by far the largest dependency; splitting it keeps the
        // app chunk small enough to cache independently of library upgrades.
        manualChunks: {
          cytoscape: ['cytoscape', 'cytoscape-fcose'],
          vue: ['vue', 'pinia'],
        },
      },
    },
  },
})
