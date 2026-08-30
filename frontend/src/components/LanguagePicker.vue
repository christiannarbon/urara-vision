<script setup lang="ts">
/**
 * Language picker for the topbar.
 *
 * The same control as the theme picker beside it, deliberately: two adjacent
 * settings that behave differently for no reason is the kind of thing that
 * makes a toolbar feel arbitrary. A menu rather than a two-state toggle also
 * means a third language costs a catalogue file and nothing else.
 *
 * It carries no swatch, which is the one way it differs. The theme picker's
 * dot is the theme -- a palette is a colour. A language is not, and a glyph
 * standing in for one would be decoration.
 *
 * Every language is listed under its own name for itself. "Japanese" is no use
 * to somebody who cannot read the word "Japanese", and the reader most likely
 * to be reaching for this control is exactly the reader who cannot read the
 * language it is currently in.
 */
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

import { useI18n, type Locale, type MessageKey } from '../i18n'

const { locale, locales, t, setLocale } = useI18n()

const open = ref(false)
const root = ref<HTMLElement | null>(null)
const list = ref<HTMLElement | null>(null)

/** A language's endonym. The `satisfies` ties the key to the locale union, so
 *  a new language without a `locale.<id>` entry fails the typecheck. */
function nameOf(id: Locale): string {
  return t(`locale.${id}` satisfies MessageKey)
}

const current = computed(() => nameOf(locale.value))

function choose(id: Locale) {
  setLocale(id)
  open.value = false
}

function onDocPointer(e: PointerEvent) {
  if (!root.value?.contains(e.target as Node)) open.value = false
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && open.value) {
    // Stopped here rather than left to bubble: the shell's own Escape handler
    // would otherwise clear the selection behind the menu that was closing.
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
      :title="t('language.current', { name: current })"
      aria-haspopup="listbox"
      :aria-expanded="open"
      @click="open = !open"
    >
      <span class="name">{{ current }}</span>
      <span class="caret" aria-hidden="true">▾</span>
    </button>

    <div v-if="open" ref="list" class="menu" role="listbox" :aria-label="t('language.label')">
      <p class="menu-head">{{ t('language.label') }}</p>
      <button
        v-for="id in locales"
        :key="id"
        class="opt"
        role="option"
        :aria-selected="id === locale"
        :data-selected="id === locale"
        :lang="id"
        @click="choose(id)"
      >
        <span class="opt-name">{{ nameOf(id) }}</span>
        <span v-if="id === locale" class="tick" aria-hidden="true">✓</span>
      </button>
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

.menu {
  position: absolute;
  top: calc(100% + var(--space-2));
  right: 0;
  z-index: 40;
  width: 180px;
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

.opt-name {
  flex: 1;
  min-width: 0;
  font-family: var(--font-display);
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tick { color: var(--accent); font-size: 12px; }
</style>
