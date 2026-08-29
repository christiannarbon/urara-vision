<script setup lang="ts">
/** Left rail: snapshot summary, layout, role and domain filters, table list. */
import { computed, ref } from 'vue'
import type { Domain, Snapshot, TableSummary } from '../api/types'
import { useTheme } from '../composables/useTheme'
import { useRolePalette } from '../composables/useRolePalette'
import { LAYOUTS, type LayoutMode } from '../graph/layout'
import { canvasTheme, domainColor, domainIndex, paletteFamily } from '../graph/palette'
import { roleSpec, rolesPresent } from '../graph/roles'

const props = defineProps<{
  snapshot: Snapshot | null
  domains: Domain[]
  tables: TableSummary[]
  activeDomains: string[]
  activeKinds: string[]
  showSources: boolean
  crossDomainOnly: boolean
  viewMode: 'overview' | 'focus'
  layoutMode: LayoutMode
  focusDepth: number
  selectedId: string | null
}>()

const emit = defineEmits<{
  (e: 'toggle-domain', id: string): void
  (e: 'toggle-kind', kind: string): void
  (e: 'set-sources', v: boolean): void
  (e: 'set-cross-domain', v: boolean): void
  (e: 'set-view-mode', v: 'overview' | 'focus'): void
  (e: 'set-layout-mode', v: LayoutMode): void
  (e: 'set-focus-depth', v: number): void
  (e: 'clear'): void
  (e: 'select', id: string): void
}>()

const tableFilter = ref('')

const { colorOf } = useRolePalette()

/**
 * Role filters come from the roles the snapshot actually contains rather than
 * from a fixed pair. A Data Vault offers hubs and satellites here; a star
 * schema still offers exactly fact and dimension, because that is all it has.
 */
const roles = computed(() => rolesPresent(props.tables.map((t) => t.kind)))

/** The spec for one table's role, for the silhouette its dot is drawn with. */
const roleOf = (kind: string) => roleSpec(kind)

const visibleTables = computed(() => {
  const q = tableFilter.value.trim().toLowerCase()
  return props.tables.filter((t) => {
    if (props.activeDomains.length && !props.activeDomains.includes(t.domainId)) return false
    if (props.activeKinds.length && !props.activeKinds.includes(t.kind)) return false
    if (q && !t.name.toLowerCase().includes(q) && !t.domainId.toLowerCase().includes(q)) return false
    return true
  })
})

/** Groups the table list by domain so it reads like the directory it came from. */
const grouped = computed(() => {
  const m = new Map<string, TableSummary[]>()
  for (const t of visibleTables.value) {
    const list = m.get(t.domainId) ?? []
    list.push(t)
    m.set(t.domainId, list)
  }
  return [...m.entries()].sort((a, b) => a[0].localeCompare(b[0]))
})

const hasFilters = computed(
  () => props.activeDomains.length > 0 || props.activeKinds.length > 0 || props.crossDomainOnly,
)

/**
 * The same hue the graph gives each domain, so the rail reads as one legend.
 *
 * Keyed off the canvas colour exactly as the hulls are, not off the app's
 * light/dark mode: on a painting whose light canvas is dark -- Matisse -- the
 * two would otherwise disagree, and a legend that does not match the thing it
 * labels is worse than no legend.
 */
const { art } = useTheme()
const domainSwatch = computed(() => {
  void art.value // re-resolves the palette when the theme changes
  const canvas = getComputedStyle(document.documentElement)
    .getPropertyValue('--graph-bg')
    .trim()
  const theme = canvasTheme(canvas || '#ffffff')
  const family = paletteFamily(art.value)
  const m = new Map<string, string>()
  for (const [id, slot] of domainIndex(props.domains.map((d) => d.id))) {
    m.set(id, domainColor(slot, theme, family).swatch)
  }
  return m
})

