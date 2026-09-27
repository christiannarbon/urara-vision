/** The canvas notes marker and the sidebar's domain notes button. */

import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Domain, GraphData, Snapshot } from '../../src/api/types'
import type { DiffMarks } from '../../src/graph/diff-marks'

// jsdom cannot run Cytoscape's renderer, so a fake tracks element classes and
// treats every other call as a chainable no-op.
const classes = vi.hoisted(() => new Map<string, Set<string>>())

vi.mock('cytoscape', () => {
  const noop = (): unknown =>
    new Proxy(function () {}, {
      get: (_t, p) => (p === Symbol.iterator ? [][Symbol.iterator] : p === 'length' ? 0 : noop()),
      apply: () => noop(),
    })
  const withFallback = <T extends object>(o: T) =>
    new Proxy(o, { get: (t, p) => (p in t ? t[p as keyof T] : noop()) })
  const element = (id: string) =>
    withFallback({
      addClass(c: string) {
        for (const x of c.split(' ')) classes.get(id)?.add(x)
        return this
      },
      removeClass(c: string) {
        for (const x of c.split(' ')) classes.get(id)?.delete(x)
        return this
      },
      hasClass: (c: string) => classes.get(id)?.has(c) ?? false,
    })
  const all = () =>
    withFallback({
      removeClass(c: string) {
        for (const id of classes.keys()) element(id).removeClass(c)
        return this
      },
      remove: () => classes.clear(),
    })
  const cy = () =>
    withFallback({
      batch: (fn: () => void) => fn(),
      add: (els: { data: { id: string } }[]) => els.forEach((e) => classes.set(e.data.id, new Set())),
      elements: all,
      nodes: all,
      getElementById: element,
    })
  return { default: Object.assign(cy, { use: () => {} }) }
})

vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return { ...actual, api: { noteCounts: vi.fn(), listNotes: vi.fn() } }
})

const { api } = await import('../../src/api/client')
const { useAuth } = await import('../../src/stores/auth')
const { useWorkspace } = await import('../../src/stores/workspace')
const GraphCanvas = (await import('../../src/components/GraphCanvas.vue')).default
const FilterSidebar = (await import('../../src/components/FilterSidebar.vue')).default
const NotesButton = (await import('../../src/components/notes/NotesButton.vue')).default
const NotesThread = (await import('../../src/components/notes/NotesThread.vue')).default

const DOMAINS: Domain[] = [{ id: 'sales', name: 'sales', title: 'Sales', description: '', docPath: '', tableCount: 2 }]

const GRAPH: GraphData = {
  nodes: [
    { id: 'sales/a', label: 'a', type: 'table', domainId: 'sales', kind: 'fact', degree: 1 },
    { id: 'sales/b', label: 'b', type: 'table', domainId: 'sales', kind: 'dimension', degree: 1 },
  ],
  links: [],
}

const mounted: VueWrapper[] = []

async function canvas(props: { notedTableIds?: Set<string>; diffMarks?: DiffMarks | null }) {
  const w = mount(GraphCanvas, {
    props: { data: GRAPH, domains: DOMAINS, selectedId: null, loading: false, layoutMode: 'force', ...props },
    attachTo: document.body,
  })
  mounted.push(w)
  await flushPromises()
  return { w, has: (id: string, c: string) => classes.get(id)?.has(c) ?? false }
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.mocked(api.noteCounts).mockResolvedValue({ counts: [] })
  vi.mocked(api.listNotes).mockResolvedValue({ notes: [] })
  // The hull layer draws on a plain canvas, which jsdom does not implement.
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
})

afterEach(() => {
  while (mounted.length) mounted.pop()!.unmount()
})

describe('canvas notes marker', () => {
  it('follows notedTableIds', async () => {
    const { w, has } = await canvas({ notedTableIds: new Set(['sales/a']) })
    expect(has('sales/a', 'has-notes')).toBe(true)
    expect(has('sales/b', 'has-notes')).toBe(false)

    await w.setProps({ notedTableIds: new Set() })
    expect(has('sales/a', 'has-notes')).toBe(false)
  })

  it('leaves diff marks alone, and the other way round', async () => {
    const diff: DiffMarks = { nodes: new Map([['sales/a', 'added']]), edges: new Map() }
    const { w, has } = await canvas({ notedTableIds: new Set(['sales/a']), diffMarks: diff })
    expect(has('sales/a', 'diff-added')).toBe(true)
    expect(has('sales/a', 'has-notes')).toBe(true)

    await w.setProps({ notedTableIds: new Set() })
    expect(has('sales/a', 'diff-added')).toBe(true)

    await w.setProps({ notedTableIds: new Set(['sales/a']), diffMarks: null })
    expect(has('sales/a', 'diff-added')).toBe(false)
    expect(has('sales/a', 'has-notes')).toBe(true)
  })
})

describe('domain notes in the sidebar', () => {
  it('opens a thread without toggling the domain', async () => {
    useWorkspace().snapshot = { id: 's1' } as Snapshot
    const auth = useAuth()
    auth.kind = 'user'
    auth.permissions = ['project.view', 'note.write']

    const w = mount(FilterSidebar, {
      props: {
        snapshot: null,
        domains: DOMAINS,
        tables: [],
        activeDomains: [],
        activeKinds: [],
        showSources: false,
        crossDomainOnly: false,
        viewMode: 'overview',
        layoutMode: 'force',
        focusDepth: 1,
        selectedId: null,
      },
      attachTo: document.body,
    })
    mounted.push(w)
    await flushPromises()

    const button = w.findComponent(NotesButton)
    expect(button.props('anchor')).toEqual({ kind: 'domain', id: 'sales' })
    await button.find('button').trigger('click')
    await flushPromises()

    expect(w.findComponent(NotesThread).exists()).toBe(true)
    expect(api.listNotes).toHaveBeenCalledWith('s1', 'domain', 'sales')
    expect(w.emitted('toggle-domain')).toBeUndefined()
  })
})
