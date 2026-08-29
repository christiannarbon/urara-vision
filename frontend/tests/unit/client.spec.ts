/**
 * The fetch wrapper.
 *
 * Everything the UI knows about the backend goes through here, so what matters
 * is that a failure arrives as something the UI can act on -- an ApiError with
 * a status -- and that query strings are built the way the handlers expect.
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  ApiError,
  api,
  clearApiToken,
  hasApiToken,
  setApiToken,
} from '../../src/api/client'

/** The URL and init of the most recent fetch call. */
let calls: Array<{ url: string; init?: RequestInit }> = []

/** Installs a fetch stub returning one canned response. */
function stubFetch(response: { status?: number; body?: unknown; reject?: boolean }) {
  const fn = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
    calls.push({ url: String(url), init })
    if (response.reject) throw new TypeError('Failed to fetch')

    const status = response.status ?? 200
    return {
      ok: status >= 200 && status < 300,
      status,
      statusText: `status ${status}`,
      json: async () => {
        if (response.body === undefined) throw new SyntaxError('not JSON')
        return response.body
      },
    } as unknown as Response
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

beforeEach(() => {
  calls = []
})

afterEach(() => {
  vi.unstubAllGlobals()
  clearApiToken()
})

describe('request', () => {
  it('returns the decoded body on success', async () => {
    stubFetch({ body: { snapshots: [{ id: 's1' }] } })
    await expect(api.listSnapshots()).resolves.toEqual({ snapshots: [{ id: 's1' }] })
  })

  it('prefixes every path with the configured API base', async () => {
    stubFetch({ body: { snapshots: [] } })
    await api.listSnapshots()
    expect(calls[0].url).toBe('/api/v1/snapshots')
  })

  it('turns an unreachable backend into an ApiError with status 0', async () => {
    // Status 0 is what lets the UI say "the API is not running" rather than
    // showing an HTTP error the reader cannot act on.
    stubFetch({ reject: true })
    await expect(api.listSnapshots()).rejects.toMatchObject({
      name: 'ApiError',
      status: 0,
    })
  })

  it("carries the server's own error message through", async () => {
    stubFetch({ status: 400, body: { error: 'query parameter "id" is required' } })
    await expect(api.table('s1', '')).rejects.toMatchObject({
      status: 400,
      message: 'query parameter "id" is required',
    })
  })

  it('falls back to the status text when the error body is not JSON', async () => {
    stubFetch({ status: 502 })
    const err = await api.listSnapshots().catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(502)
    expect((err as ApiError).message).not.toBe('')
  })

  it('preserves the status so a 404 is distinguishable from a fault', async () => {
    stubFetch({ status: 404, body: { error: 'not found' } })
    await expect(api.getSnapshot('nope')).rejects.toMatchObject({ status: 404 })
  })

  it('handles a 204 with no body', async () => {
    stubFetch({ status: 204 })
    await expect(api.deleteSnapshot('s1')).resolves.toBeUndefined()
  })
})

describe('query strings', () => {
  beforeEach(() => {
    stubFetch({ body: {} })
  })

  it('omits absent, empty and false parameters', async () => {
    // An empty domain would be sent as a filter on the empty string, which the
    // backend would read as "no tables".
    await api.graph('s1', {})
    expect(calls[0].url).toBe('/api/v1/snapshots/s1/graph')

    calls = []
    await api.graph('s1', { domain: '', sources: false })
    expect(calls[0].url).toBe('/api/v1/snapshots/s1/graph')
  })

  it('sends the filters that are set', async () => {
    await api.graph('s1', { domain: 'domain_one,domain_two', kind: 'fact', sources: true })
    const url = new URL(calls[0].url, 'http://x')
    expect(url.searchParams.get('domain')).toBe('domain_one,domain_two')
    expect(url.searchParams.get('kind')).toBe('fact')
    expect(url.searchParams.get('sources')).toBe('true')
  })

  it('escapes a table ID rather than letting its slash become a path segment', async () => {
    // Table IDs contain a slash, which is exactly why they travel as a query
    // parameter; an unescaped one would change the route.
    await api.table('s1', 'domain_one/fact_primary')
    expect(calls[0].url).toContain('id=domain_one%2Ffact_primary')
    expect(calls[0].url.startsWith('/api/v1/snapshots/s1/table?')).toBe(true)
  })

  it('escapes the snapshot ID in the path', async () => {
    await api.getSnapshot('a b/c')
    expect(calls[0].url).toBe('/api/v1/snapshots/a%20b%2Fc')
  })

  it('applies the documented defaults', async () => {
    await api.neighborhood('s1', 'a/b')
    let url = new URL(calls[0].url, 'http://x')
    expect(url.searchParams.get('depth')).toBe('1')
    // sources defaults to false, and false is omitted entirely.
    expect(url.searchParams.has('sources')).toBe(false)

    calls = []
    await api.paths('s1', 'a/b', 'c/d')
    url = new URL(calls[0].url, 'http://x')
    expect(url.searchParams.get('maxDepth')).toBe('4')

    calls = []
    await api.search('s1', 'user')
    url = new URL(calls[0].url, 'http://x')
    expect(url.searchParams.get('limit')).toBe('50')

    calls = []
    await api.lineage('s1', 'a/b')
    url = new URL(calls[0].url, 'http://x')
    expect(url.searchParams.get('direction')).toBe('upstream')
  })
})

describe('ingest', () => {
  it('posts the files as JSON', async () => {
    stubFetch({ body: { snapshot: { id: 's1' } } })
    await api.ingest('name', 'label', [{ path: 'd/x.md', content: '# x' }])

    const { url, init } = calls[0]
    expect(url).toBe('/api/v1/ingest')
    expect(init?.method).toBe('POST')
    expect(new Headers(init?.headers).get('Content-Type')).toBe('application/json')
    expect(JSON.parse(String(init?.body))).toEqual({
      name: 'name',
      sourceLabel: 'label',
      files: [{ path: 'd/x.md', content: '# x' }],
    })
  })
})

describe('deleteSnapshot', () => {
  it('uses the DELETE method', async () => {
    stubFetch({ status: 204 })
    await api.deleteSnapshot('s1')
    expect(calls[0].init?.method).toBe('DELETE')
  })
})


describe('api token', () => {
  const TOKEN = '0123456789abcdef0123456789abcdef'

  /** The Authorization header of the most recent call, however init carried it. */
  function authHeader(): string | null {
    const init = calls.at(-1)?.init
    return new Headers(init?.headers).get('Authorization')
  }

  it('sends no Authorization header when no token is set', async () => {
    stubFetch({ body: { snapshots: [] } })
    await api.listSnapshots()
    expect(authHeader()).toBeNull()
  })

  it('sends the token as a bearer credential once set', async () => {
    setApiToken(TOKEN)
    stubFetch({ body: { snapshots: [] } })
    await api.listSnapshots()
    expect(authHeader()).toBe(`Bearer ${TOKEN}`)
  })

  it('trims a pasted token', async () => {
    setApiToken(`  ${TOKEN}\n`)
    stubFetch({ body: { snapshots: [] } })
    await api.listSnapshots()
    expect(authHeader()).toBe(`Bearer ${TOKEN}`)
  })

  it('keeps the caller\'s own headers alongside the token', async () => {
    setApiToken(TOKEN)
    stubFetch({ status: 201, body: { snapshot: {}, edges: 0, diagnostics: [] } })
    await api.ingest('n', 'src', [{ path: 'a.md', content: '# a' }])

    const headers = new Headers(calls.at(-1)?.init?.headers)
    expect(headers.get('Authorization')).toBe(`Bearer ${TOKEN}`)
    expect(headers.get('Content-Type')).toBe('application/json')
  })

  it('reports a 401 as an ApiError carrying the status', async () => {
    setApiToken(TOKEN)
    stubFetch({ status: 401, body: { error: 'unauthorized' } })
    await expect(api.listSnapshots()).rejects.toMatchObject({ status: 401 })
  })

  it('drops a rejected token so the next load prompts again', async () => {
    setApiToken(TOKEN)
    expect(hasApiToken()).toBe(true)

    stubFetch({ status: 401, body: { error: 'unauthorized' } })
    await expect(api.listSnapshots()).rejects.toBeInstanceOf(ApiError)

    expect(hasApiToken()).toBe(false)
  })

  it('stops sending the token after it has been cleared', async () => {
    setApiToken(TOKEN)
    clearApiToken()
    stubFetch({ body: { snapshots: [] } })
    await api.listSnapshots()
    expect(authHeader()).toBeNull()
  })
})
