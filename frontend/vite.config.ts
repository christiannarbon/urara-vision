import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

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
