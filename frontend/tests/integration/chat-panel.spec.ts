/** The chat panel, mounted over the real store and client with only the network replaced. */

import { readFileSync } from 'node:fs'

import { mount } from '@vue/test-utils'
import type { VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ChatPanel from '../../src/components/ChatPanel.vue'
import { setLocale } from '../../src/i18n'
import { messages as en } from '../../src/i18n/messages/en'
import { messages as ja } from '../../src/i18n/messages/ja'
import { useChat } from '../../src/stores/chat'
import { useWorkspace } from '../../src/stores/workspace'
import type { Snapshot, TableSummary } from '../../src/api/types'

const STATS = {
  domains: 2,
  tables: 3,
  columns: 7,
  relationships: 2,
  lineageEdges: 1,
  sourceTables: 1,
  conformed: 1,
  filesParsed: 5,
  filesSkipped: 0,
  diagnostics: 0,
}

const snapshot: Snapshot = {
  id: 's1',
  name: 'snap',
  sourceLabel: 'docs',
  createdAt: '2026-01-01T00:00:00Z',
  stats: STATS,
}

const tables: TableSummary[] = [
  { id: 'domain_one/dim_alpha', name: 'dim_alpha', domainId: 'domain_one', kind: 'dimension', grain: '', conformed: false, columnCount: 2, description: '' },
  { id: 'domain_one/fact_primary', name: 'fact_primary', domainId: 'domain_one', kind: 'fact', grain: '', conformed: false, columnCount: 3, description: '' },
]

function answer(content: string, citations: string[] = []) {
  return {
    conversationId: 'c1',
    userMessage: { ordinal: 1, role: 'user', content: 'q', citations: [], createdAt: '' },
    assistantMessage: { ordinal: 2, role: 'assistant', content, citations, createdAt: '' },
    toolCalls: [{ name: 'list_tables', args: {} }],
    truncated: false,
    latencyMs: 12,
    model: 'test',
  }
}

/** What the next POST .../turn answers with. Replaced per test. */
let turn: { status: number; body?: unknown; reject?: boolean; hold?: boolean } = {
  status: 200,
  body: answer('an answer'),
}
let release: (() => void) | null = null
let turns = 0
let asked: string[] = []

function stubNetwork() {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const path = new URL(String(input), 'http://frontend').pathname

      if (path === '/api/chat/conversations' && init?.method === 'POST') {
        return json(201, { id: 'c1', snapshotId: 's1', title: '', createdAt: '', updatedAt: '' })
      }

      if (path.endsWith('/turn')) {
        turns += 1
        asked.push(JSON.parse(String(init?.body)).question)
        const question = asked[asked.length - 1]
        if (turn.hold) await new Promise<void>((r) => (release = r))
        if (turn.reject) throw new TypeError('Failed to fetch')
        return json(turn.status, echo(turn.body, question))
      }

      return json(404, { error: 'not found' })
    }),
  )
}

/** Puts the question back in the stored user message, as the service does. */
function echo(body: unknown, question: string): unknown {
  if (body && typeof body === 'object' && 'userMessage' in body) {
    const b = body as { userMessage: { content: string } }
    return { ...b, userMessage: { ...b.userMessage, content: question } }
  }
  return body
}

function json(status: number, body: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: `status ${status}`,
    json: async () => body,
  } as unknown as Response
}

/** An open panel over a loaded workspace. */
function panel(withTables = true) {
  const workspace = useWorkspace()
  workspace.snapshot = snapshot
  workspace.tables = withTables ? tables : []
  const chat = useChat()
  chat.open = true
  return { workspace, chat, w: mount(ChatPanel) }
}

/** Sends through the composer, the way a reader does. */
async function type(w: VueWrapper, text: string, shift = false) {
  const box = w.find('textarea')
  await box.setValue(text)
  await box.trigger('keydown', { key: 'Enter', shiftKey: shift })
  await flush()
}

async function flush() {
  await new Promise((r) => setTimeout(r, 0))
  await nextTick()
}

beforeEach(() => {
  setActivePinia(createPinia())
  turn = { status: 200, body: answer('an answer') }
  release = null
  turns = 0
  asked = []
  stubNetwork()
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  setLocale('en')
})

