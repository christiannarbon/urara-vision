/** The home project list: open, and delete through the confirm dialog. */

import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Project } from '../../src/api/types'

vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return {
    ...actual,
    api: { listProjects: vi.fn(), deleteProject: vi.fn() },
  }
})

const { api } = await import('../../src/api/client')
const { createAppRouter } = await import('../../src/router')
const HomeView = (await import('../../src/views/HomeView.vue')).default

function project(slug: string, versionCount: number): Project {
  return {
    id: slug,
    slug,
    name: slug,
    description: '',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    versionCount,
    latest: { snapshotId: `${slug}-s`, version: '0.1.0', createdAt: '2026-01-01T00:00:00Z' },
  }
}

const PROJECTS = [project('jaffle', 2), project('sakila', 1)]

async function home() {
  const router = createAppRouter(createMemoryHistory())
  await router.push('/')
  await router.isReady()
  const w = mount(HomeView, { attachTo: document.body, global: { plugins: [router] } })
  await flushPromises()
  return { w, router }
}

function rowOf(w: Awaited<ReturnType<typeof home>>['w'], slug: string) {
  const row = w.findAll('li').find((li) => li.find('.snap-name').text() === slug)
  if (!row) throw new Error(`no row for ${slug}`)
  return row
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.mocked(api.listProjects).mockResolvedValue({ projects: PROJECTS })
  vi.mocked(api.deleteProject).mockResolvedValue(undefined)
})

afterEach(() => {
  document.body.innerHTML = ''
})

describe('the project list', () => {
  it('renders one row per project with its version count', async () => {
    const { w } = await home()
    expect(w.findAll('li')).toHaveLength(2)
    expect(rowOf(w, 'jaffle').text()).toContain('2 versions')
    expect(rowOf(w, 'sakila').text()).toContain('1 version')
  })

  it('navigates to the project when a row is clicked', async () => {
    const { w, router } = await home()
    await rowOf(w, 'jaffle').find('.snap').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/projects/jaffle')
  })
})

describe('deleting a project', () => {
  it('opens the dialog with focus on Cancel', async () => {
    const { w } = await home()
    await rowOf(w, 'sakila').find('button[aria-label="Delete project"]').trigger('click')
    await flushPromises()

    const dialog = w.find('[role="alertdialog"]')
    expect(dialog.text()).toContain('Delete project sakila and all of its versions?')
    expect(document.activeElement?.textContent?.trim()).toBe('Cancel')
  })

  it('calls nothing when cancelled', async () => {
    const { w } = await home()
    await rowOf(w, 'sakila').find('button[aria-label="Delete project"]').trigger('click')
    await flushPromises()

    const cancel = w.findAll('[role="alertdialog"] button').find((b) => b.text() === 'Cancel')!
    await cancel.trigger('click')
    expect(w.find('[role="alertdialog"]').exists()).toBe(false)
    expect(api.deleteProject).not.toHaveBeenCalled()
  })

  it('cancels on Escape', async () => {
    const { w } = await home()
    await rowOf(w, 'sakila').find('button[aria-label="Delete project"]').trigger('click')
    await flushPromises()

    await w.find('[role="alertdialog"]').trigger('keydown', { key: 'Escape' })
    expect(w.find('[role="alertdialog"]').exists()).toBe(false)
    expect(api.deleteProject).not.toHaveBeenCalled()
  })

  it('deletes once and refreshes when confirmed', async () => {
    const { w } = await home()
    await rowOf(w, 'sakila').find('button[aria-label="Delete project"]').trigger('click')
    await flushPromises()
    vi.mocked(api.listProjects).mockResolvedValue({ projects: [PROJECTS[0]] })

    const confirm = w
      .findAll('[role="alertdialog"] button')
      .find((b) => b.text() === 'Delete project')!
    await confirm.trigger('click')
    await flushPromises()

    expect(api.deleteProject).toHaveBeenCalledTimes(1)
    expect(api.deleteProject).toHaveBeenCalledWith('sakila')
    expect(api.listProjects).toHaveBeenCalledTimes(2)
    expect(w.findAll('li')).toHaveLength(1)
    expect(w.find('[role="alertdialog"]').exists()).toBe(false)
  })
})
