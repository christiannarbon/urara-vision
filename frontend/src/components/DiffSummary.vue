<script setup lang="ts">
import { computed } from 'vue'

import type { DiffSummary } from '../api/types'
import { useI18n } from '../i18n'

const props = defineProps<{ summary: DiffSummary }>()
const { t } = useI18n()

const categories = ['domains', 'tables', 'columns', 'relationships', 'lineage'] as const

const rows = computed(() =>
  categories
    .map((c) => ({ key: c, ...props.summary[c] }))
    .filter((r) => r.added + r.removed + r.changed > 0),
)
</script>

<template>
  <section class="summary" :aria-label="t('diff.summary')">
    <p v-if="!rows.length" class="none muted">{{ t('diff.none') }}</p>
    <ul v-else>
      <li v-for="r in rows" :key="r.key" class="row">
        <span class="cat">{{ t(`diff.category.${r.key}`) }}</span>
        <span v-if="r.added" class="n n--added" :title="t('diff.count.added', { n: r.added })">+{{ r.added }}</span>
        <span v-if="r.removed" class="n n--removed" :title="t('diff.count.removed', { n: r.removed })">−{{ r.removed }}</span>
        <span v-if="r.changed" class="n n--changed" :title="t('diff.count.changed', { n: r.changed })">~{{ r.changed }}</span>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.summary ul { list-style: none; margin: 0; padding: 0; display: flex; flex-wrap: wrap; gap: 6px 18px; }
.row { display: flex; align-items: baseline; gap: 8px; font-size: 12.5px; }
.cat { color: var(--text-muted); }
.n { font-weight: 600; font-variant-numeric: tabular-nums; }
.n--added { color: var(--ok); }
.n--removed { color: var(--danger); }
.n--changed { color: var(--warning); }
.none { margin: 0; font-size: 12.5px; }
</style>
