<script setup lang="ts">
/** Picks which version of the open project is shown. */
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'

import { useI18n } from '../i18n'
import { useWorkspace } from '../stores/workspace'

const { t } = useI18n()
const router = useRouter()
const { snapshot, versions } = storeToRefs(useWorkspace())

const current = computed(() => snapshot.value?.project?.project.version ?? '')

function choose(e: Event) {
  const version = (e.target as HTMLSelectElement).value
  const project = snapshot.value?.projectSlug
  if (project && version !== current.value) {
    void router.push({ name: 'version', params: { project, version } })
  }
}
</script>

<template>
  <select
    v-if="versions.length > 1"
    class="switcher"
    :aria-label="t('version.label')"
    :title="t('version.label')"
    :value="current"
    @change="choose"
  >
    <option v-for="(v, i) in versions" :key="v.id" :value="v.project?.project.version">
      {{ i === 0 ? t('version.latest', { version: v.project?.project.version ?? '' }) : v.project?.project.version }}
    </option>
  </select>
</template>

<style scoped>
.switcher {
  max-width: 22ch;
  padding: 2px var(--space-2);
  font-size: 12px;
  color: var(--text-muted);
  background: var(--panel-raised);
  border: 1px solid var(--border);
  border-radius: var(--radius);
}
</style>
