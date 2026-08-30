/**
 * The workspace store.
 *
 * This is where the UI's rules live: which filters trigger a refetch, what a
 * failed request leaves on screen, and when the reader gets interrupted about a
 * document that could not be parsed. The API client is mocked, so what is under
 * test is the store's own decisions.
 */

import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError } from '../../src/api/client'
import { setLocale } from '../../src/i18n'
import { documentText, setDocumentLanguages } from '../../src/i18n/content'
import { messages as en } from '../../src/i18n/messages/en'
import { messages as ja } from '../../src/i18n/messages/ja'
import type { Diagnostic, GraphData, Snapshot, TableSummary } from '../../src/api/types'

// Mocked before the store is imported, since the store closes over the module.
vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return {
    ...actual,
    api: {
      ingest: vi.fn(),
      listSnapshots: vi.fn(),
      getSnapshot: vi.fn(),
      deleteSnapshot: vi.fn(),
      domains: vi.fn(),
      tables: vi.fn(),
      table: vi.fn(),
      graph: vi.fn(),
      neighborhood: vi.fn(),
      paths: vi.fn(),
      lineage: vi.fn(),
      search: vi.fn(),
      diagnostics: vi.fn(),
      sources: vi.fn(),
    },
  }
})

const { api } = await import('../../src/api/client')
const { useWorkspace } = await import('../../src/stores/workspace')

const snapshot: Snapshot = {
  id: 's1',
  name: 'snap',
  sourceLabel: 'docs',
  createdAt: '2026-01-01T00:00:00Z',
  stats: {
    domains: 2,
    tables: 3,
    columns: 7,
    relationships: 2,
    lineageEdges: 1,
    sourceTables: 1,
    conformed: 1,
    filesParsed: 5,
    filesSkipped: 0,
    diagnostics: 1,
  },
}

const tables: TableSummary[] = [
  { id: 'domain_one/fact_primary', name: 'fact_primary', domainId: 'domain_one', kind: 'fact', grain: '', conformed: false, columnCount: 3, description: '' },
  { id: 'domain_one/dim_alpha', name: 'dim_alpha', domainId: 'domain_one', kind: 'dimension', grain: '', conformed: false, columnCount: 2, description: '' },
]

const emptyGraph: GraphData = { nodes: [], links: [] }

/** Points every read at a working, minimal snapshot. */
function stubHappyPath(diagnostics: Diagnostic[] = []) {
  vi.mocked(api.getSnapshot).mockResolvedValue(snapshot)
  vi.mocked(api.domains).mockResolvedValue({ domains: [] })
  vi.mocked(api.tables).mockResolvedValue({ tables })
  vi.mocked(api.diagnostics).mockResolvedValue({ diagnostics })
  vi.mocked(api.graph).mockResolvedValue(emptyGraph)
  vi.mocked(api.neighborhood).mockResolvedValue(emptyGraph)
  vi.mocked(api.listSnapshots).mockResolvedValue({ snapshots: [snapshot] })
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  // The document languages are module state, like the locale: a spec that
  // loads a snapshot leaves them set for the next one.
  setDocumentLanguages(null)
})

