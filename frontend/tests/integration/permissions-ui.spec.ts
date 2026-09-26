/** What each role sees, a 403 mid-session, and route-level permissions. */

import { flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Project, Snapshot } from '../../src/api/types'
import { Perm } from '../../src/auth/permissions'
import { messages as en } from '../../src/i18n/messages/en'

vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return {
    ...actual,
    api: {
      me: vi.fn(),
      features: vi.fn(),
      listProjects: vi.fn(),
      listVersions: vi.fn(),
      deleteProject: vi.fn(),
      getVersion: vi.fn(),
      getSnapshot: vi.fn(),
      domains: vi.fn(),
      tables: vi.fn(),
      diagnostics: vi.fn(),
      graph: vi.fn(),
    },
  }
})

const { api } = await import('../../src/api/client')
const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
const { mountApp, ADMIN_PERMISSIONS } = await import('../helpers/mountApp')

// The role table in docs/tech/architecture/auth.md.
const VIEWER = ['chat.use', 'note.write', 'project.view']
const CREATOR = [...VIEWER, 'project.import', 'version.import']
const ADMIN = ADMIN_PERMISSIONS

const PROJECT: Project = {
  id: 'p-id',
  slug: 'p',
  name: 'p',
  description: '',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  versionCount: 1,
  latest: { snapshotId: 'p@0.1.0', version: '0.1.0', createdAt: '2026-01-01T00:00:00Z' },
}

function snap(version: string): Snapshot {
  return {
    id: `p@${version}`,
    name: 'p',
    sourceLabel: 'docs',
    createdAt: '2026-01-01T00:00:00Z',
    stats: {
      domains: 0, tables: 0, columns: 0, relationships: 0, lineageEdges: 0,
      sourceTables: 0, conformed: 0, filesParsed: 1, filesSkipped: 0, diagnostics: 0,
    },
    projectId: 'p-id',
    projectSlug: 'p',
    project: {
      project: { name: 'p', version, description: '' },
      internationalization: { primary: 'EN', supported: ['EN'], type: 'inline' },
    },
  }
}

const byText = (w: VueWrapper, text: string) => w.findAll('button').some((b) => b.text() === text)
const byLabel = (w: VueWrapper, label: string) => w.find(`[aria-label="${label}"]`).exists()

/** Home, with the project's versions expanded. */
async function home(permissions: string[]) {
  const w = await mountApp('/', { attachTo: document.body, permissions })
  await flushPromises()
  await w.findAll('button').find((b) => b.text() === en['versions.show'])!.trigger('click')
  await flushPromises()
  return w
}

async function workspace(permissions: string[]) {
  const w = await mountApp('/projects/p/versions/0.1.0', { attachTo: document.body, permissions })
  await flushPromises()
  return w
}

async function usersLinkShown(w: VueWrapper) {
  await w.find('[aria-haspopup="menu"]').trigger('click')
  expect(w.find('[role="menu"]').exists()).toBe(true)
  return w.findAll('[role="menuitem"]').some((b) => b.text() === en['auth.users'])
}

function seen(home: VueWrapper, ws: VueWrapper) {
  return {
    picker: home.find('.dropzone').exists(),
    projectDelete: byLabel(home, 'Delete project p'),
    versionDelete: byLabel(home, 'Delete version 0.1.0'),
    importVersion: byText(ws, en['import.version']),
    settings: byText(ws, en['settings.open']),
    chat: ws.findAll('button').some((b) => b.text().includes(en['chat.open'])),
  }
}

let mounted: VueWrapper[] = []

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.mocked(api.features).mockResolvedValue({ chat: { available: true, enabled: true } })
  vi.mocked(api.listProjects).mockResolvedValue({ projects: [PROJECT] })
  vi.mocked(api.listVersions).mockResolvedValue({ versions: [snap('0.1.0')] })
  vi.mocked(api.getVersion).mockImplementation(async (_s: string, v: string) => snap(v))
  vi.mocked(api.getSnapshot).mockImplementation(async (sid: string) => snap(sid.split('@')[1]))
  vi.mocked(api.domains).mockResolvedValue({ domains: [] })
  vi.mocked(api.tables).mockResolvedValue({ tables: [] })
  vi.mocked(api.diagnostics).mockResolvedValue({ diagnostics: [] })
  vi.mocked(api.graph).mockResolvedValue({ nodes: [], links: [] })
})

