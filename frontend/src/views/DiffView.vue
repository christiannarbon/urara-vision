<script setup lang="ts">
/** What changed between two versions of a project; `?from=&to=` makes it linkable. */
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute, useRouter } from 'vue-router'

import DiffChangeList from '../components/DiffChangeList.vue'
import DiffSummary from '../components/DiffSummary.vue'
import { api, ApiError } from '../api/client'
import type { Snapshot } from '../api/types'
import { useI18n, type MessageKey } from '../i18n'
import { useDiff } from '../stores/diff'
import { useWorkspace } from '../stores/workspace'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const store = useDiff()
const { result, loading, error } = storeToRefs(store)

const project = computed(() => String(route.params.project ?? ''))
const from = computed(() => (typeof route.query.from === 'string' ? route.query.from : ''))
const to = computed(() => (typeof route.query.to === 'string' ? route.query.to : ''))

const versions = ref<Snapshot[] | null>(null)
const versionsError = ref<ApiError | null>(null)
// A not-found this page can name itself, translated rather than the server's text.
const problem = ref<{ key: MessageKey; params: Record<string, string> } | null>(null)
const label = (v: Snapshot) => v.project?.project.version ?? ''
const projectName = computed(() => versions.value?.[0]?.project?.project.name || project.value)
const onlyOne = computed(() => versions.value !== null && versions.value.length < 2)

watch(
  project,
  async (slug) => {
    versions.value = null
    versionsError.value = null
    problem.value = null
    store.reset()
    try {
      const res = await api.listVersions(slug)
      if (slug === project.value) versions.value = res.versions
    } catch (e) {
      if (slug !== project.value) return
      if (e instanceof ApiError && e.status === 404) problem.value = { key: 'project.notFound', params: { slug } }
      else versionsError.value = e instanceof ApiError ? e : new ApiError(String(e), 0, 'error.unknown')
    }
  },
  { immediate: true },
)

watch(
  [versions, from, to],
  ([vs, f, tv]) => {
    if (!vs || vs.length < 2) return
    if (!f || !tv) {
      // Default to the newest pair; replace so Back skips the bare URL.
      void router.replace({ query: { ...route.query, from: f || label(vs[1]), to: tv || label(vs[0]) } })
      return
    }
    const labels = vs.map(label)
    const missing = [f, tv].find((v) => !labels.includes(v))
    if (missing) {
      problem.value = { key: 'version.notFound', params: { version: missing } }
      store.reset()
      return
    }
    problem.value = null
    void store.load(project.value, f, tv)
  },
  { immediate: true },
)

onBeforeUnmount(() => store.reset())

function pick(which: 'from' | 'to', v: string) {
  void router.replace({ query: { ...route.query, [which]: v } })
}

function swap() {
  void router.replace({ query: { ...route.query, from: to.value, to: from.value } })
}

function showOnGraph() {
  void router.push({ name: 'version', params: { project: project.value, version: to.value }, query: { diffFrom: from.value } })
}

function openTable(id: string) {
  useWorkspace().selectAfterOpen(project.value, to.value, id)
  showOnGraph()
}

const shownError = computed(() => {
  if (problem.value) return t(problem.value.key, problem.value.params)
  const e = versionsError.value ?? error.value
  if (!e) return ''
  return e.key ? t(e.key, { status: e.status }) : e.message
})
</script>

<template>
  <main class="main">
    <div class="page">
      <header class="head">
        <div>
          <p class="section-label">{{ t('diff.title') }}</p>
          <h1>{{ projectName }}</h1>
        </div>
        <div v-if="versions && !onlyOne" class="pickers">
          <label class="picker">
            <span class="faint tiny">{{ t('diff.from') }}</span>
            <select class="input" :value="from" @change="pick('from', ($event.target as HTMLSelectElement).value)">
              <option v-for="v in versions" :key="v.id" :value="label(v)">{{ label(v) }}</option>
            </select>
          </label>
          <button class="btn btn--ghost btn--sm swap" :title="t('diff.swap.title')" @click="swap">
            ⇄ {{ t('diff.swap') }}
          </button>
          <label class="picker">
            <span class="faint tiny">{{ t('diff.to') }}</span>
            <select class="input" :value="to" @change="pick('to', ($event.target as HTMLSelectElement).value)">
              <option v-for="v in versions" :key="v.id" :value="label(v)">{{ label(v) }}</option>
            </select>
          </label>
        </div>
      </header>

      <p v-if="shownError" class="banner" role="alert">{{ shownError }}</p>
      <p v-else-if="onlyOne" class="muted state">{{ t('diff.onlyOne') }}</p>
      <div v-else-if="loading || !result" class="state">
        <div class="spinner" aria-hidden="true" />
        <span class="muted">{{ t('diff.loading') }}</span>
      </div>
      <template v-else>
        <div class="summary-row">
          <DiffSummary :summary="result.summary" />
          <button class="btn btn--sm show-graph" @click="showOnGraph">{{ t('diff.showOnGraph') }}</button>
        </div>
        <DiffChangeList :result="result" @open-table="openTable" />
      </template>
    </div>
  </main>
</template>

<style scoped>
.main { flex: 1; min-height: 0; overflow-y: auto; }
.page { max-width: 880px; margin: 0 auto; padding: 20px 16px 40px; display: flex; flex-direction: column; gap: 16px; }
.head { display: flex; flex-wrap: wrap; align-items: flex-end; justify-content: space-between; gap: 12px; }
.head h1 { font-size: 18px; margin: 2px 0 0; }
.pickers { display: flex; align-items: flex-end; gap: 8px; flex-wrap: wrap; }
.picker { display: flex; flex-direction: column; gap: 2px; }
.tiny { font-size: 11px; }
.summary-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.banner {
  margin: 0;
  padding: 8px 12px;
  border-radius: var(--radius-sm);
  background: var(--danger-soft);
  color: var(--on-danger-soft);
  font-size: 12.5px;
}
.state { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 32px 16px; text-align: center; }
.spinner {
  width: 20px;
  height: 20px;
  border: 2px solid var(--border-strong);
  border-top-color: var(--accent);
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }
</style>
