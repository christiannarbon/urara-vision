<script setup lang="ts">
/**
 * Theme picker for the topbar.
 *
 * A plain button that opens the list of available palettes: the app's own two
 * first, then the painting-derived ones. There is no light/dark control beside
 * it -- the app is light-mode only, so a theme is one palette.
 */
import { computed, onBeforeUnmount, ref, watch, nextTick } from 'vue'

import { useTheme } from '../composables/useTheme'

const { art, themes, current, setArt } = useTheme()

const open = ref(false)
const root = ref<HTMLElement | null>(null)
const list = ref<HTMLElement | null>(null)

const label = computed(() =>
  current.value.subtitle ? `${current.value.name} — ${current.value.subtitle}` : current.value.name,
)

/** Index of the first painting, so a rule can mark where the paintings start. */
const firstPainting = computed(() => themes.findIndex((t) => t.kind === 'painting'))

function choose(id: string) {
  setArt(id)
  open.value = false
}

function onDocPointer(e: PointerEvent) {
  if (!root.value?.contains(e.target as Node)) open.value = false
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && open.value) {
    e.stopPropagation()
    open.value = false
  }
}

watch(open, async (v) => {
  if (v) {
    document.addEventListener('pointerdown', onDocPointer)
    await nextTick()
    list.value?.querySelector<HTMLElement>('[data-selected="true"]')?.focus()
  } else {
    document.removeEventListener('pointerdown', onDocPointer)
  }
})

onBeforeUnmount(() => document.removeEventListener('pointerdown', onDocPointer))
</script>

<template>
  <div ref="root" class="picker" @keydown="onKey">
    <button
      class="btn btn--ghost btn--sm trigger"
      :title="`Theme: ${label}`"
      aria-haspopup="listbox"
      :aria-expanded="open"
      @click="open = !open"
    >
      <span class="dot" :style="{ background: current.swatch }" aria-hidden="true" />
      <span class="name">{{ current.name }}</span>
      <span class="caret" aria-hidden="true">▾</span>
    </button>

    <div v-if="open" ref="list" class="menu" role="listbox" aria-label="Theme">
      <p class="menu-head">Theme</p>
      <template v-for="(t, i) in themes" :key="t.id">
        <p v-if="i === firstPainting" class="menu-head menu-head--mid">Paintings</p>
        <button
          class="opt"
          role="option"
          :aria-selected="t.id === art"
          :data-selected="t.id === art"
          @click="choose(t.id)"
        >
          <span class="dot" :style="{ background: t.swatch }" aria-hidden="true" />
          <span class="opt-text">
            <span class="opt-name">{{ t.name }}</span>
            <span class="opt-sub">{{ t.subtitle }}</span>
          </span>
          <span v-if="t.id === art" class="tick" aria-hidden="true">✓</span>
        </button>
      </template>
    </div>
  </div>
</template>

<style scoped>
.picker { position: relative; }

.trigger { gap: var(--space-2); }
.name {
  max-width: 12ch;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.caret { font-size: 9px; color: var(--text-faint); }

.dot {
  width: 10px;
  height: 10px;
  border-radius: var(--radius-full);
  border: 1px solid var(--border-strong);
  flex: none;
}

.menu {
  position: absolute;
  top: calc(100% + var(--space-2));
  right: 0;
  z-index: 40;
  width: 264px;
  padding: var(--space-1);
  background: var(--panel-raised);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
}

.menu-head {
  margin: 0;
  padding: var(--space-2) var(--space-3) var(--space-1);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.07em;
  text-transform: uppercase;
  color: var(--text-faint);
}
.menu-head--mid {
  margin-top: var(--space-1);
  padding-top: var(--space-3);
  border-top: 1px solid var(--border);
}

.opt {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: none;
  border: 0;
  border-radius: var(--radius);
  text-align: left;
  transition: background var(--dur) var(--ease);
}
.opt:hover { background: var(--bg-sunken); }
.opt[data-selected="true"] { background: var(--fact-soft); }

.opt-text { display: flex; flex-direction: column; min-width: 0; flex: 1; }
.opt-name {
  font-family: var(--font-display);
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
}
.opt-sub {
  font-size: 11px;
  color: var(--text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tick { color: var(--accent); font-size: 12px; }
</style>
