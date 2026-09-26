<script setup lang="ts">
import { computed, ref } from 'vue'

import type {
  DiffChange,
  DiffResult,
  DomainDiff,
  FieldChange,
  LineageDiff,
  RelationshipDiff,
  TableDiff,
} from '../api/types'
import { useI18n } from '../i18n'

const props = defineProps<{ result: DiffResult }>()
const emit = defineEmits<{ (e: 'open-table', id: string): void }>()
const { t } = useI18n()

type Filter = 'all' | DiffChange
const filters: Filter[] = ['all', 'added', 'removed', 'changed']
const filter = ref<Filter>('all')

interface Group {
  id: string
  domain?: DomainDiff
  tables: TableDiff[]
  relationships: RelationshipDiff[]
  lineage: LineageDiff[]
}

const domainOf = (tableId: string) => tableId.split('/')[0]

const groups = computed(() => {
  const keep = <T extends { change: DiffChange }>(xs: T[]) =>
    filter.value === 'all' ? xs : xs.filter((x) => x.change === filter.value)
  const m = new Map<string, Group>()
  const group = (id: string) => {
    let g = m.get(id)
    if (!g) {
      g = { id, tables: [], relationships: [], lineage: [] }
      m.set(id, g)
    }
    return g
  }
  for (const d of keep(props.result.domains)) group(d.id).domain = d
  for (const x of keep(props.result.tables)) group(x.domainId).tables.push(x)
  for (const x of keep(props.result.relationships)) group(domainOf(x.fromTableId)).relationships.push(x)
  for (const x of keep(props.result.lineage)) group(domainOf(x.tableId)).lineage.push(x)
  return [...m.values()].sort((a, b) => a.id.localeCompare(b.id))
})

function show(v: unknown): string {
  if (v === '' || v === null || v === undefined) return t('diff.value.empty')
  if (typeof v === 'boolean') return t(v ? 'diff.value.true' : 'diff.value.false')
  return String(v)
}

