/** The chat service's fetch wrapper. */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '../../src/api/client'
import { chatApi } from '../../src/api/chat'

/** The URL and init of every fetch call the client made. */
let calls: Array<{ url: string; init?: RequestInit }> = []

/**
 * Installs a fetch stub returning one canned response, in the style of the backend client's spec…
 */
function stubFetch(response: { status?: number; body?: unknown; reject?: boolean }) {
  const fn = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
    calls.push({ url: String(url), init })
    if (response.reject) throw new TypeError('Failed to fetch')

    const status = response.status ?? 200
    return {
      ok: status >= 200 && status < 300,
      status,
      statusText: `status ${status}`,
      json: async () => {
        if (response.body === undefined) throw new SyntaxError('not JSON')
        return response.body
      },
    } as unknown as Response
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

/** The headers of the nth call, whichever form the client passed them in. */
function headersOf(index: number): Headers {
  return new Headers(calls[index].init?.headers)
}

beforeEach(() => {
  calls = []
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.unstubAllEnvs()
})

describe('paths and methods', () => {
  it('creates a conversation with the snapshot in the body', async () => {
    stubFetch({ status: 201, body: { id: 'c1', snapshotId: 's1' } })
    await expect(chatApi.createConversation('s1')).resolves.toMatchObject({ id: 'c1' })

    expect(calls[0].url).toBe('/api/chat/conversations')
    expect(calls[0].init?.method).toBe('POST')
    expect(JSON.parse(String(calls[0].init?.body))).toEqual({ snapshotId: 's1' })
  })

  it('lists conversations by snapshot and unwraps the envelope', async () => {
    // The service answers {conversations: [...]}; callers want the array.
    stubFetch({ body: { conversations: [{ id: 'c1' }, { id: 'c2' }] } })
    await expect(chatApi.listConversations('latest')).resolves.toHaveLength(2)
    expect(calls[0].url).toBe('/api/chat/conversations?snapshot=latest')
  })

  it('fetches one conversation by id', async () => {
    stubFetch({ body: { id: 'c1', messages: [] } })
    await chatApi.getConversation('c1')
    expect(calls[0].url).toBe('/api/chat/conversations/c1')
    expect(calls[0].init?.method).toBeUndefined()
  })

  it('escapes an id that would otherwise change the path', async () => {
    stubFetch({ body: { id: 'x' } })
    await chatApi.getConversation('a/b')
    expect(calls[0].url).toBe('/api/chat/conversations/a%2Fb')
  })

  it('deletes a conversation', async () => {
    stubFetch({ status: 204 })
    await chatApi.deleteConversation('c1')
    expect(calls[0].url).toBe('/api/chat/conversations/c1')
    expect(calls[0].init?.method).toBe('DELETE')
  })

  it('posts a turn to the conversation', async () => {
    stubFetch({ body: { conversationId: 'c1' } })
    await chatApi.turn('c1', 'which tables are facts?', 'en')
    expect(calls[0].url).toBe('/api/chat/conversations/c1/turn')
    expect(calls[0].init?.method).toBe('POST')
  })
})

describe('what a turn sends', () => {
  it('posts the question and the language', async () => {
    stubFetch({ body: { conversationId: 'c1' } })
    await chatApi.turn('c1', 'which tables are facts?', 'ja')
    expect(JSON.parse(String(calls[0].init?.body))).toEqual({
      question: 'which tables are facts?',
      language: 'ja',
    })
  })

  it('sends no snapshot ID', async () => {
    // The thread pinned its snapshot at creation.
    stubFetch({ body: { conversationId: 'c1' } })
    await chatApi.turn('c1', 'anything', 'en')
    const body = JSON.parse(String(calls[0].init?.body))
    expect(body).not.toHaveProperty('snapshotId')
    expect(body).not.toHaveProperty('snapshot')
  })
})

describe('credentials', () => {
  it('sends no Authorization header on any request', async () => {
    // A token held for the backend must not reach a service that does not want
    // one; this is the assertion that keeps it inside its own boundary.
    const { setApiToken, clearApiToken } = await import('../../src/api/client')
    setApiToken('a-token-the-backend-uses')
    try {
      stubFetch({ status: 204 })
      await chatApi.deleteConversation('c1')

      stubFetch({ body: { conversations: [] } })
      await chatApi.listConversations('s1')

      stubFetch({ body: { conversationId: 'c1' } })
      await chatApi.turn('c1', 'q', 'en')

      expect(calls).toHaveLength(3)
      for (let i = 0; i < calls.length; i++) {
        expect(headersOf(i).has('Authorization')).toBe(false)
      }
    } finally {
      clearApiToken()
    }
  })
})

describe('failures', () => {
  it('throws an ApiError carrying the status on a 404', async () => {
    stubFetch({ status: 404, body: { error: 'not found' } })
    const err = await chatApi.getConversation('missing').catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(404)
  })

  it('gives a 429 its own catalogue key', async () => {
    // Rendered as "too many questions at once" rather than a generic failure,
    // and in whatever language is on screen when it is read.
    stubFetch({ status: 429, body: { detail: 'too many turns in flight; the limit is 4.' } })
    const err = await chatApi.turn('c1', 'q', 'en').catch((e: unknown) => e)
    expect(err).toMatchObject({ status: 429, key: 'error.chatBusy' })
  })

  it("carries the server's own message on a 502 and adds no key", async () => {
    stubFetch({ status: 502, body: { error: 'the model store is unavailable' } })
    const err = await chatApi.turn('c1', 'q', 'en').catch((e: unknown) => e)
    expect(err).toMatchObject({
      status: 502,
      message: 'the model store is unavailable',
      key: undefined,
    })
  })

  it('gives an unreachable service its own key and status 0', async () => {
    // The chat service is a separate container: it can be down while the rest
    // of the app is perfectly healthy, which makes this the likeliest error.
    stubFetch({ reject: true })
    const err = await chatApi.listConversations('s1').catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 0, key: 'error.chatUnreachable' })
  })

  it('resolves rather than throwing when a delete answers 204', async () => {
    stubFetch({ status: 204 })
    await expect(chatApi.deleteConversation('c1')).resolves.toBeUndefined()
  })
})

describe('configuration', () => {
  it('uses VITE_CHAT_BASE when one is set', async () => {
    // The base is read once at module load, so the override only takes effect
    // for a freshly imported copy of the module.
    vi.stubEnv('VITE_CHAT_BASE', '/elsewhere/chat')
    vi.resetModules()
    const { chatApi: overridden } = await import('../../src/api/chat')

    stubFetch({ body: { conversations: [] } })
    await overridden.listConversations('s1')
    expect(calls[0].url).toBe('/elsewhere/chat/conversations?snapshot=s1')
  })
})