describe('loadSnapshot', () => {
  it("hands the documents' own languages to whatever renders them", async () => {
    stubHappyPath()
    vi.mocked(api.getSnapshot).mockResolvedValue({
      ...snapshot,
      project: {
        project: { name: 'p', version: '0.1.0', description: '' },
        internationalization: { primary: 'EN', supported: ['EN', 'JP'], type: 'inline' },
      },
    })

    const ws = useWorkspace()
    await ws.loadSnapshot('s1')

    // Nothing else in the app knows what a [JP] tag means until this happens.
    expect(documentText('This is a column [JP] これはコラムです。')).toBe('This is a column')
  })

  it('populates the model and fetches the graph', async () => {
    stubHappyPath()
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')

    expect(ws.hasSnapshot).toBe(true)
    expect(ws.snapshot?.id).toBe('s1')
    expect(ws.tables).toHaveLength(2)
    expect(api.graph).toHaveBeenCalledOnce()
    expect(ws.busy).toBe(false)
    expect(ws.error).toBeNull()
  })

  it('resets filters and selection, so a new snapshot starts clean', async () => {
    stubHappyPath()
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    await ws.toggleDomain('domain_one')
    await ws.setCrossDomainOnly(true)
    await ws.focusOn('domain_one/fact_primary')

    await ws.loadSnapshot('s1')

    expect(ws.activeDomains).toEqual([])
    expect(ws.activeKinds).toEqual([])
    expect(ws.crossDomainOnly).toBe(false)
    expect(ws.selectedId).toBeNull()
    expect(ws.detail).toBeNull()
    expect(ws.viewMode).toBe('overview')
  })

  it('surfaces a failure and clears busy', async () => {
    vi.mocked(api.getSnapshot).mockRejectedValue(new ApiError('snapshot not found', 404))
    const ws = useWorkspace()
    await ws.loadSnapshot('nope')

    expect(ws.error).toBe('snapshot not found')
    expect(ws.busy).toBe(false)
    expect(ws.hasSnapshot).toBe(false)
  })

  it('describes a non-Error failure rather than showing "undefined"', async () => {
    vi.mocked(api.getSnapshot).mockRejectedValue('a string was thrown')
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    expect(ws.error).toBe('Something went wrong.')
  })
})

describe('what the banners say', () => {
  afterEach(() => setLocale('en'))

  it('keeps the server\'s own wording, which it cannot translate', async () => {
    vi.mocked(api.getSnapshot).mockRejectedValue(new ApiError('snapshot not found', 404))
    const ws = useWorkspace()
    await ws.loadSnapshot('nope')
    setLocale('ja')
    expect(ws.error).toBe('snapshot not found')
  })

  it('translates a failure the client itself diagnosed', async () => {
    vi.mocked(api.getSnapshot).mockRejectedValue(
      new ApiError('unreachable', 0, 'error.unreachable'),
    )
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')

    expect(ws.error).toBe(en['error.unreachable'])
    setLocale('ja')
    // Resolved on read, so a banner already on screen follows the language.
    expect(ws.error).toBe(ja['error.unreachable'])
  })

  it('fills the status placeholder into a failed request', async () => {
    vi.mocked(api.getSnapshot).mockRejectedValue(new ApiError('', 503, 'error.requestFailed'))
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    expect(ws.error).toContain('503')
  })

  it('translates the ingest status, count and all', async () => {
    let seen = ''
    vi.mocked(api.ingest).mockImplementation(async () => {
      seen = useWorkspace().statusMessage
      throw new Error('stop here')
    })
    setLocale('ja')
    const ws = useWorkspace()
    await ws.ingest('n', 'l', [
      { path: 'a.md', content: '' },
      { path: 'b.md', content: '' },
    ])

    expect(seen).toBe(ja['status.parsing.other'].replace('{n}', '2'))
    expect(ws.statusMessage).toBe('')
  })
})

describe('diagnostics', () => {
  it('interrupts only for documents that were dropped', async () => {
    stubHappyPath([
      { severity: 'warning', code: 'empty_document', message: 'dropped' },
      { severity: 'error', code: 'unresolved_reference', message: 'finding' },
    ])
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')

    expect(ws.parseFailures).toHaveLength(1)
    expect(ws.findings).toHaveLength(1)
    expect(ws.hasDiagnostics).toBe(true)
    expect(ws.needsParseNotice).toBe(true)

    ws.acknowledgeParseFailures()
    expect(ws.needsParseNotice).toBe(false)
    // The diagnostic itself does not go away; only the interruption does.
    expect(ws.parseFailures).toHaveLength(1)
  })

  it('does not interrupt for findings alone', async () => {
    stubHappyPath([{ severity: 'error', code: 'unresolved_reference', message: 'finding' }])
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')

    expect(ws.needsParseNotice).toBe(false)
    expect(ws.hasDiagnostics).toBe(true)
  })

  it('re-arms the notice for the next snapshot', async () => {
    stubHappyPath([{ severity: 'warning', code: 'empty_document', message: 'dropped' }])
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    ws.acknowledgeParseFailures()
    expect(ws.needsParseNotice).toBe(false)

    await ws.loadSnapshot('s2')
    expect(ws.needsParseNotice).toBe(true)
  })
})

