<script setup lang="ts">
/** Detail pane for the selected table: overview, columns, relationships,
 *  lineage and caveats. */
import { computed, ref, watch } from 'vue'
import type { TableResponse } from '../api/types'
import { useRolePalette } from '../composables/useRolePalette'
import { roleSpec } from '../graph/roles'
import { useI18n } from '../i18n'

const { t, tn } = useI18n()

const props = defineProps<{
  detail: TableResponse | null
  loading: boolean
  selectedId: string | null
}>()

const { colorOf } = useRolePalette()

/**
 * The role badge. Fact and dimension keep the theme's hand-tuned soft pair;
 * everything else tints itself from the colour the role resolved to, so a
 * vocabulary this app has never seen still gets a readable badge.
 */
const role = computed(() => (props.detail ? roleSpec(props.detail.table.kind) : null))
const roleTagClass = computed(() => {
  const id = role.value?.id
  return id === 'fact' || id === 'dimension' ? `tag--${id}` : 'tag--role'
})

const emit = defineEmits<{
  (e: 'navigate', id: string): void
  (e: 'focus', id: string): void
  (e: 'close'): void
}>()

type Tab = 'overview' | 'columns' | 'relationships' | 'lineage'

/** The tab strip, in order, with the catalogue key each one reads under. */
const TABS = [
  { id: 'overview', key: 'detail.tab.overview' },
  { id: 'columns', key: 'detail.tab.columns' },
  // The tab has always said "joins" rather than "relationships": it is the
  // shorter word for the same thing, and the strip is narrow.
  { id: 'relationships', key: 'detail.tab.joins' },
  { id: 'lineage', key: 'detail.tab.lineage' },
] as const

const tab = ref<Tab>('overview')
const columnFilter = ref('')

// A new table starts on its overview, and the column filter should not leak
// between tables.
watch(
  () => props.detail?.table.id,
  () => {
    tab.value = 'overview'
    columnFilter.value = ''
  },
)

const table = computed(() => props.detail?.table ?? null)

const keyColumns = computed(() => table.value?.columns.filter((c) => c.isPk || c.isFk) ?? [])

const filteredColumns = computed(() => {
  const cols = table.value?.columns ?? []
  const q = columnFilter.value.trim().toLowerCase()
  if (!q) return cols
  return cols.filter(
    (c) =>
      c.name.toLowerCase().includes(q) ||
      c.type.toLowerCase().includes(q) ||
      c.description.toLowerCase().includes(q),
  )
})

/** Column lineage keyed by column, so the columns tab can show provenance
 *  inline rather than making the reader cross-reference two tables. */
const lineageByColumn = computed(() => {
  const m = new Map<string, { source: string; column: string; notes: string; derived: boolean }>()
  for (const l of props.detail?.table.columnLineage ?? []) {
    m.set(l.column, {
      source: l.sourceTable,
      column: l.sourceColumn,
      notes: l.notes,
      derived: l.derived,
    })
  }
  return m
})

const unresolvedRelationships = computed(
  () =>
    props.detail?.table.relationships.filter(
      (r) => r.resolution === 'unresolved' || r.resolution === 'narrative',
    ) ?? [],
)

const resolvedRelationships = computed(
  () =>
    props.detail?.table.relationships.filter(
      (r) => r.resolution === 'local' || r.resolution === 'conformed',
    ) ?? [],
)

function shortId(id: string): string {
  const i = id.indexOf('/')
  return i >= 0 ? id.slice(i + 1) : id
}

function domainOf(id: string): string {
  const i = id.indexOf('/')
  return i >= 0 ? id.slice(0, i) : ''
}
</script>

