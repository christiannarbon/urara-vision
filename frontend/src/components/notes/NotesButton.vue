<script setup lang="ts">
/** A note count for one anchor that opens its thread in a popover. */
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

import { Perm } from '../../auth/permissions'
import { useI18n } from '../../i18n'
import { useAuth } from '../../stores/auth'
import { useNotes } from '../../stores/notes'
import NotesThread from './NotesThread.vue'
import type { Anchor } from '../../notes/anchors'

const POPOVER_WIDTH = 320
const MARGIN = 8

const props = defineProps<{ anchor: Anchor; label?: string }>()
const emit = defineEmits<{ (e: 'open', anchor: Anchor): void }>()

const { t, tn } = useI18n()
const auth = useAuth()
const notes = useNotes()

const count = computed(() => notes.countFor(props.anchor))
const visible = computed(() => count.value.open + count.value.resolved > 0 || auth.can(Perm.NoteWrite))
const title = computed(() => {
  const parts = [tn('notes.button.open', count.value.open)]
  if (count.value.resolved) parts.push(tn('notes.button.resolved', count.value.resolved))
  return parts.join(' · ')
})

const open = ref(false)
const root = ref<HTMLElement | null>(null)
const trigger = ref<HTMLButtonElement | null>(null)
const pos = ref({ top: 0, left: 0, width: POPOVER_WIDTH, maxHeight: 400 })

// Fixed and clamped to the viewport, so a narrow pane cannot clip it.
function place() {
  const r = trigger.value?.getBoundingClientRect()
  if (!r) return
  const width = Math.min(POPOVER_WIDTH, window.innerWidth - 2 * MARGIN)
  const left = Math.min(Math.max(MARGIN, r.left), window.innerWidth - width - MARGIN)
  const top = r.bottom + 4
  pos.value = { top, left, width, maxHeight: Math.max(200, window.innerHeight - top - MARGIN) }
}

function onDocPointer(e: PointerEvent) {
  if (!root.value?.contains(e.target as Node)) close()
}

function toggle() {
  if (open.value) return close()
  place()
  open.value = true
  emit('open', props.anchor)
}

function close() {
  if (!open.value) return
  open.value = false
  void nextTick(() => trigger.value?.focus())
}

watch(open, (v) => {
  if (v) {
    document.addEventListener('pointerdown', onDocPointer)
    window.addEventListener('resize', place)
  } else {
    document.removeEventListener('pointerdown', onDocPointer)
    window.removeEventListener('resize', place)
  }
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onDocPointer)
  window.removeEventListener('resize', place)
})
</script>

<template>
  <span v-if="visible" ref="root" class="notes-button">
    <button
      ref="trigger"
      type="button"
      class="trigger"
      :class="{ quiet: count.open === 0 }"
      :title="title"
      :aria-label="`${label ?? t('notes.button')}: ${title}`"
      :aria-expanded="open"
      aria-haspopup="dialog"
      @click="toggle"
    >
      <svg viewBox="0 0 16 16" width="13" height="13" aria-hidden="true">
        <path d="M3 2.5h10a1 1 0 0 1 1 1v6.5a1 1 0 0 1-1 1H7l-3 2.5V11H3a1 1 0 0 1-1-1V3.5a1 1 0 0 1 1-1Z" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round" />
      </svg>
      <span v-if="count.open" class="count">{{ count.open }}</span>
    </button>

    <div
      v-if="open"
      class="popover"
      :style="{ top: `${pos.top}px`, left: `${pos.left}px`, width: `${pos.width}px`, maxHeight: `${pos.maxHeight}px` }"
    >
      <NotesThread :anchor="anchor" @close="close" />
    </div>
  </span>
</template>

<style scoped>
.notes-button { display: inline-flex; }

.trigger {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  padding: 1px var(--space-1);
  background: none;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  font-size: 11px;
  color: var(--accent);
  cursor: pointer;
}
.trigger:hover, .trigger:focus-visible, .trigger[aria-expanded='true'] { border-color: var(--border); background: var(--bg-sunken); }
.trigger.quiet { color: var(--text-faint); }
.trigger.quiet:hover { color: var(--text-muted); }
.count { font-weight: 600; font-variant-numeric: tabular-nums; }

.popover {
  position: fixed;
  z-index: 60;
  display: flex;
  flex-direction: column;
  padding: var(--space-3);
  background: var(--panel-raised);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  text-align: left;
}
</style>
