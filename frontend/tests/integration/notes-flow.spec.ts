/** Notes end to end: add, reply, resolve, and a version switch, over a stateful fake API. */

import { flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { NewNote, Note, NoteCount, Project, Snapshot, TableResponse } from '../../src/api/types'
import { messages as en } from '../../src/i18n/messages/en'

vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return {
    ...actual,
    api: {
      features: vi.fn(),
      listProjects: vi.fn(),
      listVersions: vi.fn(),
      getVersion: vi.fn(),
      getSnapshot: vi.fn(),
      domains: vi.fn(),
      tables: vi.fn(),
      diagnostics: vi.fn(),
      graph: vi.fn(),
      table: vi.fn(),
      noteCounts: vi.fn(),
      listNotes: vi.fn(),
      createNote: vi.fn(),
      updateNote: vi.fn(),
      deleteNote: vi.fn(),
    },
  }
})

const { api } = await import('../../src/api/client')
const { mountApp } = await import('../helpers/mountApp')
const { useNotes } = await import('../../src/stores/notes')

const TABLE = 'sales/fact_order'

const STATS = {
  domains: 1, tables: 1, columns: 1, relationships: 0, lineageEdges: 0,
  sourceTables: 0, conformed: 0, filesParsed: 1, filesSkipped: 0, diagnostics: 0,
}

function snap(version: string): Snapshot {
  return {
    id: `p@${version}`,
    name: 'p',
    sourceLabel: 'docs',
    createdAt: '2026-01-01T00:00:00Z',
    stats: STATS,
    projectId: 'p-id',
    projectSlug: 'p',
    project: {
      project: { name: 'p', version, description: '' },
      internationalization: { primary: 'EN', supported: ['EN'], type: 'inline' },
    },
  }
}

const PROJECT: Project = {
  id: 'p-id', slug: 'p', name: 'p', description: '',
  createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z', versionCount: 2,
  latest: { snapshotId: 'p@2.0.0', version: '2.0.0', createdAt: '2026-01-01T00:00:00Z' },
}

const DETAIL: TableResponse = {
  table: {
    id: TABLE, name: 'fact_order', kind: 'fact', kindRaw: 'Fact', domainId: 'sales', domainLabel: 'Sales',
    description: '', grain: '', layer: '', updateFrequency: '', docPath: '', conformed: false,
    columns: [{ name: 'order_id', type: 'int', description: '', ordinal: 0, isPk: true, isFk: false }],
    relationships: [], columnLineage: [], notes: [],
  },
  incoming: [], upstream: [], siblings: [],
}

// A tiny notes backend: one store per snapshot, counts derived from it.
let store: Note[] = []
let seq = 0

function fakeNotesApi() {
  vi.mocked(api.noteCounts).mockImplementation(async (sid) => {
    const counts = new Map<string, NoteCount>()
    for (const n of store.filter((n) => n.snapshotId === sid && !n.parentId)) {
      const key = `${n.anchorKind}:${n.anchorId}`
      const c = counts.get(key) ?? { anchorKind: n.anchorKind, anchorId: n.anchorId, open: 0, resolved: 0 }
      if (n.resolvedAt) c.resolved++
      else c.open++
      counts.set(key, c)
    }
    return { counts: [...counts.values()] }
  })
  vi.mocked(api.listNotes).mockImplementation(async (sid, kind, id) => ({
    notes: store
      .filter((n) => n.snapshotId === sid && !n.parentId && n.anchorKind === kind && n.anchorId === id)
      .map((n) => ({ ...n, replies: store.filter((r) => r.parentId === n.id) })),
  }))
  vi.mocked(api.createNote).mockImplementation(async (sid, body: NewNote) => {
    const parent = store.find((n) => n.id === body.parentId)
    const n: Note = {
      id: `n${++seq}`, snapshotId: sid, parentId: body.parentId,
      anchorKind: parent?.anchorKind ?? body.anchorKind, anchorId: parent?.anchorId ?? body.anchorId,
      body: body.body, authorId: 'u-admin', authorName: 'Admin',
      createdAt: '2026-09-01T00:00:00Z', updatedAt: '2026-09-01T00:00:00Z',
    }
    store.push(n)
    return n
  })
  vi.mocked(api.updateNote).mockImplementation(async (id, patch) => {
    const n = store.find((x) => x.id === id)!
    if ('resolved' in patch) n.resolvedAt = patch.resolved ? '2026-09-02T00:00:00Z' : undefined
    else n.body = patch.body
    return { ...n }
  })
}

