<script setup lang="ts">
/**
 * Documentation problems found during ingest, grouped by kind.
 *
 * This is the part of the tool that pays for itself: unresolved references,
 * conformed dimensions that have drifted apart, and join keys naming columns
 * that no document declares are all invisible when reading one file at a time.
 */
import { computed, ref } from 'vue'
import type { Diagnostic } from '../api/types'
import { codeLabel, splitDiagnostics } from '../diagnostics'
import { useI18n } from '../i18n'

const { t } = useI18n()

const props = defineProps<{ open: boolean; diagnostics: Diagnostic[] }>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'select', id: string): void
}>()

const severityFilter = ref<'all' | 'error' | 'warning' | 'info'>('all')

const filtered = computed(() =>
  severityFilter.value === 'all'
    ? props.diagnostics
    : props.diagnostics.filter((d) => d.severity === severityFilter.value),
)

const rank = { error: 0, warning: 1, info: 2 } as const

function group(items: Diagnostic[]) {
  const m = new Map<string, Diagnostic[]>()
  for (const d of items) {
    const list = m.get(d.code) ?? []
    list.push(d)
    m.set(d.code, list)
  }
  return [...m.entries()].sort((a, b) => {
    const sa = rank[a[1][0].severity] ?? 3
    const sb = rank[b[1][0].severity] ?? 3
    return sa !== sb ? sa - sb : b[1].length - a[1].length
  })
}

/**
 * Dropped documents come first regardless of severity. Their severity is low
 * on purpose — a stray README is harmless — but they are the only findings
 * that mean the model is incomplete, so they must not sort below a pile of
 * advisory notes.
 */
const sections = computed(() => {
  const { unparsed, findings } = splitDiagnostics(filtered.value)
  return [
    {
      key: 'unparsed',
      title: t('diagnostics.unparsed'),
      note: t('diagnostics.unparsed.note'),
      groups: group(unparsed),
    },
    {
      key: 'findings',
      title: t('diagnostics.findings'),
      note: t('diagnostics.findings.note'),
      groups: group(findings),
    },
  ].filter((sec) => sec.groups.length > 0)
})

const counts = computed(() => ({
  all: props.diagnostics.length,
  error: props.diagnostics.filter((d) => d.severity === 'error').length,
  warning: props.diagnostics.filter((d) => d.severity === 'warning').length,
  info: props.diagnostics.filter((d) => d.severity === 'info').length,
}))

const collapsed = ref<Set<string>>(new Set())
function toggle(code: string) {
  const next = new Set(collapsed.value)
  if (next.has(code)) next.delete(code)
  else next.add(code)
  collapsed.value = next
}

const label = codeLabel
</script>

<template>
  <div v-if="open" class="drawer" role="region" :aria-label="t('diagnostics.label')">
    <header class="head">
      <div>
        <h2>{{ t('diagnostics.title') }}</h2>
        <p class="faint tiny">{{ t('diagnostics.subtitle') }}</p>
      </div>
      <button
        class="btn btn--ghost btn--sm"
        :aria-label="t('diagnostics.close')"
        @click="emit('close')"
      >
        ✕
      </button>
    </header>

    <div class="filters">
      <button
        v-for="s in (['all', 'error', 'warning', 'info'] as const)"
        :key="s"
        class="fchip"
        :class="{ 'fchip--on': severityFilter === s, [`fchip--${s}`]: severityFilter === s }"
        @click="severityFilter = s"
      >
        {{ t(`diagnostics.severity.${s}`) }}<span class="fcount">{{ counts[s] }}</span>
      </button>
    </div>

    <div class="body">
      <p v-if="!sections.length" class="empty muted">{{ t('diagnostics.empty') }}</p>

      <div v-for="sec in sections" :key="sec.key" class="section">
        <header class="shead">
          <h3>{{ sec.title }}</h3>
          <p class="faint tiny">{{ sec.note }}</p>
        </header>

        <section v-for="[code, items] in sec.groups" :key="code" class="group">
          <button class="ghead" @click="toggle(code)">
            <span class="caret" :class="{ 'caret--open': !collapsed.has(code) }">▸</span>
            <span class="sev" :class="`sev--${items[0].severity}`" />
            <span class="gtitle">{{ label(code).title }}</span>
            <span class="gcount">{{ items.length }}</span>
          </button>

          <template v-if="!collapsed.has(code)">
            <p v-if="label(code).blurb" class="blurb faint">{{ label(code).blurb }}</p>
            <ul>
              <li v-for="(d, i) in items" :key="i">
                <button v-if="d.tableId" class="msg linkish" @click="emit('select', d.tableId)">
                  {{ d.message }}
                </button>
                <span v-else class="msg">{{ d.message }}</span>
                <span v-if="d.docPath" class="faint tiny path mono">{{ d.docPath }}</span>
              </li>
            </ul>
          </template>
        </section>
      </div>
    </div>
  </div>
