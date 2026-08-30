/** Central state for the loaded snapshot, the graph view and the selection. */

import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { api, ApiError, setApiToken } from '../api/client'
import { splitDiagnostics } from '../diagnostics'
import { translate as t, translateCount as tn } from '../i18n'
import type { MessageKey } from '../i18n'
import type { LayoutMode } from '../graph/layout'
import type {
  Diagnostic,
  Domain,
  GraphData,
  IngestFile,
  Snapshot,
  TableResponse,
  TableSummary,
} from '../api/types'

export type ViewMode = 'overview' | 'focus'

export type { LayoutMode }

export const useWorkspace = defineStore('workspace', () => {
  // --- snapshot ----------------------------------------------------------
  const snapshot = ref<Snapshot | null>(null)
  const snapshots = ref<Snapshot[]>([])
  const domains = ref<Domain[]>([])
  const tables = ref<TableSummary[]>([])
  const diagnostics = ref<Diagnostic[]>([])
  /** Set once the reader has dismissed the "documents could not be parsed"
   *  notice for the current snapshot. */
  const parseFailuresAcknowledged = ref(false)

  // --- graph -------------------------------------------------------------
  const graph = ref<GraphData>({ nodes: [], links: [] })
  const graphLoading = ref(false)
  const viewMode = ref<ViewMode>('overview')
  // How the canvas arranges the graph. Purely a drawing choice: it changes no
  // query, so unlike viewMode it never triggers a refetch.
  const layoutMode = ref<LayoutMode>('force')

  // --- filters -----------------------------------------------------------
  const activeDomains = ref<string[]>([])
  const activeKinds = ref<string[]>([])
  const showSources = ref(false)
  const crossDomainOnly = ref(false)
  const focusDepth = ref(1)

  // --- selection ---------------------------------------------------------
  const selectedId = ref<string | null>(null)
  const detail = ref<TableResponse | null>(null)
  const detailLoading = ref(false)

  // --- status ------------------------------------------------------------
  const busy = ref(false)

  /**
   * The banners hold what to say rather than the words for it, and resolve
   * through the catalogue on read. Both can sit on screen indefinitely -- an
   * error until it is dismissed, a status for as long as an ingest takes --
   * so a string frozen at the moment it was set would survive a language
   * change that everything around it had followed.
   */
  const status = ref<{ key: 'status.parsing'; n: number } | { key: 'status.loading' } | null>(null)
  const statusMessage = computed(() => {
    const s = status.value
    if (!s) return ''
    return s.key === 'status.parsing' ? tn('status.parsing', s.n) : t('status.loading')
  })

  /** A catalogue key where the frontend decided what went wrong, or the
   *  server's own prose where it did not. Never both. */
  const errorKey = ref<MessageKey | null>(null)
  const errorParams = ref<Record<string, string | number> | undefined>(undefined)
  const errorDetail = ref<string | null>(null)
  const error = computed(() =>
    errorKey.value ? t(errorKey.value, errorParams.value) : errorDetail.value,
  )

  function clearError() {
    errorKey.value = null
    errorParams.value = undefined
    errorDetail.value = null
  }
  /** Set once the backend has answered 401. The app shows the token prompt
   *  rather than an error banner: without a token there is nothing to read, so
   *  a dismissible message would leave the reader stuck on an empty screen. */
  const authRequired = ref(false)

  const hasSnapshot = computed(() => snapshot.value !== null)

  /** Documents that were dropped, versus findings about documents that parsed. */
  const buckets = computed(() => splitDiagnostics(diagnostics.value))
  const parseFailures = computed(() => buckets.value.unparsed)
  const findings = computed(() => buckets.value.findings)

  /** Whether the Diagnostics panel has anything at all worth opening for. */
  const hasDiagnostics = computed(() => diagnostics.value.length > 0)

  /** A dropped document is the one problem urgent enough to interrupt with:
   *  the model is silently incomplete until it is fixed. */
  const needsParseNotice = computed(
    () => parseFailures.value.length > 0 && !parseFailuresAcknowledged.value,
  )

  function acknowledgeParseFailures() {
    parseFailuresAcknowledged.value = true
  }

  const domainById = computed(() => {
    const m = new Map<string, Domain>()
    for (const d of domains.value) m.set(d.id, d)
    return m
  })

  const tableById = computed(() => {
    const m = new Map<string, TableSummary>()
    for (const t of tables.value) m.set(t.id, t)
    return m
  })

  function setError(e: unknown) {
    if (e instanceof ApiError && e.status === 401) {
      // Not a failure to report, a credential to collect. The client has
      // already dropped the rejected token.
      authRequired.value = true
      clearError()
      return
    }
    clearError()
    if (e instanceof ApiError && e.key) {
      errorKey.value = e.key
      // The status is the only thing any of these keys interpolates, and
      // passing it to the ones that do not is harmless.
      errorParams.value = { status: e.status }
    } else if (e instanceof Error) {
      errorDetail.value = e.message
    } else {
      errorKey.value = 'error.unknown'
    }
  }

  /** Uploads a picked directory and loads the resulting snapshot. */
  async function ingest(name: string, sourceLabel: string, files: IngestFile[]) {
    busy.value = true
    clearError()
    status.value = { key: 'status.parsing', n: files.length }
    try {
      const res = await api.ingest(name, sourceLabel, files)
      await loadSnapshot(res.snapshot.id)
      return res
    } catch (e) {
      setError(e)
      return null
    } finally {
      busy.value = false
      status.value = null
    }
  }

  /** Loads a snapshot's domains, tables, diagnostics and graph. */
  async function loadSnapshot(sid: string) {
    busy.value = true
    clearError()
    status.value = { key: 'status.loading' }
    try {
      const [snap, dom, tbl, diag] = await Promise.all([
        api.getSnapshot(sid),
        api.domains(sid),
        api.tables(sid),
        api.diagnostics(sid),
      ])
      snapshot.value = snap
      domains.value = dom.domains
      tables.value = tbl.tables
      diagnostics.value = diag.diagnostics
      parseFailuresAcknowledged.value = false

      // A fresh snapshot starts unfiltered, with nothing selected.
      activeDomains.value = []
      activeKinds.value = []
      crossDomainOnly.value = false
      selectedId.value = null
      detail.value = null
      viewMode.value = 'overview'

      await refreshGraph()
    } catch (e) {
      setError(e)
    } finally {
      busy.value = false
      status.value = null
    }
  }

  /** Stores a token and retries the first call the app makes. Returns false if
   *  the backend rejected it, which leaves authRequired set and the prompt up. */
  async function submitApiToken(token: string): Promise<boolean> {
    setApiToken(token)
    authRequired.value = false
    try {
      const res = await api.listSnapshots()
      snapshots.value = res.snapshots
      return true
    } catch (e) {
      setError(e)
      return false
    }
  }

  async function refreshSnapshots() {
    try {
      const res = await api.listSnapshots()
      snapshots.value = res.snapshots
    } catch (e) {
      setError(e)
    }
  }

  /** Rebuilds the graph from the current filters and view mode. */
  async function refreshGraph() {
    const sid = snapshot.value?.id
    if (!sid) return
    graphLoading.value = true
    try {
      if (viewMode.value === 'focus' && selectedId.value) {
        graph.value = await api.neighborhood(
          sid,
          selectedId.value,
          focusDepth.value,
          showSources.value,
        )
      } else {
        graph.value = await api.graph(sid, {
          domain: activeDomains.value.join(',') || undefined,
          kind: activeKinds.value.join(',') || undefined,
          sources: showSources.value,
          crossDomainOnly: crossDomainOnly.value,
        })
      }
    } catch (e) {
      setError(e)
    } finally {
      graphLoading.value = false
    }
  }

  /** Selects a table and loads its detail pane. Source nodes have no detail. */
  async function select(id: string | null) {
    selectedId.value = id
    if (!id || !snapshot.value) {
      detail.value = null
      return
    }
    if (!tableById.value.has(id)) {
      // A source-model node: keep it selected for highlighting, but there is
      // no table document to show.
      detail.value = null
      return
    }
    detailLoading.value = true
    try {
      detail.value = await api.table(snapshot.value.id, id)
    } catch (e) {
      setError(e)
      detail.value = null
    } finally {
      detailLoading.value = false
    }
  }

  function setLayoutMode(mode: LayoutMode) {
    layoutMode.value = mode
  }

  async function setViewMode(mode: ViewMode) {
    viewMode.value = mode
    await refreshGraph()
  }

  async function toggleDomain(id: string) {
    const i = activeDomains.value.indexOf(id)
    if (i >= 0) activeDomains.value.splice(i, 1)
    else activeDomains.value.push(id)
    if (viewMode.value === 'overview') await refreshGraph()
  }

  async function toggleKind(kind: string) {
    const i = activeKinds.value.indexOf(kind)
    if (i >= 0) activeKinds.value.splice(i, 1)
    else activeKinds.value.push(kind)
    if (viewMode.value === 'overview') await refreshGraph()
  }

  async function clearFilters() {
    activeDomains.value = []
    activeKinds.value = []
    crossDomainOnly.value = false
    await refreshGraph()
  }

  async function setShowSources(v: boolean) {
    showSources.value = v
    await refreshGraph()
  }

  async function setCrossDomainOnly(v: boolean) {
    crossDomainOnly.value = v
    if (v) viewMode.value = 'overview'
    await refreshGraph()
  }

  async function setFocusDepth(n: number) {
    focusDepth.value = n
    if (viewMode.value === 'focus') await refreshGraph()
  }

  /** Focuses the graph on one table, switching out of the overview. */
  async function focusOn(id: string) {
    await select(id)
    viewMode.value = 'focus'
    await refreshGraph()
  }

  async function removeSnapshot(sid: string) {
    try {
      await api.deleteSnapshot(sid)
      if (snapshot.value?.id === sid) {
        snapshot.value = null
        domains.value = []
        tables.value = []
        diagnostics.value = []
        parseFailuresAcknowledged.value = false
        graph.value = { nodes: [], links: [] }
        selectedId.value = null
        detail.value = null
      }
      await refreshSnapshots()
    } catch (e) {
      setError(e)
    }
  }

  function dismissError() {
    clearError()
  }

  return {
    snapshot,
    snapshots,
    domains,
    tables,
    diagnostics,
    graph,
    graphLoading,
    viewMode,
    layoutMode,
    activeDomains,
    activeKinds,
    showSources,
    crossDomainOnly,
    focusDepth,
    selectedId,
    detail,
    detailLoading,
    busy,
    statusMessage,
    error,
    hasSnapshot,
    parseFailures,
    findings,
    hasDiagnostics,
    needsParseNotice,
    domainById,
    tableById,
    ingest,
    loadSnapshot,
    authRequired,
    submitApiToken,
    refreshSnapshots,
    refreshGraph,
    select,
    setViewMode,
    setLayoutMode,
    toggleDomain,
    toggleKind,
    clearFilters,
    setShowSources,
    setCrossDomainOnly,
    setFocusDepth,
    focusOn,
    removeSnapshot,
    acknowledgeParseFailures,
    dismissError,
  }
})
