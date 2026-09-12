/** Thin fetch wrapper over the chat service. */

import { ApiError } from './client'
import type { MessageKey } from '../i18n'

const CHAT_BASE = (import.meta.env.VITE_CHAT_BASE as string | undefined) ?? '/api/chat'

/** One stored turn, as the service returns it. */
export interface ChatMessage {
  ordinal: number
  role: 'user' | 'assistant'
  content: string
  citations: string[]
  createdAt: string
}

/** One thread. */
export interface Conversation {
  id: string
  snapshotId: string
  title: string
  createdAt: string
  updatedAt: string
  messages?: ChatMessage[]
}

/** What one turn produced. */
export interface TurnResult {
  conversationId: string
  userMessage: ChatMessage
  assistantMessage: ChatMessage
  toolCalls: { name: string; args: Record<string, unknown> }[]
  truncated: boolean
  latencyMs: number
  model: string
}

/** One request, with no `Authorization` header and no timeout. */
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await fetch(`${CHAT_BASE}${path}`, init)
  } catch {
    // The likeliest error a reader meets: the service is a separate container
    // and can be down while the rest of the app is perfectly healthy.
    throw new ApiError(
      'Cannot reach the chat service. Check that it is running and reachable.',
      0,
      'chat.error.unavailable',
    )
  }

  if (!res.ok) {
    // Both spellings: the service writes `error` from its own handlers and
    // `detail` where FastAPI wrote the response, and the 429 uses `detail`.
    let detail = res.statusText
    try {
      const body = await res.json()
      if (body && typeof body.error === 'string') detail = body.error
      else if (body && typeof body.detail === 'string') detail = body.detail
    } catch {
      // Not JSON; the status text is the best available message.
    }

    // Its own key, because "every slot is taken, try again shortly" is
    // something a reader can act on and a generic failure is not.
    if (res.status === 429) {
      throw new ApiError(
        detail || 'Too many questions are being answered at once.',
        429,
        'chat.error.busy',
      )
    }

    const key: MessageKey | undefined = detail ? undefined : 'error.requestFailed'
    throw new ApiError(detail || `Request failed with status ${res.status}.`, res.status, key)
  }

  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

const json = { 'Content-Type': 'application/json' }

export const chatApi = {
  /** Start a thread about one snapshot. */
  createConversation(snapshotId: string): Promise<Conversation> {
    return request<Conversation>('/conversations', {
      method: 'POST',
      headers: json,
      body: JSON.stringify({ snapshotId }),
    })
  },

  /** Threads about one snapshot, newest first and without their transcripts. */
  async listConversations(snapshotId: string): Promise<Conversation[]> {
    const body = await request<{ conversations: Conversation[] }>(
      `/conversations?snapshot=${encodeURIComponent(snapshotId)}`,
    )
    return body.conversations
  },

  /** One thread with its full transcript. */
  getConversation(id: string): Promise<Conversation> {
    return request<Conversation>(`/conversations/${encodeURIComponent(id)}`)
  },

  /** Remove a thread and its messages. Answers 204 with no body. */
  deleteConversation(id: string): Promise<void> {
    return request<void>(`/conversations/${encodeURIComponent(id)}`, { method: 'DELETE' })
  },

  /** Ask one question of an existing conversation. */
  turn(id: string, question: string, language: string): Promise<TurnResult> {
    return request<TurnResult>(`/conversations/${encodeURIComponent(id)}/turn`, {
      method: 'POST',
      headers: json,
      body: JSON.stringify({ question, language }),
    })
  },
}
