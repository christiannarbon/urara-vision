/** The diff page: default versions, summary, grouping, filter and swap. */

import { flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { DiffResult, Snapshot } from '../../src/api/types'

vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return {
    ...actual,
    api: {
      features: vi.fn(),
      listVersions: vi.fn(),
      listProjects: vi.fn(),
      diff: vi.fn(),
    },
  }
})

const { api, ApiError } = await import('../../src/api/client')
const { mountApp } = await import('../helpers/mountApp')

function snap(version: string): Snapshot {
  return {
    id: `p@${version}`,
    name: 'p',
    sourceLabel: 'docs',
    createdAt: '2026-01-01T00:00:00Z',
    stats: {
      domains: 1, tables: 1, columns: 1, relationships: 0, lineageEdges: 0,
      sourceTables: 0, conformed: 0, filesParsed: 1, filesSkipped: 0, diagnostics: 0,
    },
    projectId: 'p-id',
    projectSlug: 'p',
    project: {
      project: { name: 'Shop', version, description: '' },
      internationalization: { primary: 'EN', supported: ['EN'], type: 'inline' },
    },
  }
}

const zero = { added: 0, removed: 0, changed: 0 }

function emptyResult(from: string, to: string): DiffResult {
  return {
    project: 'p',
    from: { version: from, snapshotId: `p@${from}` },
    to: { version: to, snapshotId: `p@${to}` },
    summary: { domains: zero, tables: zero, columns: zero, relationships: zero, lineage: zero },
    domains: [],
    tables: [],
    relationships: [],
    lineage: [],
  }
}

function busyResult(from: string, to: string): DiffResult {
  return {
    ...emptyResult(from, to),
    summary: {
      domains: zero,
      tables: { added: 1, removed: 1, changed: 1 },
      columns: { added: 0, removed: 0, changed: 1 },
      relationships: { added: 1, removed: 0, changed: 0 },
      lineage: { added: 0, removed: 1, changed: 0 },
    },
    tables: [
      { id: 'catalog/fact_restocks', domainId: 'catalog', change: 'added', fields: [], columns: [] },
      { id: 'sales/dim_promotions', domainId: 'sales', change: 'removed', fields: [], columns: [] },
      {
        id: 'sales/fact_orders', domainId: 'sales', change: 'changed', fields: [],
        columns: [{ name: 'amount', change: 'changed', fields: [{ field: 'type', from: 'INT64', to: 'NUMERIC' }] }],
      },
    ],
    relationships: [
      {
        fromTableId: 'catalog/fact_restocks', toTableId: 'catalog/dim_products', targetRef: 'dim_products',
        fromColumn: 'product_id', toColumn: 'product_id', change: 'added', fields: [],
      },
    ],
    lineage: [
      {
        tableId: 'sales/fact_orders', column: 'amount', sourceTable: 'shop.refunds',
        sourceColumn: 'amount', change: 'removed', fields: [],
      },
    ],
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.mocked(api.features).mockResolvedValue({ chat: { available: false, enabled: false } })
  vi.mocked(api.listProjects).mockResolvedValue({ projects: [] })
  vi.mocked(api.listVersions).mockResolvedValue({ versions: ['3.0.0', '2.0.0', '1.0.0'].map(snap) })
  vi.mocked(api.diff).mockImplementation(async (_s, from, to) => busyResult(from, to))
})

describe('the diff page', () => {
  it('replaces a missing query with the two newest versions', async () => {
    const w = await mountApp('/projects/p/diff')
    await flushPromises()
    expect(w.vm.$route.query).toEqual({ from: '2.0.0', to: '3.0.0' })
    expect(api.diff).toHaveBeenLastCalledWith('p', '2.0.0', '3.0.0')
    expect(w.text()).toContain('Shop')
  })

  it('says so when there is only one version', async () => {
    vi.mocked(api.listVersions).mockResolvedValue({ versions: [snap('1.0.0')] })
    const w = await mountApp('/projects/p/diff')
    await flushPromises()
    expect(w.text()).toContain('Only one version; nothing to compare.')
    expect(api.diff).not.toHaveBeenCalled()
  })

  it('shows "No differences." for a zero summary', async () => {
    vi.mocked(api.diff).mockImplementation(async (_s, from, to) => emptyResult(from, to))
    const w = await mountApp('/projects/p/diff?from=1.0.0&to=1.0.0')
    await flushPromises()
    expect(w.text()).toContain('No differences.')
  })

  it('groups changes by domain, and the filter hides the other kinds', async () => {
    const w = await mountApp('/projects/p/diff?from=1.0.0&to=2.0.0')
    await flushPromises()

    const group = (d: string) => w.find(`[data-domain="${d}"]`)
    expect(group('catalog').text()).toContain('catalog/fact_restocks')
    expect(group('catalog').text()).toContain('catalog/fact_restocks.product_id → catalog/dim_products.product_id')
    expect(group('sales').text()).toContain('sales/dim_promotions')
    expect(group('sales').text()).toContain('type: INT64 → NUMERIC')
    expect(group('sales').text()).toContain('shop.refunds.amount')

    const chip = w.findAll('.fchip').find((b) => b.text() === 'Removed')!
    await chip.trigger('click')
    expect(w.findAll('[data-change]').every((e) => e.attributes('data-change') === 'removed')).toBe(true)
    expect(w.text()).toContain('sales/dim_promotions')
    expect(w.text()).not.toContain('catalog/fact_restocks')
    expect(group('catalog').exists()).toBe(false)
  })

  it('an unknown project shows a translated message', async () => {
    vi.mocked(api.listVersions).mockRejectedValue(new ApiError('project not found', 404))
    const w = await mountApp('/projects/p/diff')
    await flushPromises()
    expect(w.text()).toContain('There is no project called “p”.')
  })

  it('an unknown version shows a translated message without a request', async () => {
    const w = await mountApp('/projects/p/diff?from=1.0.0&to=9.9.9')
    await flushPromises()
    expect(w.text()).toContain('This project has no version “9.9.9”.')
    expect(api.diff).not.toHaveBeenCalled()
  })

  it('shows booleans as yes/no and expands descriptions by button', async () => {
    vi.mocked(api.diff).mockImplementation(async (_s, from, to) => ({
      ...emptyResult(from, to),
      summary: { ...emptyResult(from, to).summary, tables: { added: 0, removed: 0, changed: 1 } },
      tables: [{
        id: 'sales/dim_x', domainId: 'sales', change: 'changed', columns: [],
        fields: [
          { field: 'description', from: 'Old text.', to: 'New text.' },
          { field: 'conformed', from: false, to: true },
        ],
      }],
    }))
    const w = await mountApp('/projects/p/diff?from=1.0.0&to=2.0.0')
    await flushPromises()
    expect(w.text()).toContain('conformed: no → yes')

    const toggle = w.find('button.prose-toggle')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('true')
  })

  it('swap reverses the query', async () => {
    const w = await mountApp('/projects/p/diff?from=1.0.0&to=2.0.0')
    await flushPromises()
    await w.find('button.swap').trigger('click')
    await flushPromises()
    expect(w.vm.$route.query).toEqual({ from: '2.0.0', to: '1.0.0' })
    expect(api.diff).toHaveBeenLastCalledWith('p', '2.0.0', '1.0.0')
  })

  it('a picker change updates the query', async () => {
    const w = await mountApp('/projects/p/diff?from=1.0.0&to=2.0.0')
    await flushPromises()
    await w.findAll('select')[1].setValue('3.0.0')
    await flushPromises()
    expect(w.vm.$route.query).toEqual({ from: '1.0.0', to: '3.0.0' })
  })
})
