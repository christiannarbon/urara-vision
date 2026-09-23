/** Routes, and the project view following its route param. */

import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import { RouterView, createMemoryHistory } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { createAppRouter } from '../../src/router'
import { useUi } from '../../src/stores/ui'
import { useWorkspace } from '../../src/stores/workspace'
import type { Snapshot, TableSummary } from '../../src/api/types'

const snapshot: Snapshot = {
  id: 's1',
  projectId: 'p1',
  projectSlug: 'first',
  name: 'first',
  sourceLabel: 'docs',
  createdAt: '2026-01-01T00:00:00Z',
  stats: {
    domains: 1, tables: 1, columns: 1, relationships: 0, lineageEdges: 0,
    sourceTables: 0, conformed: 0, filesParsed: 1, filesSkipped: 0, diagnostics: 0,
  },
}

const table: TableSummary = {
  id: 'domain_one/fact_primary', name: 'fact_primary', domainId: 'domain_one',
  kind: 'fact', grain: '', conformed: false, columnCount: 1, description: '',
}

beforeEach(() => setActivePinia(createPinia()))

describe('routes', () => {
  it('resolves a project path with its param', () => {
    const route = createAppRouter(createMemoryHistory()).resolve('/projects/jaffle-shop-ddd')
    expect(route.name).toBe('project')
    expect(route.params.project).toBe('jaffle-shop-ddd')
  })

  it('redirects an unknown path home', async () => {
    const router = createAppRouter(createMemoryHistory())
    await router.push('/no/such/page')
    expect(router.currentRoute.value.name).toBe('home')
  })
})

describe('ProjectView', () => {
  it('opens the project on mount and again when the param changes', async () => {
    const store = useWorkspace()
    const open = vi.spyOn(store, 'openProject').mockResolvedValue()
    const router = createAppRouter(createMemoryHistory())
    await router.push('/projects/first')
    await router.isReady()

    const Shell = defineComponent({ render: () => h(RouterView) })
    const w = mount(Shell, { global: { plugins: [router], stubs: { SearchOverlay: true } } })
    await flushPromises()
    expect(open.mock.calls.map((c) => c[0])).toEqual(['first'])

    await router.push('/projects/second')
    await flushPromises()
    expect(open.mock.calls.map((c) => c[0])).toEqual(['first', 'second'])
    w.unmount()
  })

  it('leaves no workspace, banner or overlay behind when the route goes home', async () => {
    const store = useWorkspace()
    vi.spyOn(store, 'openProject').mockResolvedValue()
    const clearBanner = vi.spyOn(store, 'clearProjectError')
    const ui = useUi()
    const router = createAppRouter(createMemoryHistory())
    await router.push('/projects/first')
    await router.isReady()

    const Shell = defineComponent({ render: () => h(RouterView) })
    const w = mount(Shell, {
      global: { plugins: [router], stubs: { SearchOverlay: true, GraphCanvas: true } },
    })
    await flushPromises()

    store.$patch({ snapshot, tables: [table] })
    ui.searchOpen = true

    await router.push('/')
    await flushPromises()

    expect(store.hasSnapshot).toBe(false)
    expect(store.tables).toEqual([])
    expect(clearBanner).toHaveBeenCalled()
    expect(ui.searchOpen).toBe(false)
    w.unmount()
  })
})
