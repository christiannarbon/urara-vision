<script setup lang="ts">
/** Command-palette style search over tables and their columns. */
import { computed, nextTick, ref, watch } from 'vue'
import { api } from '../api/client'
import type { SearchHit } from '../api/types'
import { useRolePalette } from '../composables/useRolePalette'
import { roleSpec } from '../graph/roles'
import { useI18n } from '../i18n'

const { t } = useI18n()

const props = defineProps<{ open: boolean; snapshotId: string | null }>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'select', id: string): void
}>()

const { colorOf } = useRolePalette()
const roleOf = (kind: string) => roleSpec(kind)

const query = ref('')
const hits = ref<SearchHit[]>([])
const active = ref(0)
const searching = ref(false)
const input = ref<HTMLInputElement | null>(null)

let seq = 0
let timer: ReturnType<typeof setTimeout> | null = null

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    query.value = ''
    hits.value = []
    active.value = 0
    await nextTick()
    input.value?.focus()
  },
)

watch(query, (q) => {
  if (timer) clearTimeout(timer)
  const trimmed = q.trim()
  if (!trimmed || !props.snapshotId) {
    hits.value = []
    searching.value = false
    return
  }
  searching.value = true
  // Debounced so a fast typist does not fire a request per keystroke.
  timer = setTimeout(() => void run(trimmed), 140)
})

async function run(q: string) {
  if (!props.snapshotId) return
  const mine = ++seq
  try {
    const res = await api.search(props.snapshotId, q, 40)
    // Ignore a response that a newer query has already superseded.
    if (mine !== seq) return
    hits.value = res.hits
    active.value = 0
  } catch {
    if (mine === seq) hits.value = []
  } finally {
    if (mine === seq) searching.value = false
  }
}

const empty = computed(() => !searching.value && query.value.trim() !== '' && hits.value.length === 0)

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    emit('close')
    return
  }
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    active.value = Math.min(active.value + 1, hits.value.length - 1)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    active.value = Math.max(active.value - 1, 0)
  } else if (e.key === 'Enter') {
    e.preventDefault()
    const hit = hits.value[active.value]
    if (hit) choose(hit.tableId)
  }
}

function choose(id: string) {
  emit('select', id)
  emit('close')
}
</script>

<template>
  <div v-if="open" class="backdrop" @click.self="emit('close')">
    <div class="palette" role="dialog" aria-modal="true" :aria-label="t('search.label')">
      <input
        ref="input"
        v-model="query"
        class="q"
        type="search"
        :placeholder="t('search.placeholder')"
        :aria-label="t('search.input.label')"
        @keydown="onKeydown"
      />

      <div class="results">
        <p v-if="!query.trim()" class="hint faint">{{ t('search.hint') }}</p>
        <p v-else-if="searching" class="hint faint">{{ t('search.searching') }}</p>
        <p v-else-if="empty" class="hint faint">{{ t('search.noMatches') }}</p>

        <ul v-else>
          <li v-for="(h, i) in hits" :key="h.tableId">
            <button
              class="hit"
              :class="{ 'hit--active': i === active }"
              @click="choose(h.tableId)"
              @mouseenter="active = i"
            >
              <i
                class="dot"
                :class="`dot--${roleOf(h.kind).swatch}`"
                :style="{ background: colorOf(h.kind) }"
              />
              <span class="mono hit-name">{{ h.name }}</span>
              <span class="tag tag--muted">{{ h.domainId }}</span>
              <span v-if="h.matchedOn?.length" class="faint tiny cols">
                {{ h.matchedOn.slice(0, 3).join(', ') }}
                <template v-if="h.matchedOn.length > 3">+{{ h.matchedOn.length - 3 }}</template>
              </span>
            </button>
          </li>
        </ul>
      </div>

      <footer class="foot faint tiny">
        <span><kbd>↑</kbd><kbd>↓</kbd> {{ t('search.key.navigate') }}</span>
        <span><kbd>↵</kbd> {{ t('search.key.open') }}</span>
        <span><kbd>esc</kbd> {{ t('search.key.close') }}</span>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.backdrop {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: flex;
  justify-content: center;
  padding-top: 12vh;
  background: var(--overlay);
  backdrop-filter: blur(2px);
}

.palette {
  width: min(620px, calc(100% - 32px));
  max-height: 64vh;
  display: flex;
  flex-direction: column;
  background: var(--panel-raised);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  overflow: hidden;
}

.q {
  padding: 14px 16px;
  border: none;
  border-bottom: 1px solid var(--border);
  background: transparent;
  color: var(--text);
  font: inherit;
  font-size: 15px;
}
.q:focus { outline: none; }
.q::placeholder { color: var(--text-faint); }

.results { flex: 1; overflow-y: auto; padding: 6px; }
.results ul { list-style: none; margin: 0; padding: 0; }
.hint { padding: 18px 12px; text-align: center; font-size: 12.5px; }

.hit {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 7px 10px;
  border: none;
  border-radius: var(--radius);
  background: none;
  color: var(--text);
  text-align: left;
  font-size: 13px;
}
.hit--active { background: var(--accent); color: var(--accent-contrast); }
/* On the highlighted row the tag's own background would read as a dark blob
   against the accent fill, so it becomes a plain outlined label. */
.hit--active .tag {
  background: transparent;
  border: 1px solid color-mix(in srgb, var(--accent-contrast) 45%, transparent);
  color: inherit;
}
.hit--active .faint { color: inherit; opacity: 0.8; }
.hit-name { flex: none; }
.cols { margin-left: auto; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.dot { width: 7px; height: 7px; flex: none; border-radius: 2px; }
.dot--round { border-radius: 50%; }
.dot--angular { border-radius: 0; clip-path: polygon(50% 0, 100% 50%, 50% 100%, 0 50%); }
/* The active hit is filled with the accent, and a role colour on top of it
   reads as neither. The dot goes flat for the one row that is active. */
.hit--active .dot { background: var(--accent-contrast) !important; }

.foot {
  display: flex;
  gap: 14px;
  padding: 7px 14px;
  border-top: 1px solid var(--border);
  background: var(--bg-sunken);
}
kbd {
  display: inline-block;
  margin-right: 3px;
  padding: 0 4px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  background: var(--panel);
  font-family: var(--font-mono);
  font-size: 10px;
}
.tiny { font-size: 11px; }
</style>
