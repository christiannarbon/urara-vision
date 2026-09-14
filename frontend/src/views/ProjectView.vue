<script setup lang="ts">
/** The workspace for one project: filters, canvas and the right-hand pane. */
import { onBeforeUnmount, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute } from 'vue-router'

import ChatPanel from '../components/ChatPanel.vue'
import DiagnosticsPanel from '../components/DiagnosticsPanel.vue'
import FilterSidebar from '../components/FilterSidebar.vue'
import GraphCanvas from '../components/GraphCanvas.vue'
import SearchOverlay from '../components/SearchOverlay.vue'
import TableDetail from '../components/TableDetail.vue'
import { useChat } from '../stores/chat'
import { useFeatures } from '../stores/features'
import { useUi } from '../stores/ui'
import { useWorkspace } from '../stores/workspace'

const route = useRoute()
const store = useWorkspace()
const {
  snapshot,
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
  hasSnapshot,
} = storeToRefs(store)

const ui = useUi()
const { searchOpen, diagnosticsOpen } = storeToRefs(ui)
const { open: chatOpen } = storeToRefs(useChat())
const { chatEnabled } = storeToRefs(useFeatures())

const canvas = ref<InstanceType<typeof GraphCanvas> | null>(null)

watch(
  () => route.params.project,
  (project) => {
    if (typeof project === 'string') void store.openProject(project)
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  store.$patch({ snapshot: null, selectedId: null, detail: null })
})

async function navigate(id: string) {
  await store.select(id)
  canvas.value?.panTo(id)
}

async function focusOn(id: string) {
  await store.focusOn(id)
}
</script>

<template>
  <main v-if="hasSnapshot" class="workspace" :class="{ 'workspace--wide': diagnosticsOpen || chatOpen }">
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

    <ChatPanel v-if="chatEnabled && chatOpen" class="pane pane--right" />

    <DiagnosticsPanel
      v-else-if="diagnosticsOpen"
      class="pane pane--right"
      :open="diagnosticsOpen"
      :diagnostics="diagnostics"
      @close="diagnosticsOpen = false"
      @select="navigate"
    />

    <TableDetail
      v-else
      class="pane pane--right"
      :detail="detail"
      :loading="detailLoading"
      :selected-id="selectedId"
      @navigate="navigate"
      @focus="focusOn"
      @close="store.select(null)"
    />
  </main>
  <!-- Loading, or a not-found or empty project: the banner in App says which. -->
  <main v-else class="main" />

  <SearchOverlay
    :open="searchOpen"
    :snapshot-id="snapshot?.id ?? null"
    @close="searchOpen = false"
    @select="navigate"
  />
</template>

<style scoped>
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
}
</style>