</template>

<style scoped>
.drawer {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--panel);
  border-left: 1px solid var(--border);
  overflow: hidden;
}

.head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
  padding: 13px 15px 11px;
  border-bottom: 1px solid var(--border);
}
.head h2 { font-size: 14px; }
.tiny { font-size: 11px; }

.filters {
  display: flex;
  gap: 5px;
  padding: 10px 15px;
  border-bottom: 1px solid var(--border);
  flex-wrap: wrap;
}
.fchip {
  padding: 3px 9px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-full);
  background: var(--panel);
  color: var(--text-muted);
  font-size: 11px;
  font-weight: 500;
  text-transform: capitalize;
}
.fchip:hover { color: var(--text); }
.fchip--on { background: var(--text); border-color: var(--text); color: var(--panel); }
.fchip--error { background: var(--danger); border-color: var(--danger); color: var(--on-danger); }
.fchip--warning { background: var(--warning); border-color: var(--warning); color: var(--on-warning); }
.fchip--info { background: var(--info); border-color: var(--info); color: var(--on-info); }
.fcount { margin-left: 5px; opacity: 0.7; }

.body { flex: 1; overflow-y: auto; padding: 6px 0 24px; }
.empty { padding: 28px 16px; text-align: center; font-size: 12.5px; }

.shead {
  padding: 12px 15px 8px;
  border-bottom: 1px solid var(--border);
}
.shead h3 { font-size: 11px; text-transform: uppercase; letter-spacing: 0.06em; }
.shead p { margin: 3px 0 0; line-height: 1.45; }

.group { border-bottom: 1px solid var(--border); }
.section:last-child .group:last-child { border-bottom: none; }

.ghead {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 10px 15px;
  border: none;
  background: none;
  color: var(--text);
  text-align: left;
  font-size: 12.5px;
  font-weight: 600;
}
.ghead:hover { background: var(--bg-sunken); }
.caret { font-size: 9px; color: var(--text-faint); transition: transform var(--dur) var(--ease); }
.caret--open { transform: rotate(90deg); }
.gtitle { flex: 1; }
.gcount {
  padding: 0 6px;
  border-radius: var(--radius-full);
  background: var(--bg-sunken);
  font-size: 10.5px;
  color: var(--text-muted);
  font-weight: 600;
}

.sev { width: 7px; height: 7px; border-radius: 50%; flex: none; }
.sev--error { background: var(--danger); }
.sev--warning { background: var(--warning); }
.sev--info { background: var(--info); }

.blurb { padding: 0 15px 8px 32px; font-size: 11.5px; line-height: 1.5; }

.group ul { list-style: none; margin: 0; padding: 0 15px 12px 32px; }
.group li { padding: 5px 0; border-bottom: 1px solid var(--border); }
.group li:last-child { border-bottom: none; }

.msg {
  display: block;
  font-size: 12px;
  line-height: 1.5;
  text-align: left;
  color: var(--text-muted);
}
.linkish {
  border: none;
  background: none;
  padding: 0;
  color: var(--text-muted);
  width: 100%;
}
.linkish:hover { color: var(--accent); text-decoration: underline; }
.path { display: block; margin-top: 2px; }
</style>