const collapsed = ref<Set<string>>(new Set())
function toggleGroup(id: string) {
  const next = new Set(collapsed.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  collapsed.value = next
}
</script>

<template>
  <aside class="rail" aria-label="Filters and tables">
    <section v-if="snapshot" class="block stats">
      <div class="stat">
        <strong>{{ snapshot.stats.tables }}</strong><span>tables</span>
      </div>
      <div class="stat">
        <strong>{{ snapshot.stats.domains }}</strong><span>domains</span>
      </div>
      <div class="stat">
        <strong>{{ snapshot.stats.columns }}</strong><span>columns</span>
      </div>
      <div class="stat">
        <strong>{{ snapshot.stats.sourceTables }}</strong><span>sources</span>
      </div>
    </section>

    <section class="block">
      <h3 class="section-label">View</h3>
      <div class="seg" role="group" aria-label="View mode">
        <button
          class="seg-btn"
          :class="{ 'seg-btn--on': viewMode === 'overview' }"
          @click="emit('set-view-mode', 'overview')"
        >
          Whole model
        </button>
        <button
          class="seg-btn"
          :class="{ 'seg-btn--on': viewMode === 'focus' }"
          :disabled="!selectedId"
          :title="selectedId ? 'Show only the selected table and its neighbours' : 'Select a table first'"
          @click="emit('set-view-mode', 'focus')"
        >
          Focused
        </button>
      </div>

      <div class="seg seg--layout" role="group" aria-label="Layout">
        <button
          v-for="l in LAYOUTS"
          :key="l.id"
          class="seg-btn"
          :class="{ 'seg-btn--on': layoutMode === l.id }"
          :title="l.hint"
          @click="emit('set-layout-mode', l.id)"
        >
          {{ l.label }}
        </button>
      </div>

      <label v-if="viewMode === 'focus'" class="depth">
        <span class="faint">Depth</span>
        <input
          type="range"
          min="1"
          max="3"
          :value="focusDepth"
          @input="emit('set-focus-depth', Number(($event.target as HTMLInputElement).value))"
        />
        <span class="mono">{{ focusDepth }}</span>
      </label>

      <label class="check">
        <input
          type="checkbox"
          :checked="showSources"
          @change="emit('set-sources', ($event.target as HTMLInputElement).checked)"
        />
        <span>Show upstream source models</span>
      </label>

      <label class="check">
        <input
          type="checkbox"
          :checked="crossDomainOnly"
          :disabled="viewMode === 'focus'"
          @change="emit('set-cross-domain', ($event.target as HTMLInputElement).checked)"
        />
        <span>Cross-domain joins only</span>
      </label>
    </section>

    <section class="block">
      <div class="block-head">
        <h3 class="section-label">Filters</h3>
        <button v-if="hasFilters" class="btn btn--ghost btn--sm" @click="emit('clear')">Clear</button>
      </div>

      <div class="chips">
        <button
          v-for="r in roles"
          :key="r.id"
          class="chip chip--role"
          :class="{ 'chip--on': activeKinds.includes(r.id) }"
          :style="{ '--swatch': colorOf(r.id) }"
          :title="r.family === 'other' ? `Role read from the documents: ${r.id}` : r.label"
          @click="emit('toggle-kind', r.id)"
        >
          <i class="swatch" :class="`swatch--${r.swatch}`" />
          {{ r.label }}
        </button>
      </div>

      <div class="chips domains">
        <button
          v-for="d in domains"
          :key="d.id"
          class="chip chip--domain"
          :class="{ 'chip--on': activeDomains.includes(d.id) }"
          :style="{ '--swatch': domainSwatch.get(d.id) }"
          :title="d.title"
          @click="emit('toggle-domain', d.id)"
        >
          <i class="swatch" />
          {{ d.id }}
          <span class="chip-count">{{ d.tableCount }}</span>
        </button>
      </div>
    </section>

    <section class="block grow">
      <h3 class="section-label">Tables ({{ visibleTables.length }})</h3>
      <input
        v-model="tableFilter"
        class="input"
        type="search"
        placeholder="Filter tables…"
        aria-label="Filter tables"
      />

      <div class="list">
        <div v-for="[domain, items] in grouped" :key="domain" class="group">
          <button
            class="group-head"
            :style="{ '--swatch': domainSwatch.get(domain) }"
            @click="toggleGroup(domain)"
          >
            <span class="caret" :class="{ 'caret--open': !collapsed.has(domain) }">▸</span>
            <i class="swatch" />
            <span class="mono">{{ domain }}</span>
            <span class="faint">{{ items.length }}</span>
          </button>
          <ul v-if="!collapsed.has(domain)" class="items">
            <li v-for="t in items" :key="t.id">
              <button
                class="item"
                :class="{ 'item--on': selectedId === t.id }"
                :title="t.grain || t.description"
                @click="emit('select', t.id)"
              >
                <i
                  class="dot"
                  :class="`dot--${roleOf(t.kind).swatch}`"
                  :style="{ background: colorOf(t.kind) }"
                />
                <span class="mono item-name">{{ t.name }}</span>
                <span v-if="t.conformed" class="cf" title="Conformed: also defined in other domains">
                  ↔
                </span>
              </button>
            </li>
          </ul>
        </div>
        <p v-if="!grouped.length" class="muted small pad">No tables match.</p>
      </div>
    </section>
  </aside>
</template>

<style scoped>
.rail {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--panel);
  border-right: 1px solid var(--border);
  overflow: hidden;
}

