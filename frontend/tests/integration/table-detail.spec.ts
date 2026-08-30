/**
 * The detail pane, mounted, in both languages.
 *
 * This is the densest surface in the app -- five headings, a tab strip and two
 * counted strings -- and the one where a missed string would be least visible
 * in review. Mounting it is the only way to see them all resolve.
 */

import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'

import TableDetail from '../../src/components/TableDetail.vue'
import type { TableResponse } from '../../src/api/types'
import { setLocale } from '../../src/i18n'
import { messages as en } from '../../src/i18n/messages/en'
import { messages as ja } from '../../src/i18n/messages/ja'

function detail(over: Partial<TableResponse['table']> = {}): TableResponse {
  return {
    table: {
      id: 'sales/fact_order',
      name: 'fact_order',
      kind: 'fact',
      kindRaw: 'Fact',
      domainId: 'sales',
      domainLabel: 'Sales',
      description: 'One row per order line.',
      grain: 'order line',
      layer: '',
      updateFrequency: '',
      docPath: 'sales/fact_order.md',
      conformed: false,
      conformedIn: [],
      columns: [],
      relationships: [],
      columnLineage: [],
      relationshipNote: '',
      notes: [],
      ...over,
    },
    incoming: [],
    upstream: [],
    siblings: [],
  }
}

function pane(res: TableResponse | null, loading = false) {
  return mount(TableDetail, {
    props: { detail: res, loading, selectedId: res?.table.id ?? null },
  })
}

afterEach(() => setLocale('en'))

describe('the empty and loading states', () => {
  it('reads from the catalogue', () => {
    expect(pane(null).text()).toContain(en['detail.empty'])
    expect(pane(null, true).text()).toContain(en['detail.loading'])
  })

  it('follows a language change without remounting', async () => {
    const w = pane(null)
    setLocale('ja')
    await nextTick()
    expect(w.text()).toContain(ja['detail.empty'])
  })
})

describe('the tab strip', () => {
  it('names its tabs from the catalogue', () => {
    const labels = pane(detail())
      .findAll('[role="tab"]')
      .map((b) => b.text().split(/\s+/)[0])
    expect(labels).toEqual([
      en['detail.tab.overview'],
      en['detail.tab.columns'],
      en['detail.tab.joins'],
      en['detail.tab.lineage'],
    ])
  })

  it('translates them too', async () => {
    setLocale('ja')
    const w = pane(detail())
    await nextTick()
    expect(w.findAll('[role="tab"]')[2].text()).toContain(ja['detail.tab.joins'])
  })
})

describe('the overview', () => {
  it('labels the properties it shows, and only those', () => {
    const w = pane(detail())
    expect(w.text()).toContain(en['detail.grain'])
    expect(w.text()).toContain(en['detail.domain'])
    // Neither is set on this table, so neither row should be there to label.
    expect(w.text()).not.toContain(en['detail.layer'])
    expect(w.text()).not.toContain(en['detail.updated'])
  })

  it('names the role in the active language', async () => {
    setLocale('ja')
    const w = pane(detail())
    await nextTick()
    expect(w.find('.tag').text()).toBe(ja['role.fact'])
  })
})

describe('counted strings', () => {
  it('agrees in number in English', async () => {
    const w = pane({
      ...detail(),
      upstream: [{ id: 'u1', label: 'raw.orders', dataset: '', columnCount: 1, columns: ['a'] }],
    })
    await w.findAll('[role="tab"]')[3].trigger('click')
    expect(w.text()).toContain('1 col')
    expect(w.text()).not.toContain('1 cols')
  })

  it('reads the same for one and many in Japanese', async () => {
    setLocale('ja')
    const w = pane({
      ...detail(),
      upstream: [
        { id: 'u1', label: 'raw.orders', dataset: '', columnCount: 4, columns: ['a', 'b'] },
      ],
    })
    await w.findAll('[role="tab"]')[3].trigger('click')
    expect(w.text()).toContain(ja['detail.lineage.columns.other'].replace('{n}', '4'))
  })
})
