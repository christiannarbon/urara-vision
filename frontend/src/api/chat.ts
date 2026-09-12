/**
 * Thin fetch wrapper over the chat service.
 *
 * A separate module from `client.ts` rather than another section of it, for
 * two reasons that both have to hold: a different base path, and **no token**.
 * The chat service takes no credential of its own -- it holds the backend's
 * internally -- so sending the one this app holds would put a credential
 * outside the boundary it belongs in for nothing in return.
 *
 * `ApiError` is imported rather than redefined, so a caller can catch one class
 * whichever client threw it. The rule about its catalogue key is the one stated
 * there: a key when this client decided what went wrong, so the banner renders
 * in whatever language is on screen when it is read, and no key when the
 * message is the server's own prose.
 */

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

/** One thread. `messages` is populated when a single conversation is fetched
 *  and absent from a listing, which mirrors the service exactly -- including
 *  the consequence that an empty thread and an unfetched one look the same. */
export interface Conversation {
  id: string
  snapshotId: string
  title: string
  createdAt: string
  updatedAt: string
  messages?: ChatMessage[]
}

/** What one turn produced. Both messages come back as they were stored:
 *  ordinals are the database's to assign, and rendering what was sent instead
 *  of what was written eventually renders a turn that was never persisted. */
export interface TurnResult {
  conversationId: string
  userMessage: ChatMessage
  assistantMessage: ChatMessage
  toolCalls: { name: string; args: Record<string, unknown> }[]
  truncated: boolean
  latencyMs: number
  model: string
}

/**
 * One request, with no `Authorization` header and no timeout.
 *
 * No timeout on purpose. A turn is a model round trip plus several tool calls
 * and a minute is not unusual; nginx allows 180s for exactly that reason, and
 * an `AbortController` set shorter here would cancel answers that were about
 * to arrive -- the reader sees a failure for a turn that in fact succeeded and
 * was stored.
 */
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
      'error.chatUnreachable',
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
        'error.chatBusy',
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
  /** Start a thread about one snapshot. The reference goes through untouched:
   *  `latest` is the service's to resolve, and resolving it here would put a
   *  second opinion in the system about which snapshot a thread is pinned to. */
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

  /** Ask one question of an existing conversation.
   *
   *  **No snapshot ID is sent.** The thread pinned its snapshot when it was
   *  created; a field the caller could send is a field that can disagree with
   *  the thread, and the service refuses one outright rather than ignoring it. */
  turn(id: string, question: string, language: string): Promise<TurnResult> {
    return request<TurnResult>(`/conversations/${encodeURIComponent(id)}/turn`, {
      method: 'POST',
      headers: json,
      body: JSON.stringify({ question, language }),
    })
  },
}
