/** User administration. Mutations throw, so the caller can show the refusal where it happened. */

import { defineStore } from 'pinia'
import { ref } from 'vue'

import { api, ApiError } from '../api/client'
import type { NewUser, Role, User } from '../api/types'
import { translate } from '../i18n'

/** A catalogue message where the client chose one, otherwise the server's own words. */
export function describeError(e: unknown): string {
  if (e instanceof ApiError && e.key) return translate(e.key, { status: e.status })
  if (e instanceof Error && e.message) return e.message
  return translate('error.unknown')
}

export const useUsers = defineStore('users', () => {
  const users = ref<User[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function load() {
    loading.value = true
    error.value = null
    try {
      users.value = (await api.listUsers()).users
    } catch (e) {
      error.value = describeError(e)
    } finally {
      loading.value = false
    }
  }

  async function create(body: NewUser) {
    await api.createUser(body)
    await load()
  }

  async function update(id: string, body: { role?: Role; displayName?: string }) {
    await api.updateUser(id, body)
    await load()
  }

  async function resetPassword(id: string, password: string) {
    await api.resetPassword(id, password)
    await load()
  }

  async function remove(id: string) {
    await api.deleteUser(id)
    await load()
  }

  return { users, loading, error, load, create, update, resetPassword, remove }
})
