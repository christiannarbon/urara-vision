/** Reaching the diff from home and the workspace, then onto the graph and back. */

import { flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { DiffResult, Project, Snapshot } from '../../src/api/types'
import type { DiffMarks } from '../../src/graph/diff-marks'

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
      diff: vi.fn(),
    },
  }
})

const { api } = await import('../../src/api/client')
const { mountApp } = await import('../helpers/mountApp')

const STATS = {
  domains: 1, tables: 2, columns: 2, relationships: 1, lineageEdges: 0,
  sourceTables: 0, conformed: 0, filesParsed: 2, filesSkipped: 0, diagnostics: 0,
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
  id: 'p-id',
  slug: 'p',
  name: 'p',
  description: '',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  versionCount: 3,
  latest: { snapshotId: 'p@3.0.0', version: '3.0.0', createdAt: '2026-01-01T00:00:00Z' },
}

const zero = { added: 0, removed: 0, changed: 0 }

function result(from: string, to: string): DiffResult {
  return {
    project: 'p',
    from: { version: from, snapshotId: `p@${from}` },
    to: { version: to, snapshotId: `p@${to}` },
    summary: { domains: zero, tables: { added: 1, removed: 0, changed: 0 }, columns: zero, relationships: { added: 1, removed: 0, changed: 0 }, lineage: zero },
    domains: [],
    tables: [{ id: 'a/new', domainId: 'a', change: 'added', fields: [], columns: [] }],
    relationships: [
      { fromTableId: 'a/new', toTableId: 'a/dim', targetRef: 'dim', fromColumn: 'dim_id', toColumn: 'dim_id', change: 'added', fields: [] },
    ],
    lineage: [],
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.mocked(api.features).mockResolvedValue({ chat: { available: false, enabled: false } })
  vi.mocked(api.listProjects).mockResolvedValue({ projects: [PROJECT] })
  vi.mocked(api.listVersions).mockResolvedValue({ versions: ['3.0.0', '2.0.0', '1.0.0'].map(snap) })
  vi.mocked(api.getVersion).mockImplementation(async (_s: string, v: string) => snap(v === 'latest' ? '3.0.0' : v))
  vi.mocked(api.getSnapshot).mockImplementation(async (sid: string) => snap(sid.split('@')[1]))
  vi.mocked(api.domains).mockResolvedValue({ domains: [] })
  vi.mocked(api.tables).mockResolvedValue({ tables: [] })
  vi.mocked(api.diagnostics).mockResolvedValue({ diagnostics: [] })
  vi.mocked(api.graph).mockResolvedValue({
    nodes: [
      { id: 'a/new', label: 'new', type: 'table', degree: 1 },
      { id: 'a/dim', label: 'dim', type: 'table', degree: 1 },
    ],
    links: [{ id: 'e1', source: 'a/new', target: 'a/dim', type: 'joins', fromColumn: 'dim_id', toColumn: 'dim_id' }],
  })
  vi.mocked(api.diff).mockImplementation(async (_s, from, to) => result(from, to))
})

type App = Awaited<ReturnType<typeof mountApp>>

const marks = (w: App) => w.findComponent({ name: 'GraphCanvas' }).props('diffMarks') as DiffMarks | null | undefined

describe('the diff flow', () => {
  it('home → expand project → Compare opens the diff against the newest', async () => {
    const w = await mountApp('/')
    await flushPromises()
    await w.find('.recent button[aria-expanded]').trigger('click')
    await flushPromises()
    const compare = w.findAll('.version .compare')
    expect(compare).toHaveLength(2) // not on the newest row
    await compare[1].trigger('click')
    await flushPromises()
    expect(w.vm.$route.name).toBe('diff')
    expect(w.vm.$route.query).toEqual({ from: '1.0.0', to: '3.0.0' })
  })

  it('workspace Compare opens the diff against the next-older version', async () => {
    const w = await mountApp('/projects/p/versions/2.0.0')
    await flushPromises()
    await w.find('button.compare').trigger('click')
    await flushPromises()
    expect(w.vm.$route.name).toBe('diff')
    expect(w.vm.$route.query).toEqual({ from: '1.0.0', to: '2.0.0' })
  })

  it('workspace Compare on the oldest version compares with the newest', async () => {
    const w = await mountApp('/projects/p/versions/1.0.0')
    await flushPromises()
    await w.find('button.compare').trigger('click')
    await flushPromises()
    expect(w.vm.$route.query).toEqual({ from: '3.0.0', to: '1.0.0' })
  })

  it('Show on graph marks the canvas, and Clear removes the query and the marks', async () => {
    const w = await mountApp('/projects/p/diff?from=2.0.0&to=3.0.0')
    await flushPromises()
    await w.find('button.show-graph').trigger('click')
    await flushPromises()

    expect(w.vm.$route.name).toBe('version')
    expect(w.vm.$route.params).toEqual({ project: 'p', version: '3.0.0' })
    expect(w.vm.$route.query).toEqual({ diffFrom: '2.0.0' })
    expect(api.diff).toHaveBeenLastCalledWith('p', '2.0.0', '3.0.0')
    expect(marks(w)?.nodes).toEqual(new Map([['a/new', 'added']]))
    expect(marks(w)?.edges).toEqual(new Map([['e1', 'added']]))

    await w.find('.diff-legend button.clear').trigger('click')
    await flushPromises()
    expect(w.vm.$route.query).toEqual({})
    expect(marks(w)).toBeNull()
    expect(w.find('.diff-legend').exists()).toBe(false)
  })
})