<template>
  <aside class="detail" :aria-label="t('detail.label')">
    <div v-if="loading" class="state">
      <div class="spinner" aria-hidden="true" />
      <span class="muted">{{ t('detail.loading') }}</span>
    </div>

    <div v-else-if="!table && selectedId" class="state">
      <p><strong>{{ shortId(selectedId) }}</strong></p>
      <p class="muted">{{ t('detail.sourceOnly') }}</p>
    </div>

    <div v-else-if="!table" class="state">
      <p class="muted">{{ t('detail.empty') }}</p>
      <p class="faint hint">{{ t('detail.empty.hint') }}</p>
    </div>

    <template v-else>
      <header class="head">
        <div class="head-top">
          <div class="titles">
            <h2 class="mono name">{{ table.name }}</h2>
            <div class="chips">
              <span
                v-if="role"
                class="tag"
                :class="roleTagClass"
                :style="{ '--role': colorOf(role.id) }"
                :title="table.kindRaw || role.label"
                >{{ role.label }}</span
              >
              <span v-if="table.conformed" class="tag tag--conformed">
                {{ t('detail.conformed') }}
              </span>
              <span class="tag tag--muted">{{ table.domainId }}</span>
            </div>
          </div>
          <button
            class="btn btn--ghost btn--sm close"
            :aria-label="t('detail.close')"
            @click="emit('close')"
          >
            ✕
          </button>
        </div>
        <button class="btn btn--sm focus-btn" @click="emit('focus', table.id)">
          {{ t('detail.focusHere') }}
        </button>
      </header>

      <nav class="tabs" role="tablist">
        <button
          v-for="item in TABS"
          :key="item.id"
          class="tab"
          :class="{ 'tab--active': tab === item.id }"
          role="tab"
          :aria-selected="tab === item.id"
          @click="tab = item.id"
        >
          {{ t(item.key) }}
          <span v-if="item.id === 'columns'" class="count">{{ table.columns.length }}</span>
          <span v-else-if="item.id === 'relationships'" class="count">
            {{ table.relationships.length }}
          </span>
          <span v-else-if="item.id === 'lineage'" class="count">
            {{ table.columnLineage.length }}
          </span>
        </button>
      </nav>

      <div class="body">
        <!-- Overview -->
        <section v-if="tab === 'overview'" class="pane">
          <p v-if="table.description" class="desc">{{ table.description }}</p>

          <dl class="props">
            <template v-if="table.grain">
              <dt>{{ t('detail.grain') }}</dt>
              <dd>{{ table.grain }}</dd>
            </template>
            <template v-if="table.kindRaw">
              <dt>{{ t('detail.type') }}</dt>
              <dd>{{ table.kindRaw }}</dd>
            </template>
            <template v-if="table.domainLabel">
              <dt>{{ t('detail.domain') }}</dt>
              <dd>{{ table.domainLabel }}</dd>
            </template>
            <template v-if="table.updateFrequency">
              <dt>{{ t('detail.updated') }}</dt>
              <dd>{{ table.updateFrequency }}</dd>
            </template>
            <template v-if="table.layer">
              <dt>{{ t('detail.layer') }}</dt>
              <dd>{{ table.layer }}</dd>
            </template>
            <dt>{{ t('detail.source') }}</dt>
            <dd class="mono faint">{{ table.docPath }}</dd>
          </dl>

          <template v-if="keyColumns.length">
            <h3 class="section-label">{{ t('detail.keys') }}</h3>
            <ul class="keys">
              <li v-for="c in keyColumns" :key="c.name">
                <code>{{ c.name }}</code>
                <span class="tag tag--muted">{{ c.isPk ? 'PK' : 'FK' }}</span>
                <span class="faint">{{ c.type }}</span>
              </li>
            </ul>
          </template>

          <template v-if="table.conformed && table.conformedIn?.length">
            <h3 class="section-label">{{ t('detail.alsoDefinedIn') }}</h3>
            <p class="muted small">{{ t('detail.alsoDefinedIn.note') }}</p>
            <ul class="links">
              <li v-for="id in table.conformedIn" :key="id">
                <button class="linkish" @click="emit('navigate', id)">
                  <span class="faint">{{ domainOf(id) }}/</span>{{ shortId(id) }}
                </button>
              </li>
            </ul>
          </template>

          <template v-if="table.notes.length">
            <h3 class="section-label">{{ t('detail.notes') }}</h3>
            <ul class="notes">
              <li v-for="(n, i) in table.notes" :key="i">{{ n }}</li>
            </ul>
          </template>
        </section>

        <!-- Columns -->
        <section v-else-if="tab === 'columns'" class="pane">
          <input
            v-model="columnFilter"
            class="input filter"
            type="search"
            :placeholder="t('detail.columns.filter')"
            :aria-label="t('detail.columns.filter.label')"
          />
          <p v-if="!filteredColumns.length" class="muted small">
            {{ t('detail.columns.noMatches') }}
          </p>
          <ul v-else class="cols-list">
            <li v-for="c in filteredColumns" :key="c.name" class="col">
              <div class="col-head">
                <code class="col-name">{{ c.name }}</code>
                <span v-if="c.isPk" class="tag tag--muted">PK</span>
                <span v-else-if="c.isFk" class="tag tag--muted">FK</span>
                <span class="faint mono col-type">{{ c.type }}</span>
              </div>
              <p v-if="c.description" class="col-desc">{{ c.description }}</p>
              <p v-if="lineageByColumn.get(c.name)" class="col-src faint tiny">
                {{ t('detail.columns.from') }}
                <code>{{ lineageByColumn.get(c.name)!.source }}</code>.<code>{{
                  lineageByColumn.get(c.name)!.column
                }}</code>
                <span v-if="lineageByColumn.get(c.name)!.derived" class="tag tag--info key">
                  {{ t('detail.columns.derived') }}
                </span>
              </p>
            </li>
          </ul>
        </section>

        <!-- Relationships -->
        <section v-else-if="tab === 'relationships'" class="pane">
          <p v-if="table.relationshipNote" class="desc small">{{ table.relationshipNote }}</p>

          <h3 class="section-label">{{ t('detail.joins.declared') }}</h3>
          <p v-if="!resolvedRelationships.length && !unresolvedRelationships.length" class="muted small">
            {{ t('detail.joins.none') }}
          </p>
          <ul class="rels">
            <li v-for="r in resolvedRelationships" :key="r.id">
              <button class="rel-target linkish" @click="emit('navigate', r.toTableId!)">
                {{ shortId(r.toTableId!) }}
              </button>
              <span v-if="r.resolution === 'conformed'" class="tag tag--warning">
                {{ t('detail.joins.crossDomain') }}
              </span>
              <div class="rel-meta mono faint">
                {{ r.fromColumn }} → {{ r.toColumn }}
                <span class="sep">·</span>{{ r.cardinality }}
              </div>
              <div v-if="r.candidates?.length && r.candidates.length > 1" class="faint tiny">
                {{
                  t('detail.joins.boundTo', {
                    target: r.toTableId!,
                    alternatives: r.candidates.filter((c) => c !== r.toTableId).join(', '),
                  })
                }}
              </div>
            </li>
          </ul>

          <template v-if="unresolvedRelationships.length">
            <h3 class="section-label warn">{{ t('detail.joins.unresolved') }}</h3>
            <ul class="rels">
              <li v-for="r in unresolvedRelationships" :key="r.id">
                <span class="mono">{{ r.targetRef }}</span>
                <span class="tag" :class="r.resolution === 'unresolved' ? 'tag--danger' : 'tag--info'">
                  {{
                    r.resolution === 'unresolved'
                      ? t('detail.joins.noDocument')
                      : t('detail.joins.prose')
                  }}
                </span>
                <div class="rel-meta mono faint">{{ r.joinKeyRaw || '—' }}</div>
              </li>
            </ul>
          </template>

          <template v-if="detail?.incoming.length">
            <h3 class="section-label">{{ t('detail.joins.referencedBy') }}</h3>
            <ul class="rels">
              <li v-for="r in detail.incoming" :key="r.tableId + r.fromColumn">
                <button class="rel-target linkish" @click="emit('navigate', r.tableId)">
                  {{ r.name }}
                </button>
                <span class="tag tag--muted">{{ r.domainId }}</span>
                <div class="rel-meta mono faint">
                  {{ r.fromColumn }} → {{ r.toColumn }}
                  <span class="sep">·</span>{{ r.cardinality }}
                </div>
              </li>
            </ul>
          </template>
        </section>

        <!-- Lineage -->
        <section v-else class="pane">
          <h3 class="section-label">{{ t('detail.lineage.upstream') }}</h3>
          <p v-if="!detail?.upstream.length" class="muted small">
            {{ t('detail.lineage.none') }}
          </p>
          <ul v-else class="lineage">
            <li v-for="u in detail.upstream" :key="u.id">
              <div class="lin-head">
                <code class="mono">{{ u.label }}</code>
                <span class="tag tag--muted">
                  {{ tn('detail.lineage.columns', u.columnCount) }}
                </span>
              </div>
              <div v-if="u.dataset" class="faint tiny mono">{{ u.dataset }}</div>
              <div class="cols">
                <code v-for="c in u.columns.slice(0, 12)" :key="c" class="pill">{{ c }}</code>
                <span v-if="u.columns.length > 12" class="faint tiny">
                  {{ t('detail.lineage.more', { n: u.columns.length - 12 }) }}
                </span>
              </div>
            </li>
          </ul>

          <template v-if="detail?.siblings.length">
            <h3 class="section-label">{{ t('detail.siblings') }}</h3>
            <p class="muted small">{{ t('detail.siblings.note') }}</p>
            <ul class="rels">
              <li v-for="s in detail.siblings.slice(0, 20)" :key="s.id">
                <button class="rel-target linkish" @click="emit('navigate', s.id)">
                  {{ s.label }}
                </button>
                <span class="tag tag--muted">{{ s.domainId }}</span>
                <div class="rel-meta faint tiny">
                  {{ tn('detail.siblings.shared', s.columns.length) }}
                </div>
              </li>
            </ul>
          </template>
        </section>
      </div>
    </template>
  </aside>
