/**
 * The entry screen, mounted, in both languages.
 *
 * The previous-ingest list is the one place a counted string reaches the
 * screen through a component, so it is where the .one/.other pair and the
 * locale's own agreement rule are worth asserting together.
 */

import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'

import WelcomeScreen from '../../src/components/WelcomeScreen.vue'
import type { Snapshot } from '../../src/api/types'
import { setLocale } from '../../src/i18n'
import { messages as en } from '../../src/i18n/messages/en'
import { messages as ja } from '../../src/i18n/messages/ja'

function snap(over: Partial<Snapshot['stats']> = {}): Snapshot {
  return {
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
      diagnostics: 0,
      ...over,
    },
  }
}

function screen(snapshots: Snapshot[] = []) {
  return mount(WelcomeScreen, { props: { snapshots, busy: false, statusMessage: '' } })
}

afterEach(() => setLocale('en'))

describe('the introduction', () => {
  it('reads from the catalogue rather than the template', () => {
    expect(screen().text()).toContain(en['welcome.title'])
  })

  it('follows a language change without remounting', async () => {
    const w = screen()
    setLocale('ja')
    await nextTick()
    expect(w.text()).toContain(ja['welcome.title'])
  })

  it('leaves the file extension it names outside the sentence', () => {
    expect(screen().find('code').text()).toBe('.md')
  })

  it('names the manifest the directory has to carry', () => {
    const codes = screen().findAll('code').map((c) => c.text())
    expect(codes).toContain('projectmeta.toml')
  })
})

describe('the previous-ingest list', () => {
  it('agrees in number in a language that marks plural', () => {
    const one = screen([snap({ tables: 1, domains: 1 })])
    expect(one.text()).toContain('1 table ·')
    expect(one.text()).toContain('1 domain ·')

    const many = screen([snap({ tables: 3, domains: 2 })])
    expect(many.text()).toContain('3 tables')
    expect(many.text()).toContain('2 domains')
  })

  it('reads the same for one and many in a language that does not', async () => {
    setLocale('ja')
    const one = screen([snap({ tables: 1, domains: 1 })])
    await nextTick()
    expect(one.text()).toContain('テーブル 1 件')

    const many = screen([snap({ tables: 3, domains: 2 })])
    await nextTick()
    expect(many.text()).toContain('テーブル 3 件')
  })

  it('names the project an ingest documented, where it declared one', () => {
    const withProject = snap()
    withProject.project = {
      project: { name: 'sample-project', version: '0.1.0', description: '' },
      internationalization: { primary: 'EN', supported: ['EN'], type: 'inline' },
    }
    expect(screen([withProject]).text()).toContain('sample-project 0.1.0')
  })

  it('says nothing about a project for an ingest older than the manifest', () => {
    const line = screen([snap()]).find('.snap .faint').text()
    expect(line.startsWith('3 tables')).toBe(true)
  })

  it('formats the timestamp in the conventions of the active language', async () => {
    const w = screen([snap()])
    const english = w.text()
    setLocale('ja')
    await nextTick()
    expect(w.text()).not.toBe(english)
    expect(w.text()).toContain(new Date('2026-01-01T00:00:00Z').toLocaleString('ja'))
  })
})
