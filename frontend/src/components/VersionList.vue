<script setup lang="ts">
/** A project's versions on the home screen, newest first. */
import { useRouter } from 'vue-router'

import type { Snapshot } from '../api/types'
import { useI18n } from '../i18n'

const props = defineProps<{ project: string; versions: Snapshot[]; disabled?: boolean }>()
const emit = defineEmits<{ (e: 'delete', version: string): void }>()

const { t, tn, locale } = useI18n()
const router = useRouter()

const label = (v: Snapshot) => v.project?.project.version ?? ''

function open(v: Snapshot) {
  void router.push({ name: 'version', params: { project: props.project, version: label(v) } })
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString(locale.value)
}
</script>

<template>
  <ul class="versions">
    <li v-for="(v, i) in versions" :key="v.id" class="version">
      <button class="version-open" :disabled="disabled" @click="open(v)">
        <span class="version-label">{{ label(v) }}</span>
        <span v-if="i === 0" class="tag">{{ t('versions.latest') }}</span>
        <span class="faint tiny">
          {{ formatDate(v.createdAt) }} · {{ tn('versions.tables', v.stats.tables) }}
        </span>
      </button>
      <button
        class="btn btn--ghost btn--sm"
        :disabled="disabled"
        :title="t('versions.delete.named', { version: label(v) })"
        :aria-label="t('versions.delete.named', { version: label(v) })"
        @click="emit('delete', label(v))"
      >
        ✕
      </button>
    </li>
  </ul>
</template>

<style scoped>
.versions { list-style: none; margin: 0 0 8px; padding: 0 0 0 16px; }
.version { display: flex; align-items: center; gap: 6px; }
.version-open {
  flex: 1;
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
  padding: 6px 8px;
  border: none;
  border-radius: var(--radius-sm);
  background: none;
  text-align: left;
}
.version-open:hover:not(:disabled) { background: var(--bg-sunken); }
.version-open:disabled { opacity: 0.5; cursor: not-allowed; }
.version-label {
  font-size: 12.5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tag {
  padding: 0 6px;
  font-size: 10.5px;
  border-radius: var(--radius-sm);
  background: var(--fact-soft);
  color: var(--text-muted);
}
.tiny { font-size: 11px; }
</style>
