<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'

import type { DiffResult } from '../api/types'
import { useI18n } from '../i18n'

const props = defineProps<{ result: DiffResult }>()
const emit = defineEmits<{ (e: 'clear'): void }>()
const { t } = useI18n()

// Counted from the summary, so tables and joins hidden by a filter still count.
const added = computed(() => props.result.summary.tables.added + props.result.summary.relationships.added)
const changed = computed(() => props.result.summary.tables.changed + props.result.summary.relationships.changed)
</script>

<template>
  <section class="diff-legend" :aria-label="t('diff.legend.label')">
    <header class="top">
      <strong>{{ t('diff.legend.title', { version: result.from.version }) }}</strong>
      <button class="btn btn--ghost btn--sm clear" @click="emit('clear')">{{ t('diff.legend.clear') }}</button>
    </header>
    <div class="swatches">
      <span class="lg"><span class="sw sw--added" />{{ t('diff.legend.added') }} {{ added }}</span>
      <span class="lg"><span class="sw sw--changed" />{{ t('diff.legend.changed') }} {{ changed }}</span>
    </div>
    <p class="note faint">
      {{ t('diff.legend.removedNote') }}
      <RouterLink
        class="full"
        :to="{ name: 'diff', params: { project: result.project }, query: { from: result.from.version, to: result.to.version } }"
      >
        {{ t('diff.legend.full') }}
      </RouterLink>
    </p>
  </section>
</template>

<style scoped>
.diff-legend {
  position: absolute;
  top: 12px;
  left: 12px;
  z-index: 6;
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-width: min(300px, calc(100% - 80px));
  padding: 8px 11px;
  background: color-mix(in srgb, var(--panel) 92%, transparent);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: var(--shadow-sm);
  font-size: 11px;
  color: var(--text-muted);
}
.top { display: flex; align-items: center; justify-content: space-between; gap: 8px; color: var(--text); }
.swatches { display: flex; gap: 12px; }
.lg { display: inline-flex; align-items: center; gap: 5px; white-space: nowrap; }
.sw { width: 10px; height: 10px; border-radius: 50%; }
.sw--added { background: color-mix(in srgb, var(--ok) 45%, transparent); }
.sw--changed { background: color-mix(in srgb, var(--warning) 45%, transparent); }
.note { margin: 0; line-height: 1.45; }
.full { color: var(--accent); }
</style>