describe('filters', () => {
  it('toggles a domain on and off, refetching each time', async () => {
    stubHappyPath()
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    vi.mocked(api.graph).mockClear()

    await ws.toggleDomain('domain_one')
    expect(ws.activeDomains).toEqual(['domain_one'])
    await ws.toggleDomain('domain_two')
    expect(ws.activeDomains).toEqual(['domain_one', 'domain_two'])
    await ws.toggleDomain('domain_one')
    expect(ws.activeDomains).toEqual(['domain_two'])
    expect(api.graph).toHaveBeenCalledTimes(3)
  })

  it('sends the active filters as CSV', async () => {
    stubHappyPath()
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    await ws.toggleDomain('domain_one')
    await ws.toggleDomain('domain_two')
    await ws.toggleKind('fact')

    expect(api.graph).toHaveBeenLastCalledWith('s1', {
      domain: 'domain_one,domain_two',
      kind: 'fact',
      sources: false,
      crossDomainOnly: false,
    })
  })

  it('sends no filter at all when nothing is active', async () => {
    stubHappyPath()
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')

    expect(api.graph).toHaveBeenLastCalledWith('s1', {
      domain: undefined,
      kind: undefined,
      sources: false,
      crossDomainOnly: false,
    })
  })

  it('clears every filter at once', async () => {
    stubHappyPath()
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    await ws.toggleDomain('domain_one')
    await ws.toggleKind('fact')
    await ws.setCrossDomainOnly(true)

    await ws.clearFilters()
    expect(ws.activeDomains).toEqual([])
    expect(ws.activeKinds).toEqual([])
    expect(ws.crossDomainOnly).toBe(false)
  })

  it('leaves the focused view alone when a filter changes', async () => {
    // Filters describe the overview; in focus mode the graph is the
    // neighbourhood of one table, so a filter must not refetch it.
    stubHappyPath()
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    await ws.focusOn('domain_one/fact_primary')
    vi.mocked(api.graph).mockClear()
    vi.mocked(api.neighborhood).mockClear()

    await ws.toggleDomain('domain_one')
    expect(api.graph).not.toHaveBeenCalled()
    expect(api.neighborhood).not.toHaveBeenCalled()
  })

  it('forces the overview when cross-domain-only is switched on', async () => {
    // The cross-domain view is a property of the whole graph; it means nothing
    // inside one table's neighbourhood.
    stubHappyPath()
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    await ws.focusOn('domain_one/fact_primary')
    expect(ws.viewMode).toBe('focus')

    await ws.setCrossDomainOnly(true)
    expect(ws.viewMode).toBe('overview')
    expect(api.graph).toHaveBeenCalled()
  })
})

describe('view mode and focus', () => {
  it('fetches the neighbourhood in focus mode, at the chosen depth', async () => {
    stubHappyPath()
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    await ws.focusOn('domain_one/fact_primary')

    expect(ws.viewMode).toBe('focus')
    expect(api.neighborhood).toHaveBeenLastCalledWith('s1', 'domain_one/fact_primary', 1, false)

    await ws.setFocusDepth(3)
    expect(api.neighborhood).toHaveBeenLastCalledWith('s1', 'domain_one/fact_primary', 3, false)
  })

  it('does not refetch when the depth changes outside focus mode', async () => {
    stubHappyPath()
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    vi.mocked(api.graph).mockClear()

    await ws.setFocusDepth(2)
    expect(api.graph).not.toHaveBeenCalled()
    expect(ws.focusDepth).toBe(2)
  })

  it('falls back to the overview when focus mode has nothing selected', async () => {
    stubHappyPath()
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    await ws.setViewMode('focus')

    // Nothing is selected, so there is no neighbourhood to ask for.
    expect(api.neighborhood).not.toHaveBeenCalled()
    expect(api.graph).toHaveBeenCalled()
  })
})

