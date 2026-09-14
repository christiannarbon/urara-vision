/** The entry screen, mounted, in both languages. */

import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'

import WelcomeScreen from '../../src/components/WelcomeScreen.vue'
import type { Project } from '../../src/api/types'
import { setLocale } from '../../src/i18n'
import { messages as en } from '../../src/i18n/messages/en'
import { messages as ja } from '../../src/i18n/messages/ja'

function project(over: Partial<Project> = {}): Project {
  return {
    id: 'p1',
    slug: 'jaffle',
    name: 'jaffle',
    description: 'Jaffle shop model',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    versionCount: 2,
    latest: { snapshotId: 's2', version: '0.2.0', createdAt: '2026-01-01T00:00:00Z' },
    ...over,
  }
}

function screen(projects: Project[] = []) {
  return mount(WelcomeScreen, { props: { projects, busy: false, statusMessage: '' } })
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

describe('the project list', () => {
  it('agrees in number in a language that marks plural', () => {
    expect(screen([project({ versionCount: 1 })]).text()).toContain('1 version ·')
    expect(screen([project({ versionCount: 2 })]).text()).toContain('2 versions ·')
  })

  it('reads the same for one and many in a language that does not', async () => {
    setLocale('ja')
    const one = screen([project({ versionCount: 1 })])
    await nextTick()
    expect(one.text()).toContain('バージョン 1 件')
  })

  it('shows the description and the latest version', () => {
    const text = screen([project()]).text()
    expect(text).toContain('Jaffle shop model')
    expect(text).toContain('latest 0.2.0')
  })

  it('leaves out the latest version for a project with none', () => {
    expect(screen([project({ latest: null })]).text()).not.toContain('latest')
  })

  it('emits the slug on open and delete', async () => {
    const w = screen([project()])
    await w.find('.snap').trigger('click')
    await w.find('button[aria-label="Delete project"]').trigger('click')
    expect(w.emitted('open')).toEqual([['jaffle']])
    expect(w.emitted('delete')).toEqual([['jaffle']])
  })

  it('formats the timestamp in the conventions of the active language', async () => {
    const w = screen([project()])
    const english = w.text()
    setLocale('ja')
    await nextTick()
    expect(w.text()).not.toBe(english)
    expect(w.text()).toContain(new Date('2026-01-01T00:00:00Z').toLocaleString('ja'))
  })
})