afterEach(() => {
  for (const w of mounted) w.unmount()
  mounted = []
  vi.unstubAllGlobals()
})

async function views(permissions: string[]) {
  const h = await home(permissions)
  const ws = await workspace(permissions)
  mounted.push(h, ws)
  return { h, ws }
}

describe('each role sees what it may do', () => {
  it('viewer: chat only', async () => {
    const { h, ws } = await views(VIEWER)
    expect(seen(h, ws)).toEqual({
      picker: false, projectDelete: false, versionDelete: false,
      importVersion: false, settings: false, chat: true,
    })
    expect(await usersLinkShown(ws)).toBe(false)
  })

  it('creator: imports, no deletes, settings or users', async () => {
    const { h, ws } = await views(CREATOR)
    expect(seen(h, ws)).toEqual({
      picker: true, projectDelete: false, versionDelete: false,
      importVersion: true, settings: false, chat: true,
    })
    expect(await usersLinkShown(ws)).toBe(false)
  })

  it('admin: everything', async () => {
    const { h, ws } = await views(ADMIN)
    expect(seen(h, ws)).toEqual({
      picker: true, projectDelete: true, versionDelete: true,
      importVersion: true, settings: true, chat: true,
    })
    expect(await usersLinkShown(ws)).toBe(true)
  })

  it('a viewer with no projects is told to ask for an import', async () => {
    vi.mocked(api.listProjects).mockResolvedValue({ projects: [] })
    const w = await mountApp('/', { permissions: VIEWER })
    mounted.push(w)
    await flushPromises()
    expect(w.text()).toContain(en['projects.askForImport'])
    expect(w.find('.dropzone').exists()).toBe(false)
  })
})

describe('a 403 mid-session', () => {
  it('shows error.notAllowed and reloads /auth/me', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{"error":"not allowed"}', { status: 403 })))
    vi.mocked(api.deleteProject).mockImplementation((slug) => actual.api.deleteProject(slug))
    vi.mocked(api.me).mockResolvedValue({ user: null, kind: 'user', permissions: VIEWER })

    const w = await mountApp('/', { attachTo: document.body, permissions: ADMIN })
    mounted.push(w)
    await flushPromises()
    await w.find('[aria-label="Delete project p"]').trigger('click')
    await w.findAll('button').find((b) => b.text() === en['projects.delete'])!.trigger('click')
    await flushPromises()

    expect(w.find('.banner').text()).toContain(en['error.notAllowed'])
    expect(api.me).toHaveBeenCalledTimes(1)
    // The refetched permissions take effect.
    expect(byLabel(w, 'Delete project p')).toBe(false)
  })
})

describe('route permissions', () => {
  it('sends a user without the permission home with a banner', async () => {
    const w = await mountApp('/', { permissions: VIEWER })
    mounted.push(w)
    w.vm.$router.addRoute({
      path: '/guarded',
      component: { template: '<p>guarded</p>' },
      meta: { permission: Perm.UserManage },
    })
    await w.vm.$router.push('/guarded')
    await flushPromises()

    expect(w.vm.$route.path).toBe('/')
    expect(w.find('.banner').text()).toContain(en['access.denied'])
  })

  it('lets a user with the permission through', async () => {
    const w = await mountApp('/', { permissions: ADMIN })
    mounted.push(w)
    w.vm.$router.addRoute({
      path: '/guarded',
      component: { template: '<p>guarded</p>' },
      meta: { permission: Perm.UserManage },
    })
    await w.vm.$router.push('/guarded')
    await flushPromises()

    expect(w.vm.$route.path).toBe('/guarded')
  })
})