describe('the empty state', () => {
  it('offers three suggestions, one naming a table from the workspace', () => {
    const { w } = panel()
    expect(w.text()).toContain(en['chat.emptyTitle'])
    const suggestions = w.findAll('.suggestion')
    expect(suggestions).toHaveLength(3)
    expect(suggestions[2].text()).toContain('fact_primary')
  })

  it('drops the table suggestion when the workspace has no tables', () => {
    const { w } = panel(false)
    expect(w.findAll('.suggestion')).toHaveLength(2)
  })

  it('sends a suggestion when it is clicked', async () => {
    const { w } = panel()
    await w.findAll('.suggestion')[0].trigger('click')
    await flush()

    expect(asked[0]).toBe(en['chat.suggestion.diagnostics'])
    expect(w.find('.answer').text()).toContain('an answer')
  })
})

describe('the composer', () => {
  it('sends on Enter', async () => {
    const { w } = panel()
    await type(w, 'which tables are facts?')

    expect(asked).toEqual(['which tables are facts?'])
    expect(w.find('.answer').exists()).toBe(true)
  })

  it('does not send on Shift+Enter, and keeps the draft', async () => {
    const { w } = panel()
    await type(w, 'first line', true)

    expect(turns).toBe(0)
    expect(w.find('textarea').element.value).toBe('first line')
  })

  it('is disabled while a turn is running and enabled again after', async () => {
    turn = { status: 200, body: answer('an answer'), hold: true }
    const { w } = panel()

    void type(w, 'why?')
    await flush()
    expect(w.find('textarea').attributes('disabled')).toBeDefined()
    expect(w.find('.thinking').exists()).toBe(true)

    release?.()
    await flush()
    expect(w.find('textarea').attributes('disabled')).toBeUndefined()
    expect(w.find('.thinking').exists()).toBe(false)
  })
})

describe('citations', () => {
  it('renders one chip per ID, labelled by the bare name', async () => {
    turn = { status: 200, body: answer('see these', ['domain_one/fact_primary', 'domain_one/dim_alpha']) }
    const { w } = panel()
    await type(w, 'which?')

    const chips = w.findAll('.cite')
    expect(chips).toHaveLength(2)
    expect(chips[0].text()).toBe('fact_primary')
    expect(chips[0].attributes('title')).toContain('domain_one/fact_primary')
  })

  it('focuses the canvas on the full ID, not the bare name', async () => {
    // The bare name would fail silently for every conformed table, which is
    // exactly the case worth asserting.
    turn = { status: 200, body: answer('see this', ['domain_one/fact_primary']) }
    const { workspace, w } = panel()
    const focusOn = vi.spyOn(workspace, 'focusOn').mockResolvedValue(undefined)

    await type(w, 'which?')
    await w.find('.cite').trigger('click')

    expect(focusOn).toHaveBeenCalledWith('domain_one/fact_primary')
  })
})

describe('rendering an answer', () => {
  it('renders markdown through the renderer', async () => {
    turn = { status: 200, body: answer('a **bold** claim') }
    const { w } = panel()
    await type(w, 'why?')

    expect(w.find('.answer').html()).toContain('<strong>bold</strong>')
  })

  it('escapes a script tag and mounts no script element', async () => {
    // Duplicates a 06.4 assertion on purpose: this proves the component routes
    // content through the renderer rather than binding it raw.
    turn = { status: 200, body: answer('quoting <script>alert(1)</script> back') }
    const { w } = panel()
    await type(w, 'echo it')

    expect(w.find('.answer').html()).not.toContain('<script')
    expect(w.element.querySelectorAll('script')).toHaveLength(0)
    expect(w.find('.answer').text()).toContain('<script>alert(1)</script>')
  })

  it('renders the reader\'s own words as text, never as markup', async () => {
    const { w } = panel()
    await type(w, '<script>alert(1)</script>')

    expect(w.element.querySelectorAll('script')).toHaveLength(0)
    expect(w.find('.bubble').text()).toContain('<script>alert(1)</script>')
  })
})

