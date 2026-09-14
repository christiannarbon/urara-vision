<script setup lang="ts">
/** The picker: ingest a directory or open a previous snapshot. */
import { onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'

import WelcomeScreen from '../components/WelcomeScreen.vue'
import { useWorkspace } from '../stores/workspace'

const router = useRouter()
const store = useWorkspace()
const { snapshots, busy, statusMessage } = storeToRefs(store)

onMounted(() => {
  void store.refreshSnapshots()
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

async function onOpen(sid: string) {
  await store.loadSnapshot(sid)
  const slug = store.snapshot?.projectSlug
  if (slug) await router.push({ name: 'project', params: { project: slug } })
}
</script>

<template>
  <main class="main">
    <WelcomeScreen
      :snapshots="snapshots"
      :busy="busy"
      :status-message="statusMessage"
      @ingest="onIngest"
      @open="onOpen"
      @delete="store.removeSnapshot"
    />
  </main>
</template>

<style scoped>
.main { flex: 1; min-height: 0; overflow: hidden; }
</style>
