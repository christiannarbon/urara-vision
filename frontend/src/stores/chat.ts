/** Chat panel state: the transcript, the turn in flight, and what went wrong. */

import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

import { ApiError } from '../api/client'
import { chatApi } from '../api/chat'
import { activeLocale } from '../i18n'
import { useWorkspace } from './workspace'
import type { ChatMessage } from '../api/chat'
import type { MessageKey } from '../i18n'

export const useChat = defineStore('chat', () => {
  const workspace = useWorkspace()

  const conversationId = ref<string | null>(null)
  const messages = ref<ChatMessage[]>([])
  const pending = ref(false)
  const open = ref(false)
  const lastToolCalls = ref<{ name: string }[]>([])
  const truncated = ref(false)

  // A key where this app decided what went wrong, the server's prose where it
  // did not, never both -- an error can sit on screen across a language change.
  const errorKey = ref<MessageKey | null>(null)
  const errorDetail = ref<string | null>(null)

  /** The question whose turn failed, kept so it can be retried without retyping. */
  const failedQuestion = ref<string | null>(null)

  /** Shown once after the loaded snapshot changes, then dismissed. */
  const notice = ref<MessageKey | null>(null)

  function clearError() {
    errorKey.value = null
    errorDetail.value = null
  }

  function dismissNotice() {
    notice.value = null
  }

  function openPanel() {
    open.value = true
  }

  function closePanel() {
    open.value = false
  }

  function togglePanel() {
    open.value = !open.value
  }

  function reset() {
    conversationId.value = null
    messages.value = []
    pending.value = false
    lastToolCalls.value = []
    truncated.value = false
    failedQuestion.value = null
    clearError()
  }

  /** Creates the thread on the first question, not when the panel opens: an
   *  empty conversation per open is litter nobody notices until there are
   *  hundreds. */
  async function ensureConversation(): Promise<string | null> {
    if (conversationId.value) return conversationId.value
    const snapshotId = workspace.snapshot?.id
    if (!snapshotId) return null
    const created = await chatApi.createConversation(snapshotId)
    conversationId.value = created.id
    return created.id
  }

  function recordFailure(e: unknown) {
    clearError()
    if (e instanceof ApiError) {
      if (e.status === 429) errorKey.value = 'chat.error.busy'
      else if (e.status === 0) errorKey.value = 'chat.error.unavailable'
      else if (e.status === 400) errorKey.value = 'chat.error.tooLong'
      else if (!e.key && e.message) errorDetail.value = e.message
      else errorKey.value = 'chat.error.generic'
      return
    }
    errorKey.value = 'chat.error.generic'
  }

  async function ask(question: string) {
    const text = question.trim()
    if (!text || pending.value || !workspace.snapshot?.id) return

    // Appended before the request so the question appears instantly, and left
    // in place on failure so it can be retried without retyping. Remembered by
    // position, not identity: reading it back gives a reactive proxy.
    const settled = messages.value.length
    messages.value = [
      ...messages.value,
      {
        ordinal: settled,
        role: 'user',
        content: text,
        citations: [],
        createdAt: new Date().toISOString(),
      },
    ]

    pending.value = true
    clearError()
    failedQuestion.value = null
    truncated.value = false

    try {
      const id = await ensureConversation()
      if (!id) return
      const result = await chatApi.turn(id, text, activeLocale.value.toUpperCase())
      messages.value = [
        ...messages.value.slice(0, settled),
        result.userMessage,
        result.assistantMessage,
      ]
      truncated.value = result.truncated
      lastToolCalls.value = result.toolCalls.map((c) => ({ name: c.name }))
    } catch (e) {
      failedQuestion.value = text
      recordFailure(e)
    } finally {
      // Always: a stuck spinner with no way out is the worst failure here.
      pending.value = false
    }
  }

  async function retry() {
    const question = failedQuestion.value
    if (!question) return
    // The failed attempt is the last thing in the transcript; drop it first.
    messages.value = messages.value.slice(0, -1)
    failedQuestion.value = null
    await ask(question)
  }

  // A thread is pinned to its snapshot server-side, so continuing one against a
  // new model would answer about the wrong thing.
  watch(
    () => workspace.snapshot?.id ?? null,
    (id, previous) => {
      if (previous == null || id == null || id === previous) return
      reset()
      notice.value = 'chat.snapshotChanged'
    },
  )

  return {
    conversationId,
    messages,
    pending,
    open,
    lastToolCalls,
    truncated,
    errorKey,
    errorDetail,
    failedQuestion,
    notice,
    openPanel,
    closePanel,
    togglePanel,
    ensureConversation,
    ask,
    retry,
    reset,
    clearError,
    dismissNotice,
  }
})