describe('failure', () => {
  it('keeps the question on screen with a retry button', async () => {
    turn = { status: 500, body: { error: 'boom' } }
    const { w } = panel()
    await type(w, 'why?')

    expect(w.find('.bubble').text()).toBe('why?')
    expect(w.find('.banner--error').exists()).toBe(true)
    expect(w.find('.banner--error button').exists()).toBe(true)
  })

  it('re-sends the same question on retry', async () => {
    turn = { status: 500, body: { error: 'boom' } }
    const { w } = panel()
    await type(w, 'why?')

    turn = { status: 200, body: answer('an answer') }
    await w.find('.banner--error button').trigger('click')
    await flush()

    expect(asked).toEqual(['why?', 'why?'])
    expect(w.find('.answer').exists()).toBe(true)
    expect(w.findAll('.bubble')).toHaveLength(1)
  })

  it('shows the busy message on a 429', async () => {
    turn = { status: 429, body: { detail: 'too many turns in flight' } }
    const { w } = panel()
    await type(w, 'why?')

    expect(w.find('.banner--error').text()).toContain(en['chat.error.busy'])
  })

  it('shows the unavailable message when the service cannot be reached', async () => {
    turn = { status: 0, reject: true }
    const { w } = panel()
    await type(w, 'why?')

    expect(w.find('.banner--error').text()).toContain(en['chat.error.unavailable'])
  })

  it('never leaves the composer stuck', async () => {
    for (const failure of [
      { status: 500, body: { error: 'boom' } },
      { status: 429, body: { detail: 'busy' } },
      { status: 0, reject: true },
    ]) {
      setActivePinia(createPinia())
      turn = failure
      const { w } = panel()
      await type(w, 'why?')

      expect(w.find('textarea').attributes('disabled')).toBeUndefined()
    }
  })
})

describe('accessibility', () => {
  it('marks the transcript as a polite log', () => {
    const { w } = panel()
    const log = w.find('[role="log"]')
    expect(log.exists()).toBe(true)
    expect(log.attributes('aria-live')).toBe('polite')
  })

  it('closes on Escape', async () => {
    const { chat, w } = panel()
    await w.find('[role="region"]').trigger('keydown.esc')
    expect(chat.open).toBe(false)
  })

  it('puts every click handler on a real button', () => {
    // Asserted against the source: a handler on a div renders the same but is
    // unreachable by keyboard, and the DOM alone cannot show the difference.
    const src = readFileSync('src/components/ChatPanel.vue', 'utf8')
    const template = src.slice(src.indexOf('<template>'), src.indexOf('</template>'))
    const tags = [...template.matchAll(/<(\w+)[^>]*?@click/gs)].map((m) => m[1])
    expect(tags.length).toBeGreaterThan(0)
    expect(tags.filter((tag) => tag !== 'button')).toEqual([])
  })

  it('gives every citation chip an accessible name carrying the full ID', async () => {
    turn = { status: 200, body: answer('see this', ['domain_one/fact_primary']) }
    const { w } = panel()
    await type(w, 'which?')

    expect(w.find('.cite').attributes('aria-label')).toContain('domain_one/fact_primary')
  })
})

describe('render cost', () => {
  it('does not re-parse mounted answers while the reader types', async () => {
    // Bound in the template, every keystroke re-parsed every answer in the
    // thread; the count is what keeps that from coming back.
    const markdown = await import('../../src/chat/markdown')
    const render = vi.spyOn(markdown, 'renderAnswer')
    const { w } = panel()

    await type(w, 'first?')
    await type(w, 'second?')
    const parsed = render.mock.calls.length

    const box = w.find('textarea')
    for (const ch of 'hello') await box.setValue(box.element.value + ch)

    expect(render.mock.calls.length).toBe(parsed)
  })
})

describe('language', () => {
  it('re-renders its strings when the locale changes', async () => {
    const { w } = panel()
    expect(w.text()).toContain(en['chat.emptyTitle'])

    setLocale('ja')
    await nextTick()

    expect(w.text()).toContain(ja['chat.emptyTitle'])
  })

  it('renders an error raised before the change in the new language', async () => {
    // What storing a catalogue key rather than prose buys: a banner can sit on
    // screen indefinitely and must follow the language around it.
    turn = { status: 429, body: { detail: 'busy' } }
    const { w } = panel()
    await type(w, 'why?')
    expect(w.find('.banner--error').text()).toContain(en['chat.error.busy'])

    setLocale('ja')
    await nextTick()

    expect(w.find('.banner--error').text()).toContain(ja['chat.error.busy'])
  })
})
