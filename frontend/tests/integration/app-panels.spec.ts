/** The right-hand pane: one panel at a time, and the buttons that say so. */

import type { VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return {
    ...actual,
    api: new Proxy(
      {},
      {
        get: (_, prop) =>
          prop === 'features'
            ? vi.fn().mockResolvedValue({ chat: { available: true, enabled: true } })
            : vi.fn().mockResolvedValue({ snapshots: [], projects: [] }),
      },
    ),
  }
})

const { mountApp } = await import('../helpers/mountApp')
const { useWorkspace } = await import('../../src/stores/workspace')
const { useChat } = await import('../../src/stores/chat')
const { useFeatures } = await import('../../src/stores/features')

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

beforeEach(() => setActivePinia(createPinia()))

// The snapshot already belongs to the routed project, so the view keeps it.
async function app() {
  const workspace = useWorkspace()
  workspace.snapshot = {
    id: 's1', name: 's1', sourceLabel: 'docs', createdAt: '', stats: STATS, projectId: 'p1', projectSlug: 's1',
  }
  const chat = useChat()
  // Set up front: the load on mount resolves after these specs click.
  useFeatures().chatEnabled = true
  return { workspace, chat, w: await mountApp('/projects/s1') }
}

const button = (w: VueWrapper, match: RegExp) =>
  w.findAll('.topbar button').find((b) => match.test(b.attributes('title') ?? ''))!

const chatButton = (w: VueWrapper) => button(w, /chat/i)
const diagButton = (w: VueWrapper) => button(w, /diagnostic/i)

describe('the chat button', () => {
  it('is absent until a snapshot is loaded', async () => {
    useWorkspace()
    const w = await mountApp()
    expect(w.findAll('.topbar button').find((b) => /chat/i.test(b.attributes('title') ?? ''))).toBeUndefined()
  })

  it('toggles the panel and reports its state', async () => {
    const { chat, w } = await app()
    expect(chatButton(w).attributes('aria-expanded')).toBe('false')

    await chatButton(w).trigger('click')
    expect(chat.open).toBe(true)
    expect(chatButton(w).attributes('aria-expanded')).toBe('true')

    await chatButton(w).trigger('click')
    expect(chat.open).toBe(false)
  })
})

describe('two panels, one pane', () => {
  it('closes diagnostics when chat opens', async () => {
    const { w } = await app()
    await diagButton(w).trigger('click')
    await chatButton(w).trigger('click')

    expect(w.findAll('.pane--right')).toHaveLength(1)
    expect(w.find('.composer').exists()).toBe(true)
    expect(diagButton(w).attributes('aria-expanded')).toBe('false')
  })

  it('closes chat when diagnostics opens', async () => {
    // The asymmetric version left this button reporting itself open while
    // nothing on screen changed.
    const { chat, w } = await app()
    await chatButton(w).trigger('click')
    await diagButton(w).trigger('click')

    expect(chat.open).toBe(false)
    expect(w.findAll('.pane--right')).toHaveLength(1)
    expect(w.find('.composer').exists()).toBe(false)
    expect(chatButton(w).attributes('aria-expanded')).toBe('false')
  })

  it('never reports a hidden panel as open', async () => {
    const { w } = await app()
    for (const press of [chatButton, diagButton, chatButton, diagButton]) {
      await press(w).trigger('click')
      const open = w.findAll('.topbar button').filter((b) => b.attributes('aria-expanded') === 'true')
      expect(open.length).toBeLessThanOrEqual(1)
      expect(w.findAll('.pane--right')).toHaveLength(1)
    }
  })
})

describe('layout', () => {
  it('leaves the workspace alone while both panels are closed', async () => {
    const { w } = await app()
    expect(w.find('main.workspace').classes()).not.toContain('workspace--wide')
  })

  it('widens it while chat is open', async () => {
    const { w } = await app()
    await chatButton(w).trigger('click')
    expect(w.find('main.workspace').classes()).toContain('workspace--wide')
  })
})

describe('escape', () => {
  it('closes chat before anything else', async () => {
    const { chat, w } = await app()
    await chatButton(w).trigger('click')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await w.vm.$nextTick()

    expect(chat.open).toBe(false)
  })
})
