<script setup lang="ts">
/** A note count for one anchor that opens its thread in a popover. */
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

import { Perm } from '../../auth/permissions'
import { useI18n } from '../../i18n'
import { useAuth } from '../../stores/auth'
import { useNotes } from '../../stores/notes'
import { useWorkspace } from '../../stores/workspace'
import NotesThread from './NotesThread.vue'
import type { Anchor } from '../../notes/anchors'

const POPOVER_WIDTH = 320
const MARGIN = 8
const GAP = 4
const MIN_HEIGHT = 200

const props = defineProps<{ anchor: Anchor; label?: string }>()
const emit = defineEmits<{ (e: 'open', anchor: Anchor): void }>()

const { t, tn } = useI18n()
const auth = useAuth()
const notes = useNotes()
const workspace = useWorkspace()

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
const style = ref<Record<string, string>>({})

const px = (n: number) => `${n}px`

// Fixed and kept inside the window, so a narrow or scrolled pane cannot clip it.
// Opens upward when there is too little room below.
function place() {
  const r = trigger.value?.getBoundingClientRect()
  if (!r) return
  const width = Math.min(POPOVER_WIDTH, window.innerWidth - 2 * MARGIN)
  const left = Math.min(Math.max(MARGIN, r.left), window.innerWidth - width - MARGIN)
  const below = window.innerHeight - r.bottom - GAP - MARGIN
  const above = r.top - GAP - MARGIN
  const base = { left: px(left), width: px(width) }
  style.value =
    below < MIN_HEIGHT && above > below
      ? { ...base, bottom: px(window.innerHeight - r.top + GAP), maxHeight: px(above) }
      : { ...base, top: px(r.bottom + GAP), maxHeight: px(below) }
}

// Capture, so scrolling any pane (not only the window) re-places the popover.
const SCROLL = { capture: true, passive: true }

function removeListeners() {
  document.removeEventListener('pointerdown', onDocPointer)
  window.removeEventListener('resize', place)
  window.removeEventListener('scroll', place, SCROLL)
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
    window.addEventListener('scroll', place, SCROLL)
  } else {
    removeListeners()
  }
})

// A thread belongs to one version.
watch(() => workspace.snapshot?.id, close)

onBeforeUnmount(removeListeners)
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
      :style="style"
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
