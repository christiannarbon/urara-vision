<script setup lang="ts">
/** Server-wide settings. Today, one switch: chat. */
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'

import { useI18n } from '../i18n'
import { useFeatures } from '../stores/features'

const { t } = useI18n()
const emit = defineEmits<{ (e: 'close'): void }>()

const features = useFeatures()
const { chatAvailable, chatEnabled } = storeToRefs(features)

const dialog = ref<HTMLElement | null>(null)
const saving = ref(false)
const failed = ref(false)

const switchDisabled = computed(() => !chatAvailable.value || saving.value)

onMounted(() => dialog.value?.focus())

async function toggleChat() {
  if (switchDisabled.value) return
  saving.value = true
  failed.value = false
  try {
    await features.setChatEnabled(!chatEnabled.value)
  } catch {
    failed.value = true
  } finally {
    saving.value = false
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    // Stopped here so App's Escape handling does not also close a panel.
    e.stopPropagation()
    emit('close')
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
  <div class="backdrop" @click.self="emit('close')">
    <div
      ref="dialog"
      class="card"
      role="dialog"
      aria-modal="true"
      aria-labelledby="settings-title"
      tabindex="-1"
      @keydown="onKeydown"
    >
      <header class="head">
        <h1 id="settings-title">{{ t('settings.title') }}</h1>
        <button type="button" class="btn btn--ghost btn--sm" @click="emit('close')">
          {{ t('settings.close') }}
        </button>
      </header>

      <div class="row">
        <div class="text">
          <span id="settings-chat" class="name">{{ t('settings.chat') }}</span>
          <span class="muted hint">{{ t('settings.chat.hint') }}</span>
          <span v-if="!chatAvailable" class="faint hint">{{ t('settings.chat.notDeployed') }}</span>
        </div>
        <button
          type="button"
          role="switch"
          class="switch"
          aria-labelledby="settings-chat"
          :aria-checked="chatEnabled"
          :disabled="switchDisabled"
          @click="toggleChat"
        >
          <span class="knob" aria-hidden="true" />
        </button>
      </div>

      <p v-if="failed" class="error" role="alert">{{ t('settings.chat.failed') }}</p>
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
  max-width: 440px;
  padding: 22px 24px 24px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-lg);
  background: var(--panel);
}
.card:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }

.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-4);
}
h1 { font-size: 18px; }

.row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-4);
}
.text { display: flex; flex-direction: column; gap: 4px; }
.name { font-size: 13px; font-weight: 600; }
.hint { font-size: 12.5px; line-height: 1.5; }

.switch {
  position: relative;
  flex: none;
  width: 36px;
  height: 20px;
  margin-top: 2px;
  padding: 0;
  border: 1px solid var(--border-strong);
  border-radius: 999px;
  background: var(--bg-sunken);
  cursor: pointer;
}
.switch[aria-checked='true'] { background: var(--accent); border-color: var(--accent); }
.switch:disabled { opacity: 0.5; cursor: not-allowed; }
.switch:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.knob {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: var(--panel-raised);
  transition: transform 0.15s ease;
}
.switch[aria-checked='true'] .knob { transform: translateX(16px); }

.error {
  margin-top: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--danger);
  border-radius: var(--radius);
  background: var(--danger-soft);
  color: var(--on-danger-soft);
  font-size: 12.5px;
}
</style>
