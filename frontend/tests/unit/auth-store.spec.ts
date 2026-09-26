/** The auth store and the redirect target the guard follows. */

import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return { ...actual, api: { me: vi.fn(), login: vi.fn(), logout: vi.fn() } }
})

const { api, ApiError } = await import('../../src/api/client')
const { useAuth } = await import('../../src/stores/auth')
const { useChat } = await import('../../src/stores/chat')
const { useDiff } = await import('../../src/stores/diff')
const { useWorkspace } = await import('../../src/stores/workspace')
const { safeNext } = await import('../../src/router')

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('load', () => {
  it('treats a 401 as signed out without throwing', async () => {
    vi.mocked(api.me).mockRejectedValue(new ApiError('not signed in', 401, 'error.notSignedIn'))
    const auth = useAuth()
    await expect(auth.load()).resolves.toBeUndefined()
    expect(auth.loaded).toBe(true)
    expect(auth.signedIn).toBe(false)
    expect(auth.user).toBeNull()
  })

  it('rethrows other failures, signed out', async () => {
    vi.mocked(api.me).mockRejectedValue(new ApiError('down', 0, 'error.unreachable'))
    const auth = useAuth()
    await expect(auth.load()).rejects.toMatchObject({ status: 0 })
    expect(auth.signedIn).toBe(false)
  })

  it('counts anonymous (AUTH_DISABLED) as signed in', async () => {
    vi.mocked(api.me).mockResolvedValue({ user: null, kind: 'anonymous', permissions: ['project.view'] })
    const auth = useAuth()
    await auth.load()
    expect(auth.signedIn).toBe(true)
  })
})

describe('can', () => {
  it('reads the permissions from /me', async () => {
    vi.mocked(api.me).mockResolvedValue({ user: null, kind: 'service', permissions: ['project.view', 'chat.use'] })
    const auth = useAuth()
    await auth.load()
    expect(auth.can('project.view')).toBe(true)
    expect(auth.can('project.delete')).toBe(false)
  })
})

describe('logout', () => {
  it('clears the state even when the request fails', async () => {
    vi.mocked(api.me).mockResolvedValue({ user: null, kind: 'service', permissions: ['chat.use'] })
    vi.mocked(api.logout).mockRejectedValue(new ApiError('down', 0))
    const auth = useAuth()
    await auth.load()
    await expect(auth.logout()).rejects.toBeInstanceOf(ApiError)
    expect(auth.signedIn).toBe(false)
    expect(auth.permissions).toEqual([])
  })
})

describe('clearing', () => {
  it('drops what the previous user loaded', async () => {
    vi.mocked(api.logout).mockResolvedValue(undefined)
    const workspace = useWorkspace()
    workspace.projects = [{ slug: 'p' } as never]
    workspace.snapshots = [{ id: 's' } as never]
    useChat().conversationId = 'conv-1'
    useDiff().result = { project: 'p' } as never

    await useAuth().logout()

    expect(workspace.projects).toEqual([])
    expect(workspace.snapshots).toEqual([])
    expect(useChat().conversationId).toBeNull()
    expect(useDiff().result).toBeNull()
  })
})

describe('safeNext', () => {
  it.each([
    ['/projects/x?tab=a', '/projects/x?tab=a'],
    ['//evil.com', '/'],
    ['https://evil.com', '/'],
    ['/\\evil.com', '/'],
    ['', '/'],
    [undefined, '/'],
    [['/a', '/b'], '/'],
  ])('%j → %s', (next, want) => {
    expect(safeNext(next)).toBe(want)
  })
})
