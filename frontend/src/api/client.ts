/** Thin fetch wrapper over the backend API. */

import type {
  Diagnostic,
  DiffResult,
  Domain,
  Features,
  GraphData,
  IngestFile,
  IngestResult,
  JoinPath,
  LineageEntry,
  Me,
  Project,
  SearchHit,
  Snapshot,
  SourceTable,
  TableResponse,
  TableSummary,
  User,
} from './types'
import type { MessageKey } from '../i18n'

const BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? '/api/v1'

// Tokens from before cookie sessions must not linger.
try {
  localStorage.removeItem('relviz.apiToken')
} catch {
  // Blocked site data.
}

let onUnauthorized: (() => void) | undefined

/** Registered by the auth store; avoids an import cycle. */
export function setOnUnauthorized(fn: (() => void) | undefined): void {
  onUnauthorized = fn
}

const UNSAFE = new Set(['POST', 'PUT', 'PATCH', 'DELETE'])

/** Credentials and the CSRF header for every request to our own services. */
export function withSession(init?: RequestInit): RequestInit {
  const headers = new Headers(init?.headers)
  if (UNSAFE.has((init?.method ?? 'GET').toUpperCase())) headers.set('X-Requested-With', 'urara')
  return { ...init, headers, credentials: 'same-origin' }
}

/**
 * ApiError carries the HTTP status so callers can distinguish a 404 from a server fault without…
 */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly key?: MessageKey,
    /** The parsed JSON error body, when there was one. */
    readonly body?: Record<string, unknown>,
    readonly headers?: Headers,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await fetch(`${BASE}${path}`, withSession(init))
  } catch (cause) {
    throw new ApiError(
      'Cannot reach the backend. Check that the API is running and reachable.',
      0,
      'error.unreachable',
    )
  }

  // Login answers 401 for bad credentials; that is not a lost session.
  if (res.status === 401 && path !== '/auth/login') {
    onUnauthorized?.()
    throw new ApiError('You are not signed in.', 401, 'error.notSignedIn')
  }

  if (!res.ok) {
    let detail = res.statusText
    let body: Record<string, unknown> | undefined
    try {
      const parsed = await res.json()
      if (parsed && typeof parsed === 'object') body = parsed
      if (typeof body?.error === 'string') detail = body.error
    } catch {
      // Response was not JSON; the status text is the best available message.
    }
    // A key only when the server said nothing usable; its own error text is
    // prose this client has no translation for.
    throw new ApiError(
      detail || `Request failed with status ${res.status}.`,
      res.status,
      detail ? undefined : 'error.requestFailed',
      body,
      res.headers,
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

const json = { 'Content-Type': 'application/json' }

export const api = {
  login(username: string, password: string): Promise<{ user: User }> {
    return request('/auth/login', { method: 'POST', headers: json, body: JSON.stringify({ username, password }) })
  },

  logout(): Promise<void> {
    return request('/auth/logout', { method: 'POST' })
  },

  me(): Promise<Me> {
    return request('/auth/me')
  },

  changePassword(current: string, next: string): Promise<void> {
    return request('/auth/password', {
      method: 'POST',
      headers: json,
      body: JSON.stringify({ current, new: next }),
    })
  },

  features(): Promise<Features> {
    return request('/features')
  },

  patchSettings(body: { chatEnabled: boolean }): Promise<Features> {
    return request('/settings', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
  },

  /** `project` makes the server refuse a directory whose manifest names another project. */
  ingest(name: string, sourceLabel: string, files: IngestFile[], project?: string): Promise<IngestResult> {
    return request<IngestResult>('/ingest', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, sourceLabel, files, ...(project ? { project } : {}) }),
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

  listProjects(): Promise<{ projects: Project[] }> {
    return request('/projects')
  },

  getProject(slug: string): Promise<Project> {
    return request(`/projects/${encodeURIComponent(slug)}`)
  },

  deleteProject(slug: string): Promise<void> {
    return request(`/projects/${encodeURIComponent(slug)}`, { method: 'DELETE' })
  },

  listVersions(slug: string): Promise<{ versions: Snapshot[] }> {
    return request(`/projects/${encodeURIComponent(slug)}/versions`)
  },

  getVersion(slug: string, version: string): Promise<Snapshot> {
    return request(`/projects/${encodeURIComponent(slug)}/versions/${encodeURIComponent(version)}`)
  },

  deleteVersion(slug: string, version: string): Promise<void> {
    return request(`/projects/${encodeURIComponent(slug)}/versions/${encodeURIComponent(version)}`, {
      method: 'DELETE',
    })
  },

  diff(slug: string, from: string, to: string): Promise<DiffResult> {
    return request(`/projects/${encodeURIComponent(slug)}/diff${qs({ from, to })}`)
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