</template>

<style scoped>
.detail {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--panel);
  border-left: 1px solid var(--border);
  overflow: hidden;
}

.state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  height: 100%;
  padding: 32px 24px;
  text-align: center;
  font-size: 13px;
}
.hint { font-size: 12px; }

.spinner {
  width: 20px;
  height: 20px;
  border: 2px solid var(--border-strong);
  border-top-color: var(--accent);
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }

.head {
  padding: 14px 16px 10px;
  border-bottom: 1px solid var(--border);
}
.head-top { display: flex; align-items: flex-start; gap: 8px; }
.titles { flex: 1; min-width: 0; }
.name {
  font-size: 15px;
  font-weight: 600;
  word-break: break-word;
}
.chips { display: flex; flex-wrap: wrap; gap: 5px; margin-top: 6px; }
.close { flex: none; padding: 2px 6px; }
.focus-btn { margin-top: 10px; width: 100%; justify-content: center; }

.tabs {
  display: flex;
  gap: 1px;
  padding: 6px 8px 0;
  border-bottom: 1px solid var(--border);
}
.tab {
  padding: 6px 7px;
  border: none;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--text-muted);
  font-size: 12px;
  font-weight: 500;
  text-transform: capitalize;
  white-space: nowrap;
  transition: color var(--dur) var(--ease), border-color var(--dur) var(--ease);
}
.tab:hover { color: var(--text); }
.tab--active { color: var(--accent); border-bottom-color: var(--accent); }
.count {
  margin-left: 4px;
  padding: 0 5px;
  border-radius: var(--radius-full);
  background: var(--bg-sunken);
  font-size: 10px;
  color: var(--text-faint);
}

