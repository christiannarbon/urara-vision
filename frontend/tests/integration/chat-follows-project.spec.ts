/** The chat's default context is the version on screen: the routed project's. */

import { flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { Conversation, TurnResult } from '../../src/api/chat'
import type { Project, Snapshot } from '../../src/api/types'

vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return {
    ...actual,
    api: {
      features: vi.fn(),
      listVersions: vi.fn(),
      getVersion: vi.fn(),
      getSnapshot: vi.fn(),
      domains: vi.fn(),
      tables: vi.fn(),
      diagnostics: vi.fn(),
      graph: vi.fn(),
      listProjects: vi.fn(),
    },
  }
})

vi.mock('../../src/api/chat', () => ({
  chatApi: { createConversation: vi.fn(), turn: vi.fn() },
}))

const { api } = await import('../../src/api/client')
const { chatApi } = await import('../../src/api/chat')
const { mountApp } = await import('../helpers/mountApp')
const { useChat } = await import('../../src/stores/chat')
const { messages: en } = await import('../../src/i18n/messages/en')

const STATS = {
  domains: 1,
  tables: 1,
  columns: 1,
  relationships: 0,
  lineageEdges: 0,
  sourceTables: 0,
  conformed: 0,
  filesParsed: 1,
  filesSkipped: 0,
  diagnostics: 0,
}

/** Project `slug`, whose newest version is snapshot `<slug>-v2`. */
function project(slug: string): Project {
  return {
    id: `${slug}-id`,
    slug,
    name: slug,
    description: '',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    versionCount: 2,
    latest: { snapshotId: `${slug}-v2`, version: '0.2.0', createdAt: '2026-01-01T00:00:00Z' },
  }
}

function snapshot(sid: string): Snapshot {
  const slug = sid.replace(/-v\d+$/, '')
  return {
    id: sid,
    name: slug,
    sourceLabel: 'docs',
    createdAt: '2026-01-01T00:00:00Z',
    stats: STATS,
    projectId: `${slug}-id`,
    projectSlug: slug,
  }
}

function conversation(snapshotId: string): Conversation {
  return { id: `c-${snapshotId}`, snapshotId, title: '', createdAt: '', updatedAt: '' }
}

function answer(content: string): TurnResult {
  return {
    conversationId: 'c1',
    userMessage: { ordinal: 1, role: 'user', content: 'q', citations: [], createdAt: '' },
    assistantMessage: { ordinal: 2, role: 'assistant', content, citations: [], createdAt: '' },
    toolCalls: [],
    truncated: false,
    latencyMs: 1,
    model: 'test',
  }
}

/** Opens the chat panel and sends a question, the way a reader does. */
async function ask(w: Awaited<ReturnType<typeof mountApp>>, question: string) {
  const chat = useChat()
  chat.open = true
  await flushPromises()
  const box = w.find('textarea')
  await box.setValue(question)
  await box.trigger('keydown', { key: 'Enter' })
  await flushPromises()
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.mocked(api.features).mockResolvedValue({ chat: { available: true, enabled: true } })
  vi.mocked(api.listVersions).mockImplementation(async (slug: string) => ({
    versions: [snapshot(`${slug}-v2`)],
  }))
  vi.mocked(api.getVersion).mockImplementation(async (slug: string) => snapshot(`${slug}-v2`))
  vi.mocked(api.getSnapshot).mockImplementation(async (sid: string) => snapshot(sid))
  vi.mocked(api.domains).mockResolvedValue({ domains: [] })
  vi.mocked(api.tables).mockResolvedValue({ tables: [] })
  vi.mocked(api.diagnostics).mockResolvedValue({ diagnostics: [] })
  vi.mocked(api.graph).mockResolvedValue({ nodes: [], links: [] })
  vi.mocked(api.listProjects).mockResolvedValue({ projects: [project('a'), project('b')] })
  vi.mocked(chatApi.createConversation).mockImplementation(async (sid: string) => conversation(sid))
  vi.mocked(chatApi.turn).mockResolvedValue(answer('an answer'))
})

describe('the thread the chat opens', () => {
  it('is pinned to the routed project\'s newest snapshot, never to "latest"', async () => {
    const w = await mountApp('/projects/a')
    await flushPromises()
    await ask(w, 'which tables are facts?')

    expect(chatApi.createConversation).toHaveBeenCalledTimes(1)
    expect(chatApi.createConversation).toHaveBeenCalledWith('a-v2')
  })
})

describe('changing project', () => {
  it('clears the transcript and says why', async () => {
    const w = await mountApp('/projects/a')
    await flushPromises()
    await ask(w, 'why?')
    expect(w.find('.answer').exists()).toBe(true)

    await w.vm.$router.push('/projects/b')
    await flushPromises()

    expect(w.find('.answer').exists()).toBe(false)
    expect(w.find('[role="status"]').text()).toContain(en['chat.snapshotChanged'])
    expect(useChat().conversationId).toBe(null)
  })

  it('opens the next thread against the new project', async () => {
    const w = await mountApp('/projects/a')
    await flushPromises()
    await ask(w, 'why?')

    await w.vm.$router.push('/projects/b')
    await flushPromises()
    await ask(w, 'and here?')

    expect(vi.mocked(chatApi.createConversation).mock.calls).toEqual([['a-v2'], ['b-v2']])
  })

  it('says nothing when the route leaves and returns to the same project', async () => {
    // Leaving passes the snapshot through null, which is a gap rather than a
    // different model to warn about.
    const w = await mountApp('/projects/a')
    await flushPromises()
    await ask(w, 'why?')

    await w.vm.$router.push('/')
    await flushPromises()
    await w.vm.$router.push('/projects/a')
    await flushPromises()

    expect(w.find('[role="status"]').exists()).toBe(false)
    expect(useChat().notice).toBe(null)
  })

  it('still says so when the route leaves and returns to a different project', async () => {
    // The other half of the null-gap rule: the gap is ignored, the id behind it
    // is not.
    const w = await mountApp('/projects/a')
    await flushPromises()
    await ask(w, 'why?')

    await w.vm.$router.push('/')
    await flushPromises()
    await w.vm.$router.push('/projects/b')
    await flushPromises()

    expect(w.find('[role="status"]').text()).toContain(en['chat.snapshotChanged'])
  })
})

describe('a turn still in flight', () => {
  it('drops its answer when the route changed under it', async () => {
    const held: { release?: () => void } = {}
    vi.mocked(chatApi.turn).mockImplementation(async () => {
      await new Promise<void>((r) => (held.release = r))
      return answer('an answer about project a')
    })

    const w = await mountApp('/projects/a')
    await flushPromises()
    void ask(w, 'why?')
    await flushPromises()

    await w.vm.$router.push('/projects/b')
    await flushPromises()
    held.release?.()
    await flushPromises()

    expect(w.text()).not.toContain('an answer about project a')
    expect(useChat().messages).toEqual([])
    expect(useChat().pending).toBe(false)
  })
})
