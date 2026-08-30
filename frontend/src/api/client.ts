/** Thin fetch wrapper over the backend API. */

import type {
  Diagnostic,
  Domain,
  GraphData,
  IngestFile,
  IngestResult,
  JoinPath,
  LineageEntry,
  SearchHit,
  Snapshot,
  SourceTable,
  TableResponse,
  TableSummary,
} from './types'
import type { MessageKey } from '../i18n'

const BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? '/api/v1'

// Where the API token lives between visits. It shares the `relviz.` prefix the
// theme uses; see the note in the README about why that name survived.
const TOKEN_KEY = 'relviz.apiToken'

/** The token is module state rather than a store field: every request needs it,
 *  including the ones the store makes before it is itself constructed. */
let apiToken = readStoredToken()

function readStoredToken(): string {
  try {
    return localStorage.getItem(TOKEN_KEY) ?? ''
  } catch {
    // Private windows and blocked site data both throw; a session-only token
    // still works, it just will not survive a reload.
    return ''
  }
}

/** True when a token has been supplied. It says nothing about whether the
 *  token is correct -- only a 401 from the backend can tell you that. */
export function hasApiToken(): boolean {
  return apiToken !== ''
}

/** Stores the token for subsequent requests and for the next visit. */
export function setApiToken(value: string): void {
  apiToken = value.trim()
  try {
    if (apiToken) localStorage.setItem(TOKEN_KEY, apiToken)
    else localStorage.removeItem(TOKEN_KEY)
  } catch {
    // Blocked site data; the in-memory token still covers this session.
  }
}

/** Drops a token the backend has rejected, so the next load prompts again
 *  rather than retrying a value that is known to be wrong. */
export function clearApiToken(): void {
  setApiToken('')
}

/** ApiError carries the HTTP status so callers can distinguish a 404 from a
 *  server fault without parsing strings.
 *
 *  It also carries a catalogue key where this client is the one that decided
 *  what went wrong, so the banner can be rendered in whatever language is on
 *  screen at the time rather than the one that was active when it was thrown.
 *  There is no key when the message came from the server, which writes its own
 *  prose. `message` stays English either way: it is what lands in a console
 *  and a stack trace, and those have one audience. */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly key?: MessageKey,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  // Merge rather than replace: callers set Content-Type on the bodied calls.
  const headers = new Headers(init?.headers)
  if (apiToken) headers.set('Authorization', `Bearer ${apiToken}`)

  let res: Response
  try {
    res = await fetch(`${BASE}${path}`, { ...init, headers })
  } catch (cause) {
    throw new ApiError(
      'Cannot reach the backend. Check that the API is running and reachable.',
      0,
      'error.unreachable',
    )
  }

  if (res.status === 401) {
    // Whatever we hold is wrong or expired. Drop it so the app can ask again.
    clearApiToken()
    throw new ApiError(
      'This API needs a token, and the one supplied was not accepted.',
      401,
      'error.tokenRejected',
    )
  }

  if (!res.ok) {
    let detail = res.statusText
    try {
      const body = await res.json()
      if (body && typeof body.error === 'string') detail = body.error
    } catch {
      // Response was not JSON; the status text is the best available message.
    }
    // A key only when the server said nothing usable; its own error text is
    // prose this client has no translation for.
    throw new ApiError(
      detail || `Request failed with status ${res.status}.`,
      res.status,
      detail ? undefined : 'error.requestFailed',
    )
  }

  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

function qs(params: Record<string, string | number | boolean | undefined>): string {
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === '' || v === false) continue
    sp.set(k, String(v))
  }
  const s = sp.toString()
  return s ? `?${s}` : ''
}

export const api = {
  ingest(name: string, sourceLabel: string, files: IngestFile[]): Promise<IngestResult> {
    return request<IngestResult>('/ingest', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, sourceLabel, files }),
    })
  },

  listSnapshots(): Promise<{ snapshots: Snapshot[] }> {
    return request('/snapshots')
  },

  getSnapshot(sid: string): Promise<Snapshot> {
    return request(`/snapshots/${encodeURIComponent(sid)}`)
  },

  deleteSnapshot(sid: string): Promise<void> {
    return request(`/snapshots/${encodeURIComponent(sid)}`, { method: 'DELETE' })
  },

  domains(sid: string): Promise<{ domains: Domain[] }> {
    return request(`/snapshots/${encodeURIComponent(sid)}/domains`)
  },

  tables(sid: string, domain?: string): Promise<{ tables: TableSummary[] }> {
    return request(`/snapshots/${encodeURIComponent(sid)}/tables${qs({ domain })}`)
  },

  table(sid: string, id: string): Promise<TableResponse> {
    return request(`/snapshots/${encodeURIComponent(sid)}/table${qs({ id })}`)
  },

  graph(
    sid: string,
    opts: { domain?: string; kind?: string; sources?: boolean; crossDomainOnly?: boolean } = {},
  ): Promise<GraphData> {
    return request(`/snapshots/${encodeURIComponent(sid)}/graph${qs(opts)}`)
  },

  neighborhood(
    sid: string,
    table: string,
    depth = 1,
    sources = false,
  ): Promise<GraphData> {
    return request(
      `/snapshots/${encodeURIComponent(sid)}/neighborhood${qs({ table, depth, sources })}`,
    )
  },

  paths(sid: string, from: string, to: string, maxDepth = 4): Promise<{ paths: JoinPath[] }> {
    return request(`/snapshots/${encodeURIComponent(sid)}/paths${qs({ from, to, maxDepth })}`)
  },

  lineage(
    sid: string,
    id: string,
    direction: 'upstream' | 'downstream' = 'upstream',
  ): Promise<{ direction: string; entries: LineageEntry[] }> {
    return request(`/snapshots/${encodeURIComponent(sid)}/lineage${qs({ id, direction })}`)
  },

  search(sid: string, q: string, limit = 50): Promise<{ hits: SearchHit[] }> {
    return request(`/snapshots/${encodeURIComponent(sid)}/search${qs({ q, limit })}`)
  },

  diagnostics(sid: string, severity?: string): Promise<{ diagnostics: Diagnostic[] }> {
    return request(`/snapshots/${encodeURIComponent(sid)}/diagnostics${qs({ severity })}`)
  },

  sources(sid: string): Promise<{ sources: SourceTable[] }> {
    return request(`/snapshots/${encodeURIComponent(sid)}/sources`)
  },
}
