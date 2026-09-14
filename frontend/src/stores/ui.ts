/** Overlay and side-panel state shared by the topbar and the workspace view. */

import { defineStore } from 'pinia'
import { ref } from 'vue'

import { useChat } from './chat'

export const useUi = defineStore('ui', () => {
  const chat = useChat()
  const searchOpen = ref(false)
  const diagnosticsOpen = ref(false)

  // One right-hand pane, so the two panels take turns rather than adding a third
  // column: a third would leave the canvas a sliver on a laptop. Symmetric, or the
  // hidden one's button reports itself open while nothing changes.
  function toggleChat() {
    if (!chat.open) diagnosticsOpen.value = false
    chat.togglePanel()
  }

  function toggleDiagnostics() {
    if (!diagnosticsOpen.value) chat.closePanel()
    diagnosticsOpen.value = !diagnosticsOpen.value
  }

  function showDiagnostics() {
    chat.closePanel()
    diagnosticsOpen.value = true
  }

  return { searchOpen, diagnosticsOpen, toggleChat, toggleDiagnostics, showDiagnostics }
})