.block {
  padding: 12px 14px;
  border-bottom: 1px solid var(--border);
}
.block.grow {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  border-bottom: none;
}
.block-head { display: flex; align-items: center; justify-content: space-between; }
.block-head .section-label { margin: 0; }

.stats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 6px;
  text-align: center;
}
.stat { display: flex; flex-direction: column; }
.stat strong { font-size: 15px; font-variant-numeric: tabular-nums; }
.stat span { font-size: 10px; color: var(--text-faint); text-transform: uppercase; letter-spacing: 0.04em; }

.seg {
  display: flex;
  padding: 2px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg-sunken);
  margin-bottom: 10px;
}
.seg-btn {
  flex: 1;
  padding: 5px 8px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  font-size: 12px;
  font-weight: 500;
  transition: background var(--dur) var(--ease), color var(--dur) var(--ease);
}
.seg-btn--on { background: var(--panel); color: var(--text); box-shadow: var(--shadow-sm); }
.seg-btn:disabled { opacity: 0.45; cursor: not-allowed; }

.depth {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
  font-size: 12px;
}
.depth input { flex: 1; accent-color: var(--accent); }

.check {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 3px 0;
  font-size: 12px;
  color: var(--text-muted);
  cursor: pointer;
}
.check input { accent-color: var(--accent); }
.check:has(input:disabled) { opacity: 0.5; cursor: not-allowed; }

.chips { display: flex; flex-wrap: wrap; gap: 5px; margin-top: 8px; }
.chip {
  padding: 3px 8px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-full);
  background: var(--panel);
  color: var(--text-muted);
  font-size: 11px;
  font-weight: 500;
  transition: all var(--dur) var(--ease);
}
.chip:hover { border-color: var(--text-faint); color: var(--text); }
.seg--layout { margin-top: 6px; }
.chip--on {
  background: var(--accent);
  border-color: var(--accent);
  color: var(--accent-contrast);
}
/* A selected role chip wears its own colour rather than the accent, so the
   rail and the graph agree on what a role looks like. The label goes to the
   panel colour: the role hues are mid-lightness by construction (see
   graph/roles.ts) and neither white nor the body text reads on all of them. */
.chip--role {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}
.chip--role.chip--on {
  background: var(--swatch, var(--accent));
  border-color: var(--swatch, var(--accent));
  color: var(--panel);
}
.chip--domain {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-family: var(--font-mono);
  font-size: 10.5px;
}

/* Matches the cluster hull the domain gets in the graph. */
.swatch {
  width: 8px;
  height: 8px;
  flex: none;
  border-radius: 2px;
  background: var(--swatch, var(--text-faint));
}
.chip--on .swatch { background: var(--accent-contrast); }
/* The role chips' swatches echo the node shapes; see the graph legend. */
.swatch--round { border-radius: 50%; }
.swatch--angular { border-radius: 0; clip-path: polygon(50% 0, 100% 50%, 50% 100%, 0 50%); }
.chip-count { margin-left: 4px; opacity: 0.65; }
.domains { max-height: 132px; overflow-y: auto; }

.list { flex: 1; overflow-y: auto; margin-top: 8px; min-height: 0; }
.group { margin-bottom: 2px; }
.group-head {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 4px 2px;
  border: none;
  background: none;
  color: var(--text-muted);
  font-size: 11px;
  text-align: left;
}
.group-head:hover { color: var(--text); }
.group-head .swatch { border-radius: 2px; }
.caret { display: inline-block; transition: transform var(--dur) var(--ease); font-size: 9px; }
.caret--open { transform: rotate(90deg); }

.items { list-style: none; margin: 0; padding: 0 0 4px 8px; }
.item {
  display: flex;
  align-items: center;
  gap: 7px;
  width: 100%;
  padding: 3px 7px;
  border: none;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--text);
  font-size: 12px;
  text-align: left;
  transition: background var(--dur) var(--ease);
}
.item:hover { background: var(--bg-sunken); }
.item--on { background: var(--accent); color: var(--accent-contrast); }
.item-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.dot { width: 7px; height: 7px; flex: none; border-radius: 2px; }
.dot--round { border-radius: 50%; }
.dot--angular { border-radius: 0; clip-path: polygon(50% 0, 100% 50%, 50% 100%, 0 50%); }
/* The selected row is filled with the accent, and a role colour on top of it
   reads as neither. The dot goes flat for the one row that is selected. */
.item--on .dot { background: var(--accent-contrast) !important; }

.cf { margin-left: auto; font-size: 11px; opacity: 0.6; }
.pad { padding: 8px 2px; }
.small { font-size: 12px; }
</style>
