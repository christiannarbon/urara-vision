<script setup lang="ts">
/** Imports a new version of the open project from a local directory. */
import { ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'

import type { PickedDirectory } from '../composables/useDirectoryPicker'
import { supportsNativePicker, useDirectoryPicker } from '../composables/useDirectoryPicker'
import { useI18n } from '../i18n'
import { useWorkspace } from '../stores/workspace'

const { t } = useI18n()
const router = useRouter()
const store = useWorkspace()
const { snapshot, busy } = storeToRefs(store)

const picker = useDirectoryPicker()
const fileInput = ref<HTMLInputElement | null>(null)

watch(picker.error, (msg) => {
  if (msg) store.setError(new Error(msg))
})

async function choose() {
  if (supportsNativePicker()) {
    const picked = await picker.pickNative()
    if (picked) await submit(picked)
  } else {
    fileInput.value?.click()
  }
}

async function onInput(e: Event) {
  const input = e.target as HTMLInputElement
  const picked = await picker.readFileList(input.files)
  input.value = ''
  if (picked) await submit(picked)
}

async function submit(picked: PickedDirectory) {
  const project = snapshot.value?.projectSlug
  if (!project) return
  if (!picked.files.length) {
    picker.error.value = t('picker.error.noMarkdown')
    return
  }
  const res = await store.ingest(picked.name, picked.name, [picked.manifest, ...picked.files], project)
  const version = res?.snapshot.project?.project.version
  if (res && version) {
    await router.push({ name: 'version', params: { project: res.project.slug, version } })
  }
}
</script>

<template>
  <button
    class="btn btn--ghost btn--sm"
    :title="t('import.version.title')"
    :disabled="busy || picker.reading.value"
    @click="choose"
  >
    {{ t('import.version') }}
  </button>
  <input
    ref="fileInput"
    type="file"
    webkitdirectory
    directory
    multiple
    hidden
    @change="onInput"
  />
</template>
