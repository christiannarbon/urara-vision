/** The chat store. */

import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '../../src/api/client'
import { setLocale } from '../../src/i18n'
import type { Conversation, TurnResult } from '../../src/api/chat'
import type { Snapshot, Stats } from '../../src/api/types'

// Mocked before the store is imported, since the store closes over the module.
vi.mock('../../src/api/chat', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/chat')>('../../src/api/chat')
  return {
    ...actual,
    chatApi: {
      createConversation: vi.fn(),
      listConversations: vi.fn(),
      getConversation: vi.fn(),
      deleteConversation: vi.fn(),
      turn: vi.fn(),
    },
  }
})

const { chatApi } = await import('../../src/api/chat')
const { useChat } = await import('../../src/stores/chat')
const { useWorkspace } = await import('../../src/stores/workspace')

const STATS: Stats = {
  domains: 1,
  tables: 2,
  columns: 4,
  relationships: 1,
  lineageEdges: 0,
  sourceTables: 0,
  conformed: 0,
  filesParsed: 2,
  filesSkipped: 0,
  diagnostics: 0,
}

function snapshotWithId(id: string): Snapshot {
  return { id, name: id, sourceLabel: 'docs', createdAt: '2026-01-01T00:00:00Z', stats: STATS }
}

function conversation(id = 'c1'): Conversation {
  return { id, snapshotId: 's1', title: '', createdAt: '', updatedAt: '' }
}

function turnResult(question: string, answer = 'because it is'): TurnResult {
  return {
    conversationId: 'c1',
    userMessage: { ordinal: 1, role: 'user', content: question, citations: [], createdAt: '' },
    assistantMessage: {
      ordinal: 2,
      role: 'assistant',
      content: answer,
      citations: ['sales/fct_orders'],
      createdAt: '',
    },
    toolCalls: [{ name: 'list_tables', args: {} }],
    truncated: false,
    latencyMs: 10,
    model: 'test',
  }
}

/** A store with a snapshot already loaded, as the panel only opens with one. */
function withSnapshot(id = 's1') {
  const workspace = useWorkspace()
  workspace.snapshot = snapshotWithId(id)
  return { workspace, chat: useChat() }
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.mocked(chatApi.createConversation).mockResolvedValue(conversation())
})

afterEach(() => {
  vi.clearAllMocks()
  setLocale('en')
})

describe('asking a question', () => {
  it('shows the question before the request resolves', async () => {
    const { chat } = withSnapshot()
    let release: (r: TurnResult) => void = () => {}
    vi.mocked(chatApi.turn).mockReturnValue(
      new Promise<TurnResult>((resolve) => {
        release = resolve
      }),
    )

    const inFlight = chat.ask('why?')
    await Promise.resolve()
    expect(chat.messages.map((m) => m.content)).toEqual(['why?'])
    expect(chat.pending).toBe(true)

    release(turnResult('why?'))
    await inFlight
  })

  it('replaces the optimistic message with the stored pair', async () => {
    const { chat } = withSnapshot()
    vi.mocked(chatApi.turn).mockResolvedValue(turnResult('why?'))

    await chat.ask('why?')

    expect(chat.messages).toHaveLength(2)
    expect(chat.messages.map((m) => m.role)).toEqual(['user', 'assistant'])
    expect(chat.messages.filter((m) => m.content === 'why?')).toHaveLength(1)
  })

  it('carries the tool calls and the truncation flag off the result', async () => {
    const { chat } = withSnapshot()
    vi.mocked(chatApi.turn).mockResolvedValue({ ...turnResult('why?'), truncated: true })

    await chat.ask('why?')

    expect(chat.truncated).toBe(true)
    expect(chat.lastToolCalls).toEqual([{ name: 'list_tables' }])
  })

  it('sends the active locale, upper-cased', async () => {
    const { chat } = withSnapshot()
    vi.mocked(chatApi.turn).mockResolvedValue(turnResult('なぜ'))
    setLocale('ja')

    await chat.ask('なぜ')

    expect(vi.mocked(chatApi.turn).mock.calls[0][2]).toBe('JA')
  })

  it('does nothing without a snapshot', async () => {
    const chat = useChat()
    await chat.ask('why?')
    expect(chat.messages).toEqual([])
    expect(chatApi.turn).not.toHaveBeenCalled()
  })

  it('does nothing while a turn is already in flight', async () => {
    const { chat } = withSnapshot()
    vi.mocked(chatApi.turn).mockReturnValue(new Promise<TurnResult>(() => {}))

    void chat.ask('first')
    await Promise.resolve()
    await chat.ask('second')

    expect(chatApi.turn).toHaveBeenCalledTimes(1)
  })

  it('does nothing for a question that is only whitespace', async () => {
    const { chat } = withSnapshot()
    await chat.ask('   ')
    expect(chat.messages).toEqual([])
  })
})

