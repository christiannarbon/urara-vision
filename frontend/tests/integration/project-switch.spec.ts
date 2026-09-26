/** Switching project mid-load: the workspace ends on the project in the URL. */

import { flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { Snapshot } from '../../src/api/types'

vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return {
    ...actual,
    api: {
      features: vi.fn(),
      listVersions: vi.fn(),
      getVersion: vi.fn(),
      getSnapshot: vi.fn(),
      domains: vi.fn(),
      tables: vi.fn(),
      diagnostics: vi.fn(),
      graph: vi.fn(),
      listProjects: vi.fn(),
    },
  }
})

const { api } = await import('../../src/api/client')
const { mountApp } = await import('../helpers/mountApp')
const { useWorkspace } = await import('../../src/stores/workspace')

const STATS = {
  domains: 1,
  tables: 1,
  columns: 1,
  relationships: 0,
  lineageEdges: 0,
  sourceTables: 0,
  conformed: 0,
  filesParsed: 1,
  filesSkipped: 0,
  diagnostics: 0,
}

function snapshot(sid: string): Snapshot {
  const slug = sid.replace(/-v\d+$/, '')
  return {
    id: sid,
    name: slug,
    sourceLabel: 'docs',
    createdAt: '2026-01-01T00:00:00Z',
    stats: STATS,
    projectId: `${slug}-id`,
    projectSlug: slug,
  }
}

/** Holds project `slug`'s load open until the returned function is called. */
function hold(slug: string) {
  const held: { release?: () => void } = {}
  vi.mocked(api.getVersion).mockImplementation(async (s: string) => {
    if (s === slug) await new Promise<void>((r) => (held.release = r))
    return snapshot(`${s}-v1`)
  })
  return held
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.mocked(api.features).mockResolvedValue({ chat: { available: false, enabled: false } })
  vi.mocked(api.listVersions).mockImplementation(async (slug: string) => ({
    versions: [snapshot(`${slug}-v1`)],
  }))
  vi.mocked(api.getVersion).mockImplementation(async (slug: string) => snapshot(`${slug}-v1`))
  vi.mocked(api.getSnapshot).mockImplementation(async (sid: string) => snapshot(sid))
  vi.mocked(api.domains).mockResolvedValue({ domains: [] })
  vi.mocked(api.tables).mockResolvedValue({ tables: [] })
  vi.mocked(api.diagnostics).mockResolvedValue({ diagnostics: [] })
  vi.mocked(api.graph).mockResolvedValue({ nodes: [], links: [] })
  vi.mocked(api.listProjects).mockResolvedValue({ projects: [] })
})

describe('a project load the reader has left', () => {
  it('does not overwrite the one they moved to', async () => {
    const held = hold('a')
    const w = await mountApp('/projects/a')
    await flushPromises()

    await w.vm.$router.push('/projects/b')
    await flushPromises()
    expect(useWorkspace().snapshot?.projectSlug).toBe('b')

    held.release?.()
    await flushPromises()

    expect(w.vm.$route.params.project).toBe('b')
    expect(useWorkspace().snapshot?.projectSlug).toBe('b')
  })

  it('does not overwrite the workspace after the reader goes home', async () => {
    const held = hold('a')
    const w = await mountApp('/projects/a')
    await flushPromises()

    await w.vm.$router.push('/')
    await flushPromises()

    held.release?.()
    await flushPromises()

    expect(useWorkspace().snapshot).toBe(null)
  })

  it('still loads a project the reader is waiting for', async () => {
    // The guard drops stale answers, not slow ones.
    const held = hold('a')
    const w = await mountApp('/projects/a')
    await flushPromises()

    held.release?.()
    await flushPromises()

    expect(useWorkspace().snapshot?.projectSlug).toBe('a')
    expect(w.find('main.workspace').exists()).toBe(true)
  })

  it('reloads the same project after leaving and returning', async () => {
    const w = await mountApp('/projects/a')
    await flushPromises()

    await w.vm.$router.push('/')
    await flushPromises()
    await w.vm.$router.push('/projects/a')
    await flushPromises()

    expect(useWorkspace().snapshot?.projectSlug).toBe('a')
  })
})
