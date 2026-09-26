/** The diff page's state: one comparison between two versions of a project. */

import { defineStore } from 'pinia'
import { ref } from 'vue'

import { api, ApiError } from '../api/client'
import type { DiffResult } from '../api/types'

export const useDiff = defineStore('diff', () => {
  const result = ref<DiffResult | null>(null)
  const loading = ref(false)
  // Kept whole so the view can translate its key on a language change.
  const error = ref<ApiError | null>(null)

  // Bumped by every load; an answer for older params is dropped.
  let generation = 0

  async function load(slug: string, from: string, to: string) {
    const started = ++generation
    loading.value = true
    error.value = null
    try {
      const res = await api.diff(slug, from, to)
      if (started !== generation) return
      result.value = res
    } catch (e) {
      if (started !== generation) return
      result.value = null
      error.value = e instanceof ApiError ? e : new ApiError(String(e), 0, 'error.unknown')
    } finally {
      if (started === generation) loading.value = false
    }
  }

  function reset() {
    generation += 1
    result.value = null
    loading.value = false
    error.value = null
  }

  return { result, loading, error, load, reset }
})