describe('the conversation', () => {
  it('is created once across two questions', async () => {
    const { chat } = withSnapshot()
    vi.mocked(chatApi.turn).mockResolvedValue(turnResult('why?'))

    await chat.ask('first')
    await chat.ask('second')

    expect(chatApi.createConversation).toHaveBeenCalledTimes(1)
    expect(chat.conversationId).toBe('c1')
  })

  it('is not created by opening the panel', () => {
    const { chat } = withSnapshot()
    chat.openPanel()
    expect(chat.open).toBe(true)
    expect(chatApi.createConversation).not.toHaveBeenCalled()
  })

  it('is not created by toggling the panel either', () => {
    const { chat } = withSnapshot()
    chat.togglePanel()
    chat.togglePanel()
    expect(chat.open).toBe(false)
    expect(chatApi.createConversation).not.toHaveBeenCalled()
  })
})

describe('failures', () => {
  it('keeps the question visible and retryable', async () => {
    const { chat } = withSnapshot()
    vi.mocked(chatApi.turn).mockRejectedValue(new ApiError('boom', 500))

    await chat.ask('why?')

    expect(chat.messages.map((m) => m.content)).toEqual(['why?'])
    expect(chat.failedQuestion).toBe('why?')
    expect(chat.pending).toBe(false)
  })

  it('clears pending even when the API throws', async () => {
    const { chat } = withSnapshot()
    vi.mocked(chatApi.createConversation).mockRejectedValue(new Error('network down'))

    await chat.ask('why?')

    expect(chat.pending).toBe(false)
  })

  const mapped: Array<[string, ApiError, string]> = [
    ['429', new ApiError('busy', 429, 'chat.error.busy'), 'chat.error.busy'],
    ['a network failure', new ApiError('offline', 0, 'chat.error.unavailable'), 'chat.error.unavailable'],
    ['400', new ApiError('too long', 400), 'chat.error.tooLong'],
  ]

  for (const [name, error, key] of mapped) {
    it(`maps ${name} to its own catalogue key`, async () => {
      const { chat } = withSnapshot()
      vi.mocked(chatApi.turn).mockRejectedValue(error)

      await chat.ask('why?')

      expect(chat.errorKey).toBe(key)
    })
  }

  it('never sets a key and a detail together', async () => {
    const cases = [
      new ApiError('busy', 429, 'chat.error.busy'),
      new ApiError('offline', 0, 'chat.error.unavailable'),
      new ApiError('too long', 400),
      new ApiError('the model store is unavailable', 502),
      new Error('something else'),
    ]
    for (const error of cases) {
      setActivePinia(createPinia())
      vi.mocked(chatApi.createConversation).mockResolvedValue(conversation())
      const { chat } = withSnapshot()
      vi.mocked(chatApi.turn).mockRejectedValue(error)

      await chat.ask('why?')

      expect(chat.errorKey === null || chat.errorDetail === null).toBe(true)
      expect(chat.errorKey ?? chat.errorDetail).not.toBeNull()
    }
  })

  it("keeps the server's own prose where it wrote some", async () => {
    const { chat } = withSnapshot()
    vi.mocked(chatApi.turn).mockRejectedValue(new ApiError('the model store is unavailable', 502))

    await chat.ask('why?')

    expect(chat.errorDetail).toBe('the model store is unavailable')
    expect(chat.errorKey).toBeNull()
  })

  it('falls back to the generic key for a non-ApiError', async () => {
    const { chat } = withSnapshot()
    vi.mocked(chatApi.turn).mockRejectedValue(new Error('something else'))

    await chat.ask('why?')

    expect(chat.errorKey).toBe('chat.error.generic')
  })
})

describe('a turn that outlives its snapshot', () => {
  it('drops its answer rather than writing it into the new thread', async () => {
    const { workspace, chat } = withSnapshot('s1')
    let release: (r: TurnResult) => void = () => {}
    vi.mocked(chatApi.turn).mockReturnValue(
      new Promise<TurnResult>((resolve) => {
        release = resolve
      }),
    )

    const inFlight = chat.ask('about s1?')
    await nextTick()
    workspace.snapshot = snapshotWithId('s2')
    await nextTick()
    expect(chat.messages).toEqual([])

    release(turnResult('about s1?'))
    await inFlight

    expect(chat.messages).toEqual([])
    expect(chat.conversationId).toBeNull()
    expect(chat.truncated).toBe(false)
    expect(chat.lastToolCalls).toEqual([])
    expect(chat.pending).toBe(false)
  })

  it('raises no error when it fails after the snapshot changed', async () => {
    const { workspace, chat } = withSnapshot('s1')
    let fail: (e: unknown) => void = () => {}
    vi.mocked(chatApi.turn).mockReturnValue(
      new Promise<TurnResult>((_resolve, reject) => {
        fail = reject
      }),
    )

    const inFlight = chat.ask('about s1?')
    await nextTick()
    workspace.snapshot = snapshotWithId('s2')
    await nextTick()

    fail(new ApiError('boom', 500))
    await inFlight

    expect(chat.errorKey).toBeNull()
    expect(chat.errorDetail).toBeNull()
    expect(chat.failedQuestion).toBeNull()
    expect(chat.pending).toBe(false)
  })

  it('withdraws the question when the snapshot goes mid-flight', async () => {
    const { workspace, chat } = withSnapshot('s1')
    let create: (c: Conversation) => void = () => {}
    vi.mocked(chatApi.createConversation).mockReturnValue(
      new Promise<Conversation>((resolve) => {
        create = resolve
      }),
    )

    const inFlight = chat.ask('why?')
    await nextTick()
    workspace.snapshot = null
    await nextTick()

    create(conversation())
    await inFlight

    expect(chat.messages).toEqual([])
    expect(chat.errorKey).toBeNull()
    expect(chat.pending).toBe(false)
  })
})

