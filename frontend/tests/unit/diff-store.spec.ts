/** The diff store. */

import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { DiffResult } from '../../src/api/types'

vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return { ...actual, api: { diff: vi.fn() } }
})

const { api, ApiError } = await import('../../src/api/client')
const { useDiff } = await import('../../src/stores/diff')

const zero = { added: 0, removed: 0, changed: 0 }

function result(from: string, to: string): DiffResult {
  return {
    project: 'p',
    from: { version: from, snapshotId: `s-${from}` },
    to: { version: to, snapshotId: `s-${to}` },
    summary: { domains: zero, tables: zero, columns: zero, relationships: zero, lineage: zero },
    domains: [],
    tables: [],
    relationships: [],
    lineage: [],
  }
}

function deferred<T>() {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('useDiff', () => {
  it('drops a response whose params changed mid-load', async () => {
    const slow = deferred<DiffResult>()
    vi.mocked(api.diff)
      .mockReturnValueOnce(slow.promise)
      .mockResolvedValueOnce(result('1', '3'))
    const store = useDiff()

    const first = store.load('p', '1', '2')
    await store.load('p', '1', '3')
    slow.resolve(result('1', '2'))
    await first

    expect(store.result?.to.version).toBe('3')
    expect(store.loading).toBe(false)
  })

  it('drops a stale error too', async () => {
    const slow = deferred<DiffResult>()
    vi.mocked(api.diff)
      .mockReturnValueOnce(slow.promise)
      .mockResolvedValueOnce(result('1', '3'))
    const store = useDiff()

    const first = store.load('p', '1', '2')
    await store.load('p', '1', '3')
    slow.reject(new ApiError('version 2 not found', 404))
    await first

    expect(store.error).toBeNull()
    expect(store.result?.to.version).toBe('3')
  })

  it('keeps the error of the current load', async () => {
    vi.mocked(api.diff).mockRejectedValueOnce(new ApiError('version 9 not found', 404))
    const store = useDiff()
    await store.load('p', '1', '9')
    expect(store.error?.message).toBe('version 9 not found')
    expect(store.result).toBeNull()
  })
})
