/** Mounts App the way main.ts does, on an in-memory router at `path`. */

import { mount } from '@vue/test-utils'
import { createPinia, getActivePinia, setActivePinia } from 'pinia'
import { createMemoryHistory } from 'vue-router'

import App from '../../src/App.vue'
import { createAppRouter } from '../../src/router'

export async function mountApp(path = '/', options: { attachTo?: Element | string } = {}) {
  let pinia = getActivePinia()
  if (!pinia) {
    pinia = createPinia()
    setActivePinia(pinia)
  }
  const router = createAppRouter(createMemoryHistory())
  await router.push(path)
  await router.isReady()
  return mount(App, {
    attachTo: options.attachTo,
    global: { plugins: [pinia, router], stubs: { GraphCanvas: true } },
  })
}
