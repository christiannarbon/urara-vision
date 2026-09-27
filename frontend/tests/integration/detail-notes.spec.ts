/** Notes buttons and tab dots in the table detail pane. */

import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { NoteCount, Snapshot, TableResponse } from '../../src/api/types'
import type { Anchor } from '../../src/notes/anchors'
import { messages as en } from '../../src/i18n/messages/en'

vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return { ...actual, api: { noteCounts: vi.fn(), listNotes: vi.fn() } }
})

const { api } = await import('../../src/api/client')
const { useAuth } = await import('../../src/stores/auth')
const { useWorkspace } = await import('../../src/stores/workspace')
const { anchorKey } = await import('../../src/notes/anchors')
const TableDetail = (await import('../../src/components/TableDetail.vue')).default
const NotesButton = (await import('../../src/components/notes/NotesButton.vue')).default

const ID = 'sales/fact_order'

const DETAIL: TableResponse = {
  table: {
    id: ID,
    name: 'fact_order',
    kind: 'fact',
    kindRaw: 'Fact',
    domainId: 'sales',
    domainLabel: 'Sales',
    description: '',
    grain: '',
    layer: '',
    updateFrequency: '',
    docPath: 'sales/fact_order.md',
    conformed: false,
    columns: [
      { name: 'order_id', type: 'int', description: '', ordinal: 0, isPk: true, isFk: false },
      { name: 'amount', type: 'numeric', description: '', ordinal: 1, isPk: false, isFk: false },
    ],
    relationships: [
      {
        id: 'rel-1',
        fromTableId: ID,
        toTableId: 'sales/dim_customer',
        targetRef: 'dim_customer',
        fromColumn: 'customer_id',
        toColumn: 'customer_id',
        joinKeyRaw: 'customer_id',
        cardinality: 'N:1',
        resolution: 'local',
      },
    ],
    columnLineage: [
      { column: 'amount', sourceTable: 'raw.orders', sourceColumn: 'gross', notes: '', derived: true },
      { column: 'amount', sourceTable: 'raw.refunds', sourceColumn: 'refund', notes: '', derived: true },
      { column: 'order_id', sourceTable: 'raw.orders', sourceColumn: 'id', notes: '', derived: false },
    ],
    notes: [],
  },
  incoming: [],
  upstream: [],
  siblings: [],
}

function count(a: Anchor, open: number, resolved = 0): NoteCount {
  return { anchorKind: a.kind, anchorId: a.id, open, resolved }
}

async function pane(counts: NoteCount[] = []) {
  vi.mocked(api.noteCounts).mockResolvedValue({ counts })
  const w = mount(TableDetail, { props: { detail: DETAIL, loading: false, selectedId: ID } })
  await flushPromises()
  return w
}

async function openTab(w: VueWrapper, label: string) {
  await w.findAll('[role="tab"]').find((b) => b.text().startsWith(label))!.trigger('click')
}

const anchors = (w: VueWrapper) => w.findAllComponents(NotesButton).map((b) => anchorKey(b.props('anchor')))

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.mocked(api.listNotes).mockResolvedValue({ notes: [] })
  useWorkspace().snapshot = { id: 's1' } as Snapshot
  const auth = useAuth()
  auth.kind = 'user'
  auth.permissions = ['project.view', 'note.write']
})

describe('detail pane notes', () => {
  it('puts a table button in the header', async () => {
    const w = await pane()
    expect(anchors(w)).toEqual([`table:${ID}`])
  })

  it('puts a column button on each column row', async () => {
    const w = await pane()
    await openTab(w, en['detail.tab.columns'])
    expect(anchors(w)).toContain(`column:${ID}#order_id`)
    expect(anchors(w)).toContain(`column:${ID}#amount`)
  })

  it('puts a relationship button on each join row', async () => {
    const w = await pane()
    await openTab(w, en['detail.tab.joins'])
    expect(anchors(w)).toContain('relationship:rel-1')
  })

  it('puts one lineage button per column, not per source row', async () => {
    const w = await pane()
    await openTab(w, en['detail.tab.lineage'])
    const lineage = anchors(w).filter((k) => k.startsWith('lineage:'))
    expect(lineage).toEqual([`lineage:${ID}#amount`, `lineage:${ID}#order_id`])
  })

  it('does not navigate when a row’s notes button is clicked', async () => {
    const w = await pane()
    await openTab(w, en['detail.tab.joins'])
    const button = w.findAllComponents(NotesButton).find((b) => b.props('anchor').kind === 'relationship')!
    await button.find('button').trigger('click')
    expect(w.emitted('navigate')).toBeUndefined()

    await w.find('.rel-target').trigger('click')
    expect(w.emitted('navigate')?.[0]).toEqual(['sales/dim_customer'])
  })

  it('shows a tab dot only where there are open notes', async () => {
    const w = await pane([
      count({ kind: 'column', id: `${ID}#amount` }, 1),
      count({ kind: 'relationship', id: 'rel-1' }, 0, 2),
    ])
    const dotOn = (label: string) =>
      w.findAll('[role="tab"]').find((b) => b.text().startsWith(label))!.find('.notes-dot').exists()
    expect(dotOn(en['detail.tab.columns'])).toBe(true)
    expect(dotOn(en['detail.tab.joins'])).toBe(false)
    expect(dotOn(en['detail.tab.lineage'])).toBe(false)
  })
})
