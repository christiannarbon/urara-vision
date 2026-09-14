<script setup lang="ts">
/** A modal yes/no question. */
import { nextTick, ref, watch } from 'vue'

import { useI18n } from '../i18n'

const { t } = useI18n()

const props = defineProps<{
  open: boolean
  title: string
  message: string
  confirmLabel: string
  danger?: boolean
  busy?: boolean
}>()

const emit = defineEmits<{ (e: 'confirm'): void; (e: 'cancel'): void }>()

const dialog = ref<HTMLElement | null>(null)
const cancelButton = ref<HTMLButtonElement | null>(null)

// Cancel takes focus so Enter never confirms by accident.
watch(
  () => props.open,
  (open) => {
    if (open) void nextTick(() => cancelButton.value?.focus())
  },
  { immediate: true },
)

function cancel() {
  if (!props.busy) emit('cancel')
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    // Stopped so App's Escape handling does not also close a panel.
    e.stopPropagation()
    cancel()
  } else if (e.key === 'Tab') {
    trapTab(e)
  }
}

function trapTab(e: KeyboardEvent) {
  const buttons = dialog.value?.querySelectorAll<HTMLElement>('button:not([disabled])')
  if (!buttons?.length) return
  const first = buttons[0]
  const last = buttons[buttons.length - 1]
  const active = document.activeElement
  if (e.shiftKey && (active === first || active === dialog.value)) {
    e.preventDefault()
    last.focus()
  } else if (!e.shiftKey && active === last) {
    e.preventDefault()
    first.focus()
  }
}
</script>

<template>
  <div v-if="open" class="backdrop" @click.self="cancel">
    <div
      ref="dialog"
      class="card"
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="confirm-title"
      aria-describedby="confirm-message"
      tabindex="-1"
      @keydown="onKeydown"
    >
      <h1 id="confirm-title">{{ title }}</h1>
      <p id="confirm-message" class="muted message">{{ message }}</p>
      <div class="actions">
        <button ref="cancelButton" type="button" class="btn" :disabled="busy" @click="cancel">
          {{ t('confirm.cancel') }}
        </button>
        <button
          type="button"
          class="btn"
          :class="danger ? 'btn--danger' : 'btn--primary'"
          :disabled="busy"
          @click="emit('confirm')"
        >
          {{ confirmLabel }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.backdrop {
  position: fixed;
  inset: 0;
  z-index: 100;
  display: grid;
  place-items: center;
  padding: 20px;
  background: var(--overlay);
}

.card {
  width: 100%;
  max-width: 420px;
  padding: 22px 24px 20px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-lg);
  background: var(--panel);
}
.card:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }

h1 { font-size: 16px; margin-bottom: var(--space-2); }
.message { font-size: 13px; line-height: 1.55; margin: 0 0 var(--space-4); }

.actions { display: flex; justify-content: flex-end; gap: 8px; }

.btn--danger {
  background: var(--danger);
  border-color: var(--danger);
  color: var(--on-danger);
}
.btn--danger:hover:not(:disabled) { filter: brightness(0.92); }
</style>