const mounted: VueWrapper[] = []

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  store = []
  seq = 0
  vi.mocked(api.features).mockResolvedValue({ chat: { available: false, enabled: false } })
  vi.mocked(api.listProjects).mockResolvedValue({ projects: [PROJECT] })
  vi.mocked(api.listVersions).mockResolvedValue({ versions: ['2.0.0', '1.0.0'].map(snap) })
  vi.mocked(api.getVersion).mockImplementation(async (_s, v) => snap(v === 'latest' ? '2.0.0' : v))
  vi.mocked(api.getSnapshot).mockImplementation(async (sid) => snap(sid.split('@')[1]))
  vi.mocked(api.domains).mockResolvedValue({ domains: [] })
  vi.mocked(api.tables).mockResolvedValue({
    tables: [{ id: TABLE, name: 'fact_order', domainId: 'sales', kind: 'fact', grain: '', conformed: false, columnCount: 1, description: '' }],
  })
  vi.mocked(api.diagnostics).mockResolvedValue({ diagnostics: [] })
  vi.mocked(api.graph).mockResolvedValue({ nodes: [{ id: TABLE, label: 'fact_order', type: 'table', degree: 0 }], links: [] })
  vi.mocked(api.table).mockResolvedValue(DETAIL)
  fakeNotesApi()
})

afterEach(() => {
  while (mounted.length) mounted.pop()!.unmount()
})

const click = async (w: VueWrapper, text: string) => {
  await w.findAll('button').filter((b) => b.text() === text).at(-1)!.trigger('click')
  await flushPromises()
}

describe('the notes flow', () => {
  it('add, reply and resolve a column note; a version switch reloads', async () => {
    const w = await mountApp('/projects/p/versions/1.0.0', { attachTo: document.body })
    mounted.push(w)
    await flushPromises()
    expect(api.noteCounts).toHaveBeenLastCalledWith('p@1.0.0')

    const canvas = w.findComponent({ name: 'GraphCanvas' })
    canvas.vm.$emit('select', TABLE)
    await flushPromises()

    const marked = () => (canvas.props('notedTableIds') as Set<string>).has(TABLE)
    const dot = () => w.find('[role="tab"] .notes-dot').exists()

    await w.findAll('[role="tab"]').find((b) => b.text().startsWith(en['detail.tab.columns']))!.trigger('click')
    await w.find('.col .notes-button button').trigger('click')
    await flushPromises()

    await w.find('.thread form textarea').setValue('is this the grain?')
    await w.find('.thread form').trigger('submit')
    await flushPromises()
    expect(api.createNote).toHaveBeenCalledWith('p@1.0.0', { anchorKind: 'column', anchorId: `${TABLE}#order_id`, body: 'is this the grain?' })
    expect(marked()).toBe(true)
    expect(dot()).toBe(true)

    await click(w, en['notes.item.reply'])
    await w.find('.note .editor textarea').setValue('yes, one row per line')
    await click(w, en['notes.item.reply'])
    expect(store.filter((n) => n.parentId)).toHaveLength(1)
    expect(w.find('.note--reply').text()).toContain('yes, one row per line')

    await click(w, en['notes.item.resolve'])
    expect(marked()).toBe(false)
    expect(dot()).toBe(false)

    await w.vm.$router.push('/projects/p/versions/2.0.0')
    await flushPromises()
    expect(api.noteCounts).toHaveBeenLastCalledWith('p@2.0.0')
    expect(useNotes().threads.size).toBe(0)
    expect(w.find('.thread').exists()).toBe(false)
  })

  it('closes an open domain thread when the version changes', async () => {
    vi.mocked(api.domains).mockResolvedValue({
      domains: [{ id: 'sales', name: 'sales', title: 'Sales', description: '', docPath: '', tableCount: 1 }],
    })
    const w = await mountApp('/projects/p/versions/1.0.0', { attachTo: document.body })
    mounted.push(w)
    await flushPromises()
    await w.find('.domain-row .notes-button button').trigger('click')
    await flushPromises()
    expect(w.find('.thread').exists()).toBe(true)

    await w.vm.$router.push('/projects/p/versions/2.0.0')
    await flushPromises()
    expect(w.find('.thread').exists()).toBe(false)
  })
})