describe('retry', () => {
  it('re-sends the failed question and succeeds', async () => {
    const { chat } = withSnapshot()
    vi.mocked(chatApi.turn).mockRejectedValueOnce(new ApiError('boom', 500))

    await chat.ask('why?')
    expect(chat.failedQuestion).toBe('why?')

    vi.mocked(chatApi.turn).mockResolvedValue(turnResult('why?'))
    await chat.retry()

    expect(chat.messages).toHaveLength(2)
    expect(chat.messages.filter((m) => m.content === 'why?')).toHaveLength(1)
    expect(chat.failedQuestion).toBeNull()
    expect(chat.errorKey).toBeNull()
  })

  it('does nothing when nothing failed', async () => {
    const { chat } = withSnapshot()
    await chat.retry()
    expect(chatApi.turn).not.toHaveBeenCalled()
  })
})

describe('a snapshot change', () => {
  it('resets the thread and raises the notice', async () => {
    const { workspace, chat } = withSnapshot('s1')
    vi.mocked(chatApi.turn).mockResolvedValue(turnResult('why?'))
    await chat.ask('why?')
    expect(chat.messages).toHaveLength(2)

    workspace.snapshot = snapshotWithId('s2')
    await Promise.resolve()

    expect(chat.messages).toEqual([])
    expect(chat.conversationId).toBeNull()
    expect(chat.notice).toBe('chat.snapshotChanged')
  })

  it('does not fire when the id is unchanged', async () => {
    const { workspace, chat } = withSnapshot('s1')
    vi.mocked(chatApi.turn).mockResolvedValue(turnResult('why?'))
    await chat.ask('why?')

    workspace.snapshot = snapshotWithId('s1')
    await Promise.resolve()

    expect(chat.messages).toHaveLength(2)
    expect(chat.notice).toBeNull()
  })

  it('does not fire on the first load', async () => {
    const workspace = useWorkspace()
    const chat = useChat()

    workspace.snapshot = snapshotWithId('s1')
    await Promise.resolve()

    expect(chat.notice).toBeNull()
  })

  it('resets and notices when the picker is used to switch snapshots', async () => {
    // The common path: "Back to your ingests" clears the snapshot on the way
    // out, so the change arrives as s1 -> null -> s2.
    const { workspace, chat } = withSnapshot('s1')
    vi.mocked(chatApi.turn).mockResolvedValue(turnResult('why?'))
    await chat.ask('why?')

    workspace.snapshot = null
    await nextTick()
    workspace.snapshot = snapshotWithId('s2')
    await nextTick()

    expect(chat.messages).toEqual([])
    expect(chat.conversationId).toBeNull()
    expect(chat.notice).toBe('chat.snapshotChanged')
  })

  it('clears the thread without a notice when the snapshot goes away', async () => {
    const { workspace, chat } = withSnapshot('s1')
    vi.mocked(chatApi.turn).mockResolvedValue(turnResult('why?'))
    await chat.ask('why?')

    workspace.snapshot = null
    await nextTick()

    expect(chat.messages).toEqual([])
    expect(chat.notice).toBeNull()
  })

  it('does not notice when the same snapshot is reopened through the picker', async () => {
    const { workspace, chat } = withSnapshot('s1')
    workspace.snapshot = null
    await nextTick()
    workspace.snapshot = snapshotWithId('s1')
    await nextTick()

    expect(chat.notice).toBeNull()
  })

  it('is dismissable', async () => {
    const { workspace, chat } = withSnapshot('s1')
    workspace.snapshot = snapshotWithId('s2')
    await Promise.resolve()
    expect(chat.notice).toBe('chat.snapshotChanged')

    chat.dismissNotice()
    expect(chat.notice).toBeNull()
  })
})
