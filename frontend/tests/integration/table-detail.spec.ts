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
import { setDocumentLanguages } from '../../src/i18n/content'
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

afterEach(() => {
  setLocale('en')
  setDocumentLanguages(null)
})

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

/**
 * The document's own words, as opposed to the app's.
 *
 * The pane is where nearly all of them land, and the point of reading them out
 * of the field rather than off the ingest is that switching language redraws
 * what is already loaded. That is what these assert: same props, same
 * component instance, different language.
 */
describe('prose written in more than one language', () => {
  const bilingual = () =>
    detail({
      description: 'One row per order line. [JP] 受注明細ごとに 1 行。',
      grain: 'order line [JP] 受注明細',
      updateFrequency: 'Daily [JP] 毎日',
      notes: ['Excludes cancelled rows. [JP] キャンセル分を除きます。'],
      relationshipNote: 'Joins through the date dimension. [JP] 日付ディメンションを経由します。',
      columns: [
        {
          name: 'order_id',
          type: 'STRING',
          description: 'This is a column [JP] これはコラムです。',
          ordinal: 0,
          isPk: true,
          isFk: false,
        },
      ],
    })

  it('shows one language at a time, and follows the switch without remounting', async () => {
    setDocumentLanguages({
      project: { name: 'p', version: '0.1.0', description: '' },
      internationalization: { primary: 'EN', supported: ['EN', 'JP'], type: 'inline' },
    })

    const w = pane(bilingual())
    expect(w.text()).toContain('One row per order line.')
    expect(w.text()).not.toContain('受注明細ごとに 1 行。')
    expect(w.text()).not.toContain('[JP]')

    setLocale('ja')
    await nextTick()
    expect(w.text()).toContain('受注明細ごとに 1 行。')
    expect(w.text()).toContain('毎日')
    expect(w.text()).toContain('キャンセル分を除きます。')
    expect(w.text()).not.toContain('One row per order line.')
    expect(w.text()).not.toContain('[JP]')
  })

  it('leaves the field alone for a snapshot that declares no languages', () => {
    setDocumentLanguages(null)
    expect(pane(bilingual()).text()).toContain('[JP]')
  })
})
