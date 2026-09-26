/** Importing a new version from the workspace, and a 409 from either screen. */

import { flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { Snapshot } from '../../src/api/types'

vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return {
    ...actual,
    api: {
      features: vi.fn(),
      ingest: vi.fn(),
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

const { api, ApiError } = await import('../../src/api/client')
const { mountApp } = await import('../helpers/mountApp')
const WelcomeScreen = (await import('../../src/components/WelcomeScreen.vue')).default

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

function pickedFile(relativePath: string, content = '# doc\n'): File {
  const f = new File([content], relativePath.split('/').pop() ?? relativePath)
  Object.defineProperty(f, 'webkitRelativePath', { value: relativePath })
  // jsdom's File has no text().
  Object.defineProperty(f, 'text', { value: async () => content })
  return f
}

/** Picks a directory through the button's fallback file input. */
async function importDirectory(w: VueWrapper) {
  const input = w.find('.topbar input[type="file"]')
  Object.defineProperty(input.element, 'files', {
    value: [pickedFile('v2/projectmeta.toml', '[project]\n'), pickedFile('v2/a.md')],
  })
  await input.trigger('change')
  await flushPromises()
}

async function workspace() {
  const w = await mountApp('/projects/p/versions/0.1.0')
  await flushPromises()
  return w
}

const conflict = () =>
  new ApiError('version 0.1.0 of p already exists', 409, undefined, {
    error: 'version 0.1.0 of p already exists',
    project: 'p',
    version: '0.1.0',
  })

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.mocked(api.features).mockResolvedValue({ chat: { available: false, enabled: false } })
  vi.mocked(api.listVersions).mockResolvedValue({ versions: [snap('0.1.0')] })
  vi.mocked(api.getVersion).mockImplementation(async (_s: string, v: string) => snap(v))
  vi.mocked(api.getSnapshot).mockImplementation(async (sid: string) => snap(sid.split('@')[1]))
  vi.mocked(api.domains).mockResolvedValue({ domains: [] })
  vi.mocked(api.tables).mockResolvedValue({ tables: [] })
  vi.mocked(api.diagnostics).mockResolvedValue({ diagnostics: [] })
  vi.mocked(api.graph).mockResolvedValue({ nodes: [], links: [] })
  vi.mocked(api.listProjects).mockResolvedValue({ projects: [] })
})

describe('importing a version from the workspace', () => {
  it('sends the open project, and lands on the new version', async () => {
    vi.mocked(api.ingest).mockResolvedValue({
      snapshot: snap('0.2.0'),
      project: { id: 'p-id', slug: 'p' },
      edges: 0,
      diagnostics: [],
    })
    const w = await workspace()
    await importDirectory(w)

    expect(api.ingest).toHaveBeenCalledTimes(1)
    expect(vi.mocked(api.ingest).mock.calls[0][3]).toBe('p')
    expect(w.vm.$route.fullPath).toBe('/projects/p/versions/0.2.0')
  })

  it('links to the existing version on a 409', async () => {
    vi.mocked(api.ingest).mockRejectedValue(conflict())
    const w = await workspace()
    await importDirectory(w)

    const banner = w.find('.banner--notice')
    expect(banner.text()).toContain('Version 0.1.0 already exists.')
    const link = banner.find('a')
    expect(link.attributes('href')).toBe('/projects/p/versions/0.1.0')

    await w.vm.$router.push('/')
    await flushPromises()
    await link.trigger('click')
    await flushPromises()
    expect(w.vm.$route.fullPath).toBe('/projects/p/versions/0.1.0')
    expect(w.find('.banner--notice').exists()).toBe(false)
  })

  it('shows the server message on a project mismatch', async () => {
    vi.mocked(api.ingest).mockRejectedValue(
      new ApiError('this directory is project sakila, not p', 400, undefined, {
        error: 'this directory is project sakila, not p',
      }),
    )
    const w = await workspace()
    await importDirectory(w)

    expect(w.find('.banner').text()).toContain('this directory is project sakila, not p')
    expect(w.vm.$route.fullPath).toBe('/projects/p/versions/0.1.0')
  })
})

describe('a 409 from the home picker', () => {
  it('shows the same banner and link', async () => {
    vi.mocked(api.ingest).mockRejectedValue(conflict())
    const w = await mountApp('/')
    await flushPromises()

    w.findComponent(WelcomeScreen).vm.$emit('ingest', {
      name: 'v1',
      sourceLabel: 'v1',
      files: [{ path: 'projectmeta.toml', content: '' }],
    })
    await flushPromises()

    expect(vi.mocked(api.ingest).mock.calls[0][3]).toBeUndefined()
    const banner = w.find('.banner--notice')
    expect(banner.text()).toContain('Version 0.1.0 already exists.')
    expect(banner.find('a').attributes('href')).toBe('/projects/p/versions/0.1.0')
    expect(w.vm.$route.name).toBe('home')
  })
})
