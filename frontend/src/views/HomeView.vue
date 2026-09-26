<script setup lang="ts">
/** The picker: ingest a directory or open a project. */
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'

import ConfirmDialog from '../components/ConfirmDialog.vue'
import WelcomeScreen from '../components/WelcomeScreen.vue'
import type { Project } from '../api/types'
import { useI18n } from '../i18n'
import { useWorkspace } from '../stores/workspace'

const { t } = useI18n()
const router = useRouter()
const store = useWorkspace()
const { projects, busy, statusMessage } = storeToRefs(store)

const pendingDelete = ref<Project | null>(null)
const deleting = ref(false)

const welcome = ref<InstanceType<typeof WelcomeScreen> | null>(null)
const pendingVersion = ref<{ slug: string; version: string; last: boolean } | null>(null)

const versionMessage = computed(() => {
  const p = pendingVersion.value
  if (!p) return ''
  const name = projects.value.find((x) => x.slug === p.slug)?.name ?? p.slug
  const msg = t('versions.delete.message', { version: p.version, name })
  return p.last ? `${msg} ${t('versions.delete.last')}` : msg
})

const deleteMessage = computed(() =>
  pendingDelete.value ? t('projects.delete.message', { name: pendingDelete.value.name }) : '',
)

onMounted(() => {
  void store.refreshProjects()
})

async function onIngest(payload: {
  name: string
  sourceLabel: string
  files: { path: string; content: string }[]
}) {
  const res = await store.ingest(payload.name, payload.sourceLabel, payload.files)
  // The Diagnostics panel stays closed. Findings are advisory and the reader
  // opens them when ready; only a dropped document interrupts, via the notice
  // below the header.
  const version = res?.snapshot.project?.project.version
  if (res && version) await router.push({ name: 'version', params: { project: res.project.slug, version } })
  else if (res) await router.push({ name: 'project', params: { project: res.project.slug } })
}

async function onOpen(slug: string) {
  await router.push({ name: 'project', params: { project: slug } })
}

function onDelete(slug: string) {
  pendingDelete.value = projects.value.find((p) => p.slug === slug) ?? null
}

function onDeleteVersion(slug: string, version: string, remaining: number) {
  pendingVersion.value = { slug, version, last: remaining <= 1 }
}

async function confirmDeleteVersion() {
  const p = pendingVersion.value
  if (!p) return
  deleting.value = true
  try {
    await store.removeVersion(p.slug, p.version)
    await welcome.value?.reload(p.slug)
  } finally {
    deleting.value = false
    pendingVersion.value = null
  }
}

async function confirmDelete() {
  if (!pendingDelete.value) return
  deleting.value = true
  try {
    await store.removeProject(pendingDelete.value.slug)
  } finally {
    deleting.value = false
    pendingDelete.value = null
  }
}
</script>

<template>
  <main class="main">
    <WelcomeScreen
      ref="welcome"
      :projects="projects"
      :busy="busy"
      :status-message="statusMessage"
      :load-versions="store.listVersions"
      @ingest="onIngest"
      @open="onOpen"
      @delete="onDelete"
      @delete-version="onDeleteVersion"
    />
    <ConfirmDialog
      :open="pendingDelete !== null"
      :title="t('projects.delete.title')"
      :message="deleteMessage"
      :confirm-label="t('projects.delete')"
      danger
      :busy="deleting"
      @confirm="confirmDelete"
      @cancel="pendingDelete = null"
    />
    <ConfirmDialog
      :open="pendingVersion !== null"
      :title="t('versions.delete.title')"
      :message="versionMessage"
      :confirm-label="t('versions.delete')"
      danger
      :busy="deleting"
      @confirm="confirmDeleteVersion"
      @cancel="pendingVersion = null"
    />
  </main>
</template>

<style scoped>
.main { flex: 1; min-height: 0; overflow: hidden; }
</style>
