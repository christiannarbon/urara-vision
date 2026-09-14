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
  if (res) await router.push({ name: 'project', params: { project: res.project.slug } })
}

async function onOpen(slug: string) {
  await router.push({ name: 'project', params: { project: slug } })
}

function onDelete(slug: string) {
  pendingDelete.value = projects.value.find((p) => p.slug === slug) ?? null
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
      :projects="projects"
      :busy="busy"
      :status-message="statusMessage"
      @ingest="onIngest"
      @open="onOpen"
      @delete="onDelete"
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
  </main>
</template>

<style scoped>
.main { flex: 1; min-height: 0; overflow: hidden; }
</style>
