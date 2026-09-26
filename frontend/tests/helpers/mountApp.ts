/** Mounts App the way main.ts does, on an in-memory router at `path`. */

import { mount } from '@vue/test-utils'
import { createPinia, getActivePinia, setActivePinia } from 'pinia'
import { createMemoryHistory } from 'vue-router'

import App from '../../src/App.vue'
import { createAppRouter, installAuthGuard } from '../../src/router'
import { useAuth } from '../../src/stores/auth'

export const ADMIN_PERMISSIONS = [
  'chat.use', 'note.moderate', 'note.write', 'project.delete', 'project.import', 'project.view',
  'settings.manage', 'user.delete', 'user.manage', 'version.delete', 'version.import',
]

/** Seeds the auth store as `/me` would, so specs need not mock it. */
export function seedAuth(signedIn: boolean) {
  const auth = useAuth()
  auth.loaded = true
  if (!signedIn) {
    auth.clear()
    return
  }
  auth.kind = 'user'
  auth.user = {
    id: 'u-admin', username: 'admin', displayName: 'Admin', role: 'admin',
    createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
  }
  auth.permissions = ADMIN_PERMISSIONS
}

export async function mountApp(
  path = '/',
  options: { attachTo?: Element | string; signedIn?: boolean } = {},
) {
  let pinia = getActivePinia()
  if (!pinia) {
    pinia = createPinia()
    setActivePinia(pinia)
  }
  seedAuth(options.signedIn ?? true)
  const router = createAppRouter(createMemoryHistory())
  installAuthGuard(router)
  await router.push(path)
  await router.isReady()
  return mount(App, {
    attachTo: options.attachTo,
    global: { plugins: [pinia, router], stubs: { GraphCanvas: true } },
  })
}
