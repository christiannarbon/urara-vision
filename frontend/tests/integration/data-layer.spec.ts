/**
 * The data layer end to end: the real store over the real API client, with only
 * the network replaced.
 *
 * The unit specs mock the client, so they prove the store's decisions but not
 * that the two agree on request shapes and response shapes. Here the client is
 * real, so a URL the backend would reject or a field name the store misreads
 * shows up.
 *
 * The stub answers the same routes the Go handlers do, which is what keeps the
 * two sides honest about the contract.
 */

import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useWorkspace } from '../../src/stores/workspace'

/** Every request the stub saw, for asserting on what the store asked for. */
let requested: string[] = []

const snapshot = {
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
    filesSkipped: 1,
    diagnostics: 2,
  },
}

const tables = [
  { id: 'domain_one/fact_primary', name: 'fact_primary', domainId: 'domain_one', kind: 'fact', grain: 'One row per thing.', conformed: false, columnCount: 3, description: '' },
  { id: 'domain_one/dim_alpha', name: 'dim_alpha', domainId: 'domain_one', kind: 'dimension', grain: '', conformed: false, columnCount: 2, description: '' },
  { id: 'domain_two/dim_beta', name: 'dim_beta', domainId: 'domain_two', kind: 'dimension', grain: '', conformed: true, columnCount: 2, description: '' },
]

const graph = {
  nodes: [
    { id: 'domain_one/fact_primary', label: 'fact_primary', type: 'table', domainId: 'domain_one', kind: 'fact', degree: 2 },
    { id: 'domain_one/dim_alpha', label: 'dim_alpha', type: 'table', domainId: 'domain_one', kind: 'dimension', degree: 1 },
    { id: 'domain_two/dim_beta', label: 'dim_beta', type: 'table', domainId: 'domain_two', kind: 'dimension', degree: 1 },
  ],
  links: [
    { id: 'e1', source: 'domain_one/fact_primary', target: 'domain_one/dim_alpha', type: 'joins', fromColumn: 'alpha_id', toColumn: 'alpha_id', cardinality: 'Many-to-one' },
    { id: 'e2', source: 'domain_one/fact_primary', target: 'domain_two/dim_beta', type: 'joins', fromColumn: 'beta_id', toColumn: 'beta_id', cardinality: 'Many-to-one', crossDomain: true },
  ],
}

const diagnostics = [
  { severity: 'warning', code: 'empty_document', message: 'Document is empty and was skipped.', docPath: 'domain_one/blank.md' },
  { severity: 'error', code: 'unresolved_reference', message: 'No document matched "dim_missing".', tableId: 'domain_one/fact_primary' },
]

/**
 * Answers the routes the Go router exposes. Anything else is a 404, so a store
 * that builds an unexpected URL fails rather than quietly getting a stub.
 */
function route(url: string): { status: number; body?: unknown } {
  const u = new URL(url, 'http://backend')
  const p = u.pathname
  requested.push(p + (u.search || ''))

  if (p === '/api/v1/ingest') return { status: 201, body: { snapshot, edges: 2, diagnostics } }
  if (p === '/api/v1/snapshots') return { status: 200, body: { snapshots: [snapshot] } }
  if (p === '/api/v1/snapshots/s1') return { status: 200, body: snapshot }
  if (p === '/api/v1/snapshots/s1/domains') {
    return {
      status: 200,
      body: {
        domains: [
          { id: 'domain_one', name: 'domain_one', title: 'Domain One', description: '', docPath: 'domain_one.md', tableCount: 2 },
          { id: 'domain_two', name: 'domain_two', title: 'Domain Two', description: '', docPath: 'domain_two.md', tableCount: 1 },
        ],
      },
    }
  }
  if (p === '/api/v1/snapshots/s1/tables') {
    const domain = u.searchParams.get('domain')
    return { status: 200, body: { tables: domain ? tables.filter((t) => t.domainId === domain) : tables } }
  }
  if (p === '/api/v1/snapshots/s1/diagnostics') return { status: 200, body: { diagnostics } }
  if (p === '/api/v1/snapshots/s1/graph') return { status: 200, body: graph }
  if (p === '/api/v1/snapshots/s1/neighborhood') {
    const table = u.searchParams.get('table')
    return {
      status: 200,
      body: {
        nodes: graph.nodes.filter((n) => n.id === table || n.id === 'domain_one/fact_primary'),
        links: [],
      },
    }
  }
  if (p === '/api/v1/snapshots/s1/table') {
    const id = u.searchParams.get('id')
    const summary = tables.find((t) => t.id === id)
    if (!summary) return { status: 404, body: { error: 'not found' } }
    return {
      status: 200,
      body: {
        table: { ...summary, snapshotId: 's1', kindRaw: 'Fact', updateFrequency: 'Daily', layer: '', domainLabel: 'Domain One', columns: [{ name: 'primary_id', type: 'STRING', description: '', ordinal: 0, isPk: true, isFk: false }], columnLineage: [], relationships: [], notes: [], docPath: 'domain_one/fact_primary.md' },
        incoming: [],
        upstream: [{ id: 'warehouse.upstream_model', label: 'upstream_model', dataset: 'warehouse', columns: ['primary_id'], columnCount: 1 }],
        siblings: [],
      },
    }
  }
  return { status: 404, body: { error: 'not found' } }
}