.body { flex: 1; overflow-y: auto; }
.pane { padding: 14px 16px 28px; }

.desc { font-size: 13px; line-height: 1.55; }
.small { font-size: 12px; }
.tiny { font-size: 11px; }
.nowrap { white-space: nowrap; }

.props {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 5px 14px;
  margin: 14px 0 0;
  font-size: 12.5px;
}
.props dt { color: var(--text-faint); font-weight: 600; white-space: nowrap; }
.props dd { margin: 0; word-break: break-word; }

.section-label { margin: 20px 0 8px; }
.section-label.warn { color: var(--danger); }

.keys, .notes, .rels, .links, .lineage {
  list-style: none;
  margin: 0;
  padding: 0;
}
.keys li {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 4px 0;
  font-size: 12.5px;
}
.notes li {
  position: relative;
  padding: 4px 0 4px 15px;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--text-muted);
}
.notes li::before {
  content: '';
  position: absolute;
  left: 3px;
  top: 11px;
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: var(--text-faint);
}

.rels li {
  padding: 8px 0;
  border-bottom: 1px solid var(--border);
}
.rels li:last-child { border-bottom: none; }
.rel-meta { font-size: 11px; margin-top: 3px; }
.sep { margin: 0 5px; }

.linkish {
  border: none;
  background: none;
  padding: 0;
  color: var(--accent);
  font-family: var(--font-mono);
  font-size: 12.5px;
  font-weight: 500;
}
.linkish:hover { text-decoration: underline; }
.rel-target { margin-right: 6px; }

.links li { padding: 3px 0; }

.filter { margin-bottom: 10px; }
.key { margin-left: 5px; }

.cols-list { list-style: none; margin: 0; padding: 0; }
.col {
  padding: 8px 0;
  border-bottom: 1px solid var(--border);
}
.col:last-child { border-bottom: none; }
.col-head {
  display: flex;
  align-items: baseline;
  gap: 6px;
  flex-wrap: wrap;
}
.col-name {
  font-size: 12.5px;
  font-weight: 500;
  word-break: break-all;
}
.col-type { font-size: 10.5px; margin-left: auto; }
.col-desc {
  margin: 3px 0 0;
  font-size: 12px;
  line-height: 1.45;
  color: var(--text-muted);
}
.col-src { margin: 3px 0 0; word-break: break-all; }

.lineage li {
  padding: 9px 0;
  border-bottom: 1px solid var(--border);
}
.lineage li:last-child { border-bottom: none; }
.lin-head { display: flex; align-items: center; gap: 7px; flex-wrap: wrap; }
.cols { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 6px; }
.pill {
  padding: 1px 6px;
  border-radius: var(--radius-sm);
  background: var(--bg-sunken);
  font-size: 10.5px;
  color: var(--text-muted);
}
</style>
