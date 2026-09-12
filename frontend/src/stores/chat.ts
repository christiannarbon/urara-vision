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

  // Bumped by every reset. A turn that started under an older generation is
  // about a snapshot the reader has left, so its result is dropped.
  let generation = 0

  function reset() {
    generation += 1
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

    const started = generation
    const stale = () => generation !== started

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
      if (stale()) return
      // The snapshot went while the question was being typed; withdraw it
      // rather than leave it on screen with nothing to retry.
      if (!id) {
        messages.value = messages.value.slice(0, settled)
        return
      }
      const result = await chatApi.turn(id, text, activeLocale.value.toUpperCase())
      if (stale()) return
      messages.value = [
        ...messages.value.slice(0, settled),
        result.userMessage,
        result.assistantMessage,
      ]
      truncated.value = result.truncated
      lastToolCalls.value = result.toolCalls.map((c) => ({ name: c.name }))
    } catch (e) {
      if (stale()) return
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
  // new model would answer about the wrong thing. Compared against the last id
  // actually seen, not the previous watcher value: leaving through the picker
  // passes through null, and null is a gap rather than a new snapshot.
  let lastSnapshotId: string | null = workspace.snapshot?.id ?? null

  watch(
    () => workspace.snapshot?.id ?? null,
    (id) => {
      // Leaving the workspace clears the thread but announces nothing -- the
      // reader did it themselves. The id is kept, so returning to a different
      // snapshot still counts as a change.
      if (id === null) {
        if (lastSnapshotId !== null) reset()
        return
      }
      const changed = lastSnapshotId !== null && lastSnapshotId !== id
      lastSnapshotId = id
      if (!changed) return
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
