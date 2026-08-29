import { fileURLToPath } from 'node:url'

import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  plugins: [vue()],
  test: {
    // Unit specs are pure functions and stores; the integration specs mount
    // components, which need a DOM. One environment for both keeps the setup
    // to a single file.
    environment: 'jsdom',
    include: ['tests/**/*.spec.ts'],
    setupFiles: ['tests/setup.ts'],
    // The app reads its API base from this at module load, so it has to be set
    // before any spec imports the client.
    env: {
      VITE_API_BASE: '/api/v1',
    },
    coverage: {
      provider: 'v8',
      include: ['src/**/*.ts', 'src/**/*.vue'],
      // Generated from the theme scripts, and asserting on it would just
      // restate the generator.
      exclude: ['src/styles/**', 'src/env.d.ts'],
    },
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
})
