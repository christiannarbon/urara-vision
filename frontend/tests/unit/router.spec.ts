/** Routes, and the project view following its route param. */

import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import { RouterView, createMemoryHistory } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { createAppRouter } from '../../src/router'
import { useWorkspace } from '../../src/stores/workspace'

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
})
