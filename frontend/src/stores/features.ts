/** Which optional features the server has on. The server owns the state. */

import { defineStore } from 'pinia'
import { ref } from 'vue'

import { api } from '../api/client'
import type { Features } from '../api/types'

export const useFeatures = defineStore('features', () => {
  const chatAvailable = ref(false)
  const chatEnabled = ref(false)
  const loaded = ref(false)

  function apply(f: Features) {
    chatAvailable.value = f.chat.available
    chatEnabled.value = f.chat.enabled
    loaded.value = true
  }

  /** A failure leaves chat hidden and says nothing: chat is optional. */
  async function load() {
    try {
      apply(await api.features())
    } catch {
      // Hidden is the safe default.
    }
  }

  async function setChatEnabled(v: boolean) {
    apply(await api.patchSettings({ chatEnabled: v }))
  }

  return { chatAvailable, chatEnabled, loaded, load, setChatEnabled }
})
