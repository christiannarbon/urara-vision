/** The version in the URL, and the switcher that changes it. */

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

function snap(version: string): Snapshot {
  return {
    id: `p@${version}`,
    name: 'p',
    sourceLabel: 'docs',
    createdAt: '2026-01-01T00:00:00Z',
    stats: STATS,
    projectId: 'p-id',
    projectSlug: 'p',
    project: {
      project: { name: 'p', version, description: '' },
      internationalization: { primary: 'EN', supported: ['EN'], type: 'inline' },
    },
  }
}

/** Newest first, as the API returns them. */
function withVersions(...versions: string[]) {
  vi.mocked(api.listVersions).mockResolvedValue({ versions: versions.map(snap) })
  vi.mocked(api.getVersion).mockImplementation(async (_slug: string, v: string) =>
    snap(v === 'latest' ? versions[0] : v),
  )
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.mocked(api.features).mockResolvedValue({ chat: { available: false, enabled: false } })
  vi.mocked(api.getSnapshot).mockImplementation(async (sid: string) => snap(sid.split('@')[1]))
  vi.mocked(api.domains).mockResolvedValue({ domains: [] })
  vi.mocked(api.tables).mockResolvedValue({ tables: [] })
  vi.mocked(api.diagnostics).mockResolvedValue({ diagnostics: [] })
  vi.mocked(api.graph).mockResolvedValue({ nodes: [], links: [] })
  vi.mocked(api.listProjects).mockResolvedValue({ projects: [] })
  withVersions('0.2.0', '0.1.0')
})

describe('a project URL without a version', () => {
  it('is replaced by the latest version, so Back goes home', async () => {
    const w = await mountApp('/')
    await w.vm.$router.push('/projects/p')
    await flushPromises()
    expect(w.vm.$route.fullPath).toBe('/projects/p/versions/0.2.0')

    w.vm.$router.back()
    await flushPromises()
    expect(w.vm.$route.name).toBe('home')
  })

  it('loads the snapshot once, not again after the redirect', async () => {
    await mountApp('/projects/p')
    await flushPromises()
    expect(api.getVersion).toHaveBeenCalledTimes(1)
    expect(api.getSnapshot).toHaveBeenCalledTimes(1)
  })
})

describe('a version URL', () => {
  it('passes the decoded version to the API', async () => {
    withVersions('1.0.0+b')
    await mountApp('/projects/p/versions/1.0.0%2Bb')
    await flushPromises()
    expect(api.getVersion).toHaveBeenCalledWith('p', '1.0.0+b')
  })
})

describe('the version switcher', () => {
  it('pushes the chosen version', async () => {
    const w = await mountApp('/projects/p/versions/0.2.0')
    await flushPromises()

    const select = w.find('select.switcher')
    expect((select.element as HTMLSelectElement).value).toBe('0.2.0')
    await select.setValue('0.1.0')
    await flushPromises()

    expect(w.vm.$route.fullPath).toBe('/projects/p/versions/0.1.0')
    expect(api.getVersion).toHaveBeenLastCalledWith('p', '0.1.0')
  })

  it('marks the newest as latest', async () => {
    const w = await mountApp('/projects/p/versions/0.1.0')
    await flushPromises()
    const options = w.findAll('select.switcher option').map((o) => o.text())
    expect(options).toEqual(['0.2.0 (latest)', '0.1.0'])
  })

  it('is hidden when the project has one version', async () => {
    withVersions('0.1.0')
    const w = await mountApp('/projects/p/versions/0.1.0')
    await flushPromises()
    expect(w.find('select.switcher').exists()).toBe(false)
  })
})
