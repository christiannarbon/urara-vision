<script setup lang="ts">
/** Application shell: header, three-pane workspace, overlays. */
import { computed, onMounted, onBeforeUnmount, ref, watchEffect } from 'vue'
import { storeToRefs } from 'pinia'

import ApiTokenGate from './components/ApiTokenGate.vue'
import DiagnosticsPanel from './components/DiagnosticsPanel.vue'
import FilterSidebar from './components/FilterSidebar.vue'
import GraphCanvas from './components/GraphCanvas.vue'
import SearchOverlay from './components/SearchOverlay.vue'
import TableDetail from './components/TableDetail.vue'
import ThemePicker from './components/ThemePicker.vue'
import WelcomeScreen from './components/WelcomeScreen.vue'
import { useI18n } from './i18n'
import { useWorkspace } from './stores/workspace'

const { t, tn } = useI18n()

// index.html carries an English title for the first paint; from here the tab
// follows the reader's language like everything else.
watchEffect(() => {
  document.title = t('app.title')
})

const store = useWorkspace()
const {
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
  authRequired,
} = storeToRefs(store)

const tokenGate = ref<InstanceType<typeof ApiTokenGate> | null>(null)
const tokenChecking = ref(false)

/** Hands the token to the store and tells the gate if it was refused. */
async function onTokenSubmit(token: string) {
  tokenChecking.value = true
  try {
    if (!(await store.submitApiToken(token))) tokenGate.value?.markRejected()
  } finally {
    tokenChecking.value = false
  }
}

const searchOpen = ref(false)
const diagnosticsOpen = ref(false)
const canvas = ref<InstanceType<typeof GraphCanvas> | null>(null)

/** The Diagnostics button carries a marker rather than a count: the number of
 *  findings says nothing about whether any of them matter, and a big number
 *  reads as a failure when most findings are merely worth a look. */
const marker = computed(() => {
  if (parseFailures.value.length) return 'error'
  if (findings.value.length) return 'warning'
  return null
})

const parseNotice = computed(() => tn('banner.parseFailures', parseFailures.value.length))

/** Spelled out separately so the sentence agrees in number. */
const parseNoticeDetail = computed(() =>
  tn('banner.parseFailuresDetail', parseFailures.value.length),
)

function reviewParseFailures() {
  diagnosticsOpen.value = true
  store.acknowledgeParseFailures()
}

onMounted(() => {
  void store.refreshSnapshots()
  window.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))

function onKeydown(e: KeyboardEvent) {
  const inField =
    e.target instanceof HTMLElement &&
    /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName)

  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault()
    if (hasSnapshot.value) searchOpen.value = true
    return
  }
  if (e.key === '/' && !inField && hasSnapshot.value) {
    e.preventDefault()
    searchOpen.value = true
    return
  }
  if (e.key === 'Escape' && !searchOpen.value) {
    if (diagnosticsOpen.value) diagnosticsOpen.value = false
    else if (selectedId.value) void store.select(null)
  }
}

async function onIngest(payload: {
  name: string
  sourceLabel: string
  files: { path: string; content: string }[]
}) {
  await store.ingest(payload.name, payload.sourceLabel, payload.files)
  await store.refreshSnapshots()
  // The Diagnostics panel stays closed. Findings are advisory and the reader
  // opens them when ready; only a dropped document interrupts, via the notice
  // below the header.
}

async function navigate(id: string) {
  await store.select(id)
  canvas.value?.panTo(id)
}

async function focusOn(id: string) {
  await store.focusOn(id)
}

function backToPicker() {
  store.$patch({ snapshot: null, selectedId: null, detail: null })
  void store.refreshSnapshots()
}
</script>