describe('select', () => {
  it('loads the detail pane for a table', async () => {
    stubHappyPath()
    vi.mocked(api.table).mockResolvedValue({
      table: { ...tables[0], snapshotId: 's1', kindRaw: 'Fact', updateFrequency: '', layer: '', domainLabel: '', columns: [], columnLineage: [], relationships: [], notes: [], docPath: '' },
      incoming: [],
      upstream: [],
      siblings: [],
    } as never)

    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    await ws.select('domain_one/fact_primary')

    expect(ws.selectedId).toBe('domain_one/fact_primary')
    expect(ws.detail).not.toBeNull()
    expect(ws.detailLoading).toBe(false)
  })

  it('keeps a source node selected but asks for no detail', async () => {
    // Source models are graph nodes with no table document behind them, so a
    // request would 404.
    stubHappyPath()
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    await ws.select('warehouse.upstream_model')

    expect(ws.selectedId).toBe('warehouse.upstream_model')
    expect(ws.detail).toBeNull()
    expect(api.table).not.toHaveBeenCalled()
  })

  it('clears the detail pane when the selection is cleared', async () => {
    stubHappyPath()
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    await ws.select(null)

    expect(ws.selectedId).toBeNull()
    expect(ws.detail).toBeNull()
  })

  it('empties the pane on a failed detail request rather than showing stale data', async () => {
    stubHappyPath()
    vi.mocked(api.table).mockRejectedValue(new ApiError('not found', 404))
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    await ws.select('domain_one/fact_primary')

    expect(ws.detail).toBeNull()
    expect(ws.error).toBe('not found')
    expect(ws.detailLoading).toBe(false)
  })
})

describe('removeSnapshot', () => {
  it('clears the workspace when the open snapshot is deleted', async () => {
    stubHappyPath()
    vi.mocked(api.deleteSnapshot).mockResolvedValue(undefined)
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')

    await ws.removeSnapshot('s1')

    expect(ws.hasSnapshot).toBe(false)
    expect(ws.tables).toEqual([])
    expect(ws.graph).toEqual({ nodes: [], links: [] })
    expect(ws.selectedId).toBeNull()
    expect(api.listSnapshots).toHaveBeenCalled()
  })

  it('leaves the workspace alone when a different snapshot is deleted', async () => {
    stubHappyPath()
    vi.mocked(api.deleteSnapshot).mockResolvedValue(undefined)
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')

    await ws.removeSnapshot('s2')

    expect(ws.hasSnapshot).toBe(true)
    expect(ws.tables).toHaveLength(2)
  })
})

describe('ingest', () => {
  it('loads the snapshot it just created', async () => {
    stubHappyPath()
    vi.mocked(api.ingest).mockResolvedValue({ snapshot, edges: 2, diagnostics: [] } as never)

    const ws = useWorkspace()
    const res = await ws.ingest('name', 'label', [{ path: 'd/x.md', content: '# x' }])

    expect(res).not.toBeNull()
    expect(ws.snapshot?.id).toBe('s1')
    expect(ws.busy).toBe(false)
    expect(ws.statusMessage).toBe('')
  })

  it('returns null and reports the error on failure', async () => {
    vi.mocked(api.ingest).mockRejectedValue(new ApiError('too many files', 400))
    const ws = useWorkspace()
    const res = await ws.ingest('name', 'label', [])

    expect(res).toBeNull()
    expect(ws.error).toBe('too many files')
    expect(ws.busy).toBe(false)
  })
})

describe('lookups', () => {
  it('indexes domains and tables by ID', async () => {
    stubHappyPath()
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')

    expect(ws.tableById.get('domain_one/fact_primary')?.name).toBe('fact_primary')
    expect(ws.tableById.has('nope')).toBe(false)
  })
})

describe('dismissError', () => {
  it('clears the banner', async () => {
    vi.mocked(api.getSnapshot).mockRejectedValue(new ApiError('boom', 500))
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    expect(ws.error).toBe('boom')

    ws.dismissError()
    expect(ws.error).toBeNull()
  })
})