function stubNetwork() {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: string | URL | Request) => {
      const { status, body } = route(String(input))
      return {
        ok: status >= 200 && status < 300,
        status,
        statusText: `status ${status}`,
        json: async () => body,
      } as unknown as Response
    }),
  )
}

beforeEach(() => {
  setActivePinia(createPinia())
  requested = []
  stubNetwork()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('loading a snapshot through the real client', () => {
  it('populates the whole workspace from the API shapes', async () => {
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')

    expect(ws.error).toBeNull()
    expect(ws.snapshot?.stats.tables).toBe(3)
    expect(ws.domains).toHaveLength(2)
    expect(ws.tables).toHaveLength(3)
    expect(ws.graph.nodes).toHaveLength(3)
    expect(ws.graph.links).toHaveLength(2)
    expect(ws.diagnostics).toHaveLength(2)
  })

  it('fetches exactly the endpoints the UI needs, and no others', async () => {
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')

    expect(requested).toContain('/api/v1/snapshots/s1')
    expect(requested).toContain('/api/v1/snapshots/s1/domains')
    expect(requested).toContain('/api/v1/snapshots/s1/tables')
    expect(requested).toContain('/api/v1/snapshots/s1/diagnostics')
    // No unrouted request slipped through as a 404.
    expect(ws.error).toBeNull()
  })

  it('classifies the diagnostics the backend sent', async () => {
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')

    // One dropped document and one finding: the split has to come out of the
    // real code values, not a hand-written fixture of the frontend's own.
    expect(ws.parseFailures.map((d) => d.code)).toEqual(['empty_document'])
    expect(ws.findings.map((d) => d.code)).toEqual(['unresolved_reference'])
    expect(ws.needsParseNotice).toBe(true)
  })
})

describe('filtering through the real client', () => {
  it('sends the domain filter as the handler expects it', async () => {
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    requested = []

    await ws.toggleDomain('domain_one')
    await ws.setShowSources(true)
    await ws.setCrossDomainOnly(true)

    const graphCalls = requested.filter((r) => r.startsWith('/api/v1/snapshots/s1/graph'))
    expect(graphCalls.length).toBe(3)
    const last = new URL(graphCalls[graphCalls.length - 1], 'http://x')
    expect(last.searchParams.get('domain')).toBe('domain_one')
    expect(last.searchParams.get('sources')).toBe('true')
    expect(last.searchParams.get('crossDomainOnly')).toBe('true')
  })
})

describe('selecting a table through the real client', () => {
  it('loads the detail pane, including the lineage the graph store supplied', async () => {
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    await ws.select('domain_one/fact_primary')

    expect(ws.detail?.table.name).toBe('fact_primary')
    expect(ws.detail?.table.columns[0].isPk).toBe(true)
    expect(ws.detail?.upstream).toHaveLength(1)
  })

  it('sends a table ID with its slash escaped', async () => {
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    requested = []
    await ws.select('domain_one/fact_primary')

    const call = requested.find((r) => r.startsWith('/api/v1/snapshots/s1/table'))
    expect(call).toContain('id=domain_one%2Ffact_primary')
  })

  it('surfaces a 404 from the real error path', async () => {
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')
    // In the store's tableById map but absent from the stub's table route.
    ws.tables = [...ws.tables, { id: 'gone/table', name: 'table', domainId: 'gone', kind: 'unknown', grain: '', conformed: false, columnCount: 0, description: '' }]

    await ws.select('gone/table')
    expect(ws.detail).toBeNull()
    expect(ws.error).toBe('not found')
  })
})

describe('ingest through the real client', () => {
  it('posts the documents and loads the snapshot it created', async () => {
    const ws = useWorkspace()
    const res = await ws.ingest('name', 'docs', [
      { path: 'domain_one/fact_primary.md', content: '# fact_primary' },
    ])

    expect(res?.snapshot.id).toBe('s1')
    expect(requested[0]).toBe('/api/v1/ingest')
    expect(ws.snapshot?.id).toBe('s1')
    expect(ws.graph.nodes).toHaveLength(3)
  })
})

describe('an unreachable backend', () => {
  it('reports something the reader can act on', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new TypeError('Failed to fetch')
      }),
    )
    const ws = useWorkspace()
    await ws.loadSnapshot('s1')

    expect(ws.hasSnapshot).toBe(false)
    expect(ws.error).toMatch(/backend/i)
    expect(ws.busy).toBe(false)
  })
})