<template>
  <!-- The token gate replaces the shell entirely: nothing behind it can load
       until the backend accepts a token. -->
  <ApiTokenGate v-if="authRequired" ref="tokenGate" :busy="tokenChecking" @submit="onTokenSubmit" />

  <div v-else class="app">
    <header class="topbar">
      <button
        type="button"
        class="brand"
        :class="{ 'brand--link': hasSnapshot }"
        :disabled="!hasSnapshot"
        :title="hasSnapshot ? t('topbar.home') : undefined"
        @click="backToPicker"
      >
        <span class="mark" aria-hidden="true" />
        <span class="brand-name">Urara Vision</span>
      </button>

      <div v-if="hasSnapshot" class="snap-label">
        <span class="snap-name">{{ snapshot?.name }}</span>
        <span v-if="statusMessage" class="faint tiny">{{ statusMessage }}</span>
      </div>

      <div class="spacer" />

      <template v-if="hasSnapshot">
        <button class="btn btn--ghost btn--sm" :title="t('topbar.search.title')" @click="searchOpen = true">
          {{ t('topbar.search') }} <kbd>⌘K</kbd>
        </button>
        <button
          class="btn btn--ghost btn--sm"
          :aria-expanded="diagnosticsOpen"
          :title="hasDiagnostics ? t('topbar.diagnostics.titleAttention') : t('topbar.diagnostics.title')"
          @click="diagnosticsOpen = !diagnosticsOpen"
        >
          {{ t('topbar.diagnostics') }}
          <span
            v-if="marker"
            class="mark-flag"
            :class="`mark-flag--${marker}`"
            :aria-label="t('topbar.diagnostics.flag')"
            >!</span
          >
        </button>
        <button class="btn btn--ghost btn--sm" @click="backToPicker">
          {{ t('topbar.newIngest') }}
        </button>
      </template>

      <ThemePicker />
    </header>

    <p v-if="error" class="banner" role="alert">
      {{ error }}
      <button class="btn btn--ghost btn--sm" @click="store.dismissError">
        {{ t('banner.dismiss') }}
      </button>
    </p>

    <p v-if="hasSnapshot && needsParseNotice" class="banner banner--notice" role="alert">
      <span>
        <strong>{{ parseNotice }}</strong>
        {{ parseNoticeDetail }}
      </span>
      <span class="banner-actions">
        <button class="btn btn--ghost btn--sm" @click="reviewParseFailures">
          {{ t('banner.review') }}
        </button>
        <button class="btn btn--ghost btn--sm" @click="store.acknowledgeParseFailures">
          {{ t('banner.dismiss') }}
        </button>
      </span>
    </p>

    <main v-if="!hasSnapshot" class="main">
      <WelcomeScreen
        :snapshots="snapshots"
        :busy="busy"
        :status-message="statusMessage"
        @ingest="onIngest"
        @open="store.loadSnapshot"
        @delete="store.removeSnapshot"
      />
    </main>

    <main v-else class="workspace" :class="{ 'workspace--wide': diagnosticsOpen }">
      <FilterSidebar
        class="pane pane--left"
        :snapshot="snapshot"
        :domains="domains"
        :tables="tables"
        :active-domains="activeDomains"
        :active-kinds="activeKinds"
        :show-sources="showSources"
        :cross-domain-only="crossDomainOnly"
        :view-mode="viewMode"
        :layout-mode="layoutMode"
        :focus-depth="focusDepth"
        :selected-id="selectedId"
        @toggle-domain="store.toggleDomain"
        @toggle-kind="store.toggleKind"
        @set-sources="store.setShowSources"
        @set-cross-domain="store.setCrossDomainOnly"
        @set-view-mode="store.setViewMode"
        @set-layout-mode="store.setLayoutMode"
        @set-focus-depth="store.setFocusDepth"
        @clear="store.clearFilters"
        @select="navigate"
      />

      <div class="pane pane--graph">
        <GraphCanvas
          ref="canvas"
          :data="graph"
          :domains="domains"
          :selected-id="selectedId"
          :loading="graphLoading"
          :layout-mode="layoutMode"
          @select="store.select"
          @focus="focusOn"
        />
      </div>

      <TableDetail
        v-if="!diagnosticsOpen"
        class="pane pane--right"
        :detail="detail"
        :loading="detailLoading"
        :selected-id="selectedId"
        @navigate="navigate"
        @focus="focusOn"
        @close="store.select(null)"
      />

      <DiagnosticsPanel
        v-else
        class="pane pane--right"
        :open="diagnosticsOpen"
        :diagnostics="diagnostics"
        @close="diagnosticsOpen = false"
        @select="navigate"
      />
    </main>

    <SearchOverlay
      :open="searchOpen"
      :snapshot-id="snapshot?.id ?? null"
      @close="searchOpen = false"
      @select="navigate"
    />
  </div>
</template>

<style scoped>
.app {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.topbar {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 0 var(--space-3);
  height: 46px;
  flex: none;
  background: var(--panel);
  border-bottom: 1px solid var(--border);
}

/* The wordmark doubles as the way home: from a loaded model it returns to the
   picker, where the previous ingests are listed. */
.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0;
  border: 0;
  background: none;
  color: inherit;
  border-radius: var(--radius-sm);
}
.brand--link { cursor: pointer; }
.brand--link:hover .brand-name { color: var(--accent); }
.brand:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }
.mark {
  width: 14px;
  height: 14px;
  border-radius: var(--radius-sm);
  background: var(--fact);
  box-shadow: 6px 0 0 -3px var(--dim);
}
.brand-name {
  font-family: var(--font-display);
  font-size: 14px;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.snap-label {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-left: 14px;
  padding-left: 14px;
  border-left: 1px solid var(--border);
  min-width: 0;
}
.snap-name {
  font-size: 12.5px;
  color: var(--text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 34ch;
}
.tiny { font-size: 11px; }

.spacer { flex: 1; }

kbd {
  margin-left: var(--space-1);
  padding: 0 var(--space-1);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--text-faint);
}

/* A marker, not a tally: it says "look in here", not "you have 47 problems". */
.mark-flag {
  display: inline-grid;
  place-items: center;
  margin-left: 5px;
  width: 14px;
  height: 14px;
  border-radius: var(--radius-full);
  font-size: 10px;
  font-weight: 700;
  line-height: 1;
}
.mark-flag--error { background: var(--danger); color: var(--on-danger); }
.mark-flag--warning { background: var(--warning); color: var(--on-warning); }

.banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin: 0;
  padding: 8px 14px;
  flex: none;
  background: var(--danger-soft);
  color: var(--on-danger-soft);
  border-bottom: 1px solid var(--danger);
  font-size: 12.5px;
}
.banner--notice {
  background: var(--warning-soft);
  color: var(--on-warning-soft);
  border-bottom-color: var(--warning);
}
.banner-actions { display: flex; gap: 4px; flex: none; }

.main { flex: 1; min-height: 0; overflow: hidden; }

.workspace {
  flex: 1;
  display: grid;
  grid-template-columns: 250px 1fr 372px;
  min-height: 0;
  overflow: hidden;
}
.workspace--wide { grid-template-columns: 250px 1fr 400px; }

.pane { min-width: 0; min-height: 0; overflow: hidden; }

@media (max-width: 1180px) {
  .workspace, .workspace--wide { grid-template-columns: 210px 1fr 300px; }
}

@media (max-width: 900px) {
  .workspace, .workspace--wide {
    grid-template-columns: 1fr;
    grid-template-rows: minmax(0, 1fr) minmax(0, 1fr);
  }
  .pane--left { display: none; }
  .snap-label { display: none; }
}
</style>
