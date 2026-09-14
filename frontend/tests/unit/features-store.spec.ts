/** The features store. */

import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return { ...actual, api: { features: vi.fn(), patchSettings: vi.fn() } }
})

const { api, ApiError } = await import('../../src/api/client')
const { useFeatures } = await import('../../src/stores/features')

beforeEach(() => {
  setActivePinia(createPinia())
  vi.mocked(api.features).mockReset()
  vi.mocked(api.patchSettings).mockReset()
})

describe('load', () => {
  it('starts hidden', () => {
    const f = useFeatures()
    expect(f.chatAvailable).toBe(false)
    expect(f.chatEnabled).toBe(false)
    expect(f.loaded).toBe(false)
  })

  it('takes what the server says', async () => {
    vi.mocked(api.features).mockResolvedValue({ chat: { available: true, enabled: true } })
    const f = useFeatures()
    await f.load()
    expect(f.chatAvailable).toBe(true)
    expect(f.chatEnabled).toBe(true)
    expect(f.loaded).toBe(true)
  })

  it('stays hidden and throws nothing when the request fails', async () => {
    vi.mocked(api.features).mockRejectedValue(new ApiError('down', 0))
    const f = useFeatures()
    await expect(f.load()).resolves.toBeUndefined()
    expect(f.chatEnabled).toBe(false)
    expect(f.loaded).toBe(false)
  })
})

describe('setChatEnabled', () => {
  it('patches the server and takes the returned state', async () => {
    vi.mocked(api.features).mockResolvedValue({ chat: { available: true, enabled: true } })
    vi.mocked(api.patchSettings).mockResolvedValue({ chat: { available: true, enabled: false } })
    const f = useFeatures()
    await f.load()

    await f.setChatEnabled(false)

    expect(api.patchSettings).toHaveBeenCalledWith({ chatEnabled: false })
    expect(f.chatEnabled).toBe(false)
    expect(f.chatAvailable).toBe(true)
  })

  it('leaves state alone when the patch fails', async () => {
    vi.mocked(api.features).mockResolvedValue({ chat: { available: true, enabled: true } })
    vi.mocked(api.patchSettings).mockRejectedValue(new ApiError('boom', 500))
    const f = useFeatures()
    await f.load()

    await expect(f.setChatEnabled(false)).rejects.toThrow()
    expect(f.chatEnabled).toBe(true)
  })
})
