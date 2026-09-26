/** A project's versions on the home screen: expand, open, delete. */

import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Project, Snapshot } from '../../src/api/types'

vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return {
    ...actual,
    api: { listProjects: vi.fn(), listVersions: vi.fn(), deleteVersion: vi.fn() },
  }
})

const { api } = await import('../../src/api/client')
const { createAppRouter } = await import('../../src/router')
const HomeView = (await import('../../src/views/HomeView.vue')).default
const { messages: en } = await import('../../src/i18n/messages/en')

function project(slug: string, versionCount: number): Project {
  return {
    id: slug,
    slug,
    name: slug,
    description: '',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    versionCount,
    latest: { snapshotId: `${slug}-s`, version: '0.2.0', createdAt: '2026-01-01T00:00:00Z' },
  }
}

function version(slug: string, v: string): Snapshot {
  return {
    id: `${slug}@${v}`,
    name: slug,
    sourceLabel: 'docs',
    createdAt: '2026-01-01T00:00:00Z',
    stats: {
      domains: 1, tables: 4, columns: 1, relationships: 0, lineageEdges: 0,
      sourceTables: 0, conformed: 0, filesParsed: 1, filesSkipped: 0, diagnostics: 0,
    },
    projectId: slug,
    projectSlug: slug,
    project: {
      project: { name: slug, version: v, description: '' },
      internationalization: { primary: 'EN', supported: ['EN'], type: 'inline' },
    },
  }
}

const VERSIONS: Record<string, Snapshot[]> = {
  jaffle: [version('jaffle', '0.2.0'), version('jaffle', '1.0.0+b')],
  sakila: [version('sakila', '0.1.0')],
}

async function home() {
  const router = createAppRouter(createMemoryHistory())
  await router.push('/')
  await router.isReady()
  const w = mount(HomeView, { attachTo: document.body, global: { plugins: [router] } })
  await flushPromises()
  return { w, router }
}

type Wrapper = Awaited<ReturnType<typeof home>>['w']

function rowOf(w: Wrapper, slug: string) {
  const row = w.findAll('.recent > ul > li').find((li) => li.find('.snap-name').text() === slug)
  if (!row) throw new Error(`no row for ${slug}`)
  return row
}

async function expand(w: Wrapper, slug: string) {
  await rowOf(w, slug).find('button[aria-expanded]').trigger('click')
  await flushPromises()
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.mocked(api.listProjects).mockResolvedValue({ projects: [project('jaffle', 2), project('sakila', 1)] })
  vi.mocked(api.listVersions).mockImplementation(async (slug: string) => ({ versions: VERSIONS[slug] }))
  vi.mocked(api.deleteVersion).mockResolvedValue(undefined)
})

afterEach(() => {
  document.body.innerHTML = ''
})

describe('expanding a project', () => {
  it('lists its versions, fetched once', async () => {
    const { w } = await home()
    await expand(w, 'jaffle')

    const toggle = rowOf(w, 'jaffle').find('button[aria-expanded]')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    const rows = rowOf(w, 'jaffle').findAll('.version')
    expect(rows.map((r) => r.find('.version-label').text())).toEqual(['0.2.0', '1.0.0+b'])
    expect(rows[0].text()).toContain('latest')
    expect(rows[0].text()).toContain('4 tables')
    expect(rows[1].text()).not.toContain('latest')

    await expand(w, 'jaffle')
    expect(rowOf(w, 'jaffle').find('.version').exists()).toBe(false)
    await expand(w, 'jaffle')
    expect(rowOf(w, 'jaffle').findAll('.version')).toHaveLength(2)
    expect(api.listVersions).toHaveBeenCalledTimes(1)
  })

  it('opens a version when its row is clicked', async () => {
    const { w, router } = await home()
    await expand(w, 'jaffle')
    await rowOf(w, 'jaffle').findAll('.version-open')[1].trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('version')
    expect(router.currentRoute.value.params).toEqual({ project: 'jaffle', version: '1.0.0+b' })
  })
})

describe('deleting a version', () => {
  it('does not warn about the project while other versions remain', async () => {
    const { w } = await home()
    await expand(w, 'jaffle')
    await rowOf(w, 'jaffle').find('button[aria-label="Delete version 1.0.0+b"]').trigger('click')
    await flushPromises()

    const dialog = w.find('[role="alertdialog"]')
    expect(dialog.text()).toContain('Delete version 1.0.0+b of jaffle?')
    expect(dialog.text()).not.toContain(en['versions.delete.last'])
  })

  it('warns on the last version, and the project leaves the list', async () => {
    const { w } = await home()
    await expand(w, 'sakila')
    await rowOf(w, 'sakila').find('button[aria-label="Delete version 0.1.0"]').trigger('click')
    await flushPromises()

    const dialog = w.find('[role="alertdialog"]')
    expect(dialog.text()).toContain(en['versions.delete.last'])

    vi.mocked(api.listProjects).mockResolvedValue({ projects: [project('jaffle', 2)] })
    await dialog.findAll('button').find((b) => b.text() === 'Delete version')!.trigger('click')
    await flushPromises()

    expect(api.deleteVersion).toHaveBeenCalledWith('sakila', '0.1.0')
    expect(w.find('[role="alertdialog"]').exists()).toBe(false)
    expect(w.findAll('.recent > ul > li').map((li) => li.find('.snap-name').text())).toEqual(['jaffle'])
    // The project is gone, so its versions are not fetched again (that would 404).
    expect(api.listVersions).toHaveBeenCalledTimes(1)
  })
})