const expanded = ref<Set<string>>(new Set())
function toggle(key: string) {
  const next = new Set(expanded.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  expanded.value = next
}
const isProse = (f: FieldChange) => f.field === 'description'

const tagClass = (c: DiffChange) => `tag tag--${c}`

// A prose source leaves table and column empty.
const source = (l: LineageDiff) => [l.sourceTable, l.sourceColumn].filter(Boolean).join('.') || t('diff.value.empty')
</script>

<template>
  <section class="changes">
    <div class="filters" role="group" :aria-label="t('diff.filter.label')">
      <button
        v-for="f in filters"
        :key="f"
        class="fchip"
        :class="{ 'fchip--on': filter === f }"
        :aria-pressed="filter === f"
        @click="filter = f"
      >
        {{ t(`diff.filter.${f}`) }}
      </button>
    </div>

    <p v-if="!groups.length" class="empty muted">{{ t('diff.filter.empty') }}</p>

    <section v-for="g in groups" :key="g.id" class="group" :data-domain="g.id">
      <h2 class="ghead">{{ g.id }}</h2>

      <div v-if="g.domain" class="block">
        <h3 class="section-label">{{ t('diff.section.domain') }}</h3>
        <div class="entry">
          <span :class="tagClass(g.domain.change)">{{ t(`diff.change.${g.domain.change}`) }}</span>
          <ul class="fields">
            <li v-for="f in g.domain.fields" :key="f.field">
              <button
                v-if="isProse(f)"
                type="button"
                class="prose-toggle"
                :class="{ 'prose--open': expanded.has(`d:${g.id}:${f.field}`) }"
                :aria-expanded="expanded.has(`d:${g.id}:${f.field}`)"
                :title="t('diff.expand')"
                @click="toggle(`d:${g.id}:${f.field}`)"
              >
                <span class="fname">{{ f.field }}:</span> {{ show(f.from) }} → {{ show(f.to) }}
              </button>
              <template v-else><span class="fname">{{ f.field }}:</span> {{ show(f.from) }} → {{ show(f.to) }}</template>
            </li>
          </ul>
        </div>
      </div>

      <div v-if="g.tables.length" class="block">
        <h3 class="section-label">{{ t('diff.section.tables') }}</h3>
        <ul class="entries">
          <li v-for="tb in g.tables" :key="tb.id" class="entry" :data-change="tb.change">
            <div class="line">
              <span :class="tagClass(tb.change)">{{ t(`diff.change.${tb.change}`) }}</span>
              <button v-if="tb.change !== 'removed'" class="id linkish mono" @click="emit('open-table', tb.id)">
                {{ tb.id }}
              </button>
              <span v-else class="id mono">{{ tb.id }}</span>
            </div>
            <ul v-if="tb.fields.length" class="fields">
              <li v-for="f in tb.fields" :key="f.field">
                <button
                  v-if="isProse(f)"
                  type="button"
                  class="prose-toggle"
                  :class="{ 'prose--open': expanded.has(`t:${tb.id}:${f.field}`) }"
                  :aria-expanded="expanded.has(`t:${tb.id}:${f.field}`)"
                  :title="t('diff.expand')"
                  @click="toggle(`t:${tb.id}:${f.field}`)"
                >
                  <span class="fname">{{ f.field }}:</span> {{ show(f.from) }} → {{ show(f.to) }}
                </button>
                <template v-else><span class="fname">{{ f.field }}:</span> {{ show(f.from) }} → {{ show(f.to) }}</template>
              </li>
            </ul>
            <ul v-if="tb.columns.length" class="columns">
              <li v-for="c in tb.columns" :key="c.name">
                <div class="line">
                  <span :class="tagClass(c.change)">{{ t(`diff.change.${c.change}`) }}</span>
                  <span class="mono">{{ c.name }}</span>
                </div>
                <ul v-if="c.fields.length" class="fields">
                  <li v-for="f in c.fields" :key="f.field">
                    <button
                      v-if="isProse(f)"
                      type="button"
                      class="prose-toggle"
                      :class="{ 'prose--open': expanded.has(`c:${tb.id}:${c.name}:${f.field}`) }"
                      :aria-expanded="expanded.has(`c:${tb.id}:${c.name}:${f.field}`)"
                      :title="t('diff.expand')"
                      @click="toggle(`c:${tb.id}:${c.name}:${f.field}`)"
                    >
                      <span class="fname">{{ f.field }}:</span> {{ show(f.from) }} → {{ show(f.to) }}
                    </button>
                    <template v-else><span class="fname">{{ f.field }}:</span> {{ show(f.from) }} → {{ show(f.to) }}</template>
                  </li>
                </ul>
              </li>
            </ul>
          </li>
        </ul>
      </div>

      <div v-if="g.relationships.length" class="block">
        <h3 class="section-label">{{ t('diff.section.relationships') }}</h3>
        <ul class="entries">
          <li v-for="(r, i) in g.relationships" :key="i" class="entry" :data-change="r.change">
            <div class="line">
              <span :class="tagClass(r.change)">{{ t(`diff.change.${r.change}`) }}</span>
              <span class="mono">
                {{ r.fromTableId }}.{{ r.fromColumn }} → {{ r.toTableId || r.targetRef }}.{{ r.toColumn }}
              </span>
            </div>
            <ul v-if="r.fields.length" class="fields">
              <li v-for="f in r.fields" :key="f.field">
                <span class="fname">{{ f.field }}:</span> {{ show(f.from) }} → {{ show(f.to) }}
              </li>
            </ul>
          </li>
        </ul>
      </div>

      <div v-if="g.lineage.length" class="block">
        <h3 class="section-label">{{ t('diff.section.lineage') }}</h3>
        <ul class="entries">
          <li v-for="(l, i) in g.lineage" :key="i" class="entry" :data-change="l.change">
            <div class="line">
              <span :class="tagClass(l.change)">{{ t(`diff.change.${l.change}`) }}</span>
              <span class="mono">
                {{ l.tableId }}.{{ l.column }} ← {{ source(l) }}
              </span>
            </div>
            <ul v-if="l.fields.length" class="fields">
              <li v-for="f in l.fields" :key="f.field">
                <span class="fname">{{ f.field }}:</span> {{ show(f.from) }} → {{ show(f.to) }}
              </li>
            </ul>
          </li>
        </ul>
      </div>
    </section>
  </section>
</template>

<style scoped>
.filters { display: flex; gap: 5px; flex-wrap: wrap; margin-bottom: 14px; }
.fchip {
  padding: 3px 9px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-full);
  background: var(--panel);
  color: var(--text-muted);
  font-size: 11px;
  font-weight: 500;
}
.fchip:hover:not(.fchip--on) { color: var(--text); }
.fchip--on { background: var(--text); border-color: var(--text); color: var(--panel); }

.empty { padding: 28px 16px; text-align: center; font-size: 12.5px; }

.group { padding: 12px 0; border-top: 1px solid var(--border); }
.ghead { font-size: 14px; margin: 0 0 8px; }
.block { margin: 0 0 12px; }
.block .section-label { margin: 0 0 6px; }

ul { list-style: none; margin: 0; padding: 0; }
.entries > .entry { padding: 6px 0; border-bottom: 1px solid var(--border); }
.entries > .entry:last-child { border-bottom: none; }
.line { display: flex; align-items: baseline; gap: 8px; font-size: 12.5px; min-width: 0; }
.id { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.fields { padding: 3px 0 0 16px; font-size: 12px; color: var(--text-muted); }
.fields li { padding: 1px 0; overflow-wrap: anywhere; }
.fname { color: var(--text-faint); }
.columns { padding: 4px 0 0 16px; }
.columns > li { padding: 2px 0; }

.prose-toggle {
  display: block;
  width: 100%;
  padding: 0;
  border: none;
  background: none;
  font: inherit;
  color: inherit;
  text-align: left;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.prose--open { white-space: normal; }

.linkish { border: none; background: none; padding: 0; color: var(--text); text-align: left; }
.linkish:hover { color: var(--accent); text-decoration: underline; }

/* No soft pair for --ok exists, so it is tinted like .tag--role. */
.tag--added {
  background: color-mix(in srgb, var(--ok) 18%, var(--panel));
  color: color-mix(in srgb, var(--ok) 68%, var(--text));
}
.tag--removed { background: var(--danger-soft); color: var(--on-danger-soft); }
.tag--changed { background: var(--warning-soft); color: var(--on-warning-soft); }
</style>
