/** Who is signed in. The session itself is an HttpOnly cookie JavaScript never sees. */

import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { api, ApiError } from '../api/client'
import { useChat } from './chat'
import { useDiff } from './diff'
import { useWorkspace } from './workspace'
import type { Me, User } from '../api/types'

export const useAuth = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const kind = ref<Me['kind'] | null>(null)
  const permissions = ref<string[]>([])
  const loaded = ref(false)

  // Anonymous (AUTH_DISABLED) has no user but is signed in.
  const signedIn = computed(() => kind.value !== null)

  function clear() {
    user.value = null
    kind.value = null
    permissions.value = []
    // The next user must not see the last one's data.
    useChat().reset()
    useDiff().reset()
    const workspace = useWorkspace()
    workspace.clearSnapshot()
    workspace.projects = []
    workspace.snapshots = []
  }

  /** A 401 means signed out; anything else is rethrown after clearing. */
  async function load() {
    try {
      const me = await api.me()
      user.value = me.user
      kind.value = me.kind
      permissions.value = me.permissions
    } catch (e) {
      clear()
      if (!(e instanceof ApiError && e.status === 401)) throw e
    } finally {
      loaded.value = true
    }
  }

  async function login(username: string, password: string) {
    await api.login(username, password)
    await load()
  }

  async function logout() {
    try {
      await api.logout()
    } finally {
      clear()
    }
  }

  function can(p: string): boolean {
    return permissions.value.includes(p)
  }

  return { user, kind, permissions, loaded, signedIn, clear, load, login, logout, can }
})
