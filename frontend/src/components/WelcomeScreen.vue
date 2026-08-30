<script setup lang="ts">
/** First-run screen: pick a documentation directory, or reopen a past ingest. */
import { computed, ref } from 'vue'
import type { Snapshot } from '../api/types'
import { supportsNativePicker, useDirectoryPicker } from '../composables/useDirectoryPicker'
import { useI18n } from '../i18n'

const { t, tn, locale } = useI18n()

const props = defineProps<{
  snapshots: Snapshot[]
  busy: boolean
  statusMessage: string
}>()

const emit = defineEmits<{
  (e: 'ingest', payload: { name: string; sourceLabel: string; files: { path: string; content: string }[] }): void
  (e: 'open', sid: string): void
  (e: 'delete', sid: string): void
}>()

const picker = useDirectoryPicker()
const fileInput = ref<HTMLInputElement | null>(null)
const native = supportsNativePicker()
const dragging = ref(false)

const disabled = computed(() => props.busy || picker.reading.value)

async function chooseNative() {
  const picked = await picker.pickNative()
  if (picked) submit(picked.name, picked.files)
}

function chooseFallback() {
  fileInput.value?.click()
}

async function onInput(e: Event) {
  const input = e.target as HTMLInputElement
  const picked = await picker.readFileList(input.files)
  // Reset so choosing the same directory twice still fires a change event.
  input.value = ''
  if (picked) submit(picked.name, picked.files)
}

function submit(name: string, files: { path: string; content: string }[]) {
  if (!files.length) {
    picker.error.value = t('picker.error.noMarkdown')
    return
  }
  emit('ingest', { name, sourceLabel: name, files })
}

/** An ingest's timestamp, in the conventions of the language on screen. */
function formatDate(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString(locale.value)
}
</script>

<template>
  <div class="welcome">
    <div class="card">
      <header class="intro">
        <h1>{{ t('welcome.title') }}</h1>
        <p class="muted">{{ t('welcome.intro') }}</p>
      </header>

      <div
        class="dropzone"
        :class="{ 'dropzone--busy': disabled, 'dropzone--drag': dragging }"
        @dragover.prevent="dragging = true"
        @dragleave="dragging = false"
        @drop.prevent="dragging = false"
      >
        <div v-if="disabled" class="picking">
          <div class="spinner" aria-hidden="true" />
          <p>{{ picker.progressLabel.value || statusMessage || t('welcome.working') }}</p>
        </div>

        <template v-else>
          <p class="dz-title">{{ t('welcome.dropzone.title') }}</p>
          <p class="muted dz-sub">
            {{ t('welcome.dropzone.hint.before') }} <code>.md</code>
            {{ t('welcome.dropzone.hint.after') }}
          </p>

          <div class="actions">
            <button v-if="native" class="btn btn--primary" @click="chooseNative">
              {{ t('welcome.choose') }}
            </button>
            <button class="btn" :class="{ 'btn--primary': !native }" @click="chooseFallback">
              {{ native ? t('welcome.useFileInput') : t('welcome.choose') }}
            </button>
          </div>

          <p v-if="!native" class="faint tiny note">{{ t('welcome.noPicker') }}</p>
        </template>

        <input
          ref="fileInput"
          type="file"
          class="hidden-input"
          webkitdirectory
          directory
          multiple
          @change="onInput"
        />
      </div>

      <p v-if="picker.error.value" class="error" role="alert">{{ picker.error.value }}</p>

      <section v-if="snapshots.length" class="recent">
        <h2 class="section-label">{{ t('welcome.recent') }}</h2>
        <ul>
          <li v-for="s in snapshots" :key="s.id">
            <button class="snap" :disabled="disabled" @click="emit('open', s.id)">
              <span class="snap-name">{{ s.name }}</span>
              <span class="faint tiny">
                {{ tn('welcome.stats.tables', s.stats.tables) }} ·
                {{ tn('welcome.stats.domains', s.stats.domains) }} ·
                {{ formatDate(s.createdAt) }}
              </span>
            </button>
            <button
              class="btn btn--ghost btn--sm"
              :disabled="disabled"
              :title="t('welcome.delete.title')"
              :aria-label="t('welcome.delete.label')"
              @click="emit('delete', s.id)"
            >
              ✕
            </button>
          </li>
        </ul>
      </section>
    </div>
  </div>
</template>

<style scoped>
.welcome {
  display: grid;
  place-items: center;
  min-height: 100%;
  padding: 40px 20px;
  background: var(--bg);
  overflow-y: auto;
}

.card {
  width: 100%;
  max-width: 620px;
}

.intro { margin-bottom: 22px; }
.intro h1 { font-size: 22px; margin-bottom: 8px; }
.intro p { font-size: 13.5px; line-height: 1.6; }

.dropzone {
  padding: 34px 26px;
  border: 1.5px dashed var(--border-strong);
  border-radius: var(--radius-lg);
  background: var(--panel);
  text-align: center;
  transition: border-color var(--dur) var(--ease), background var(--dur) var(--ease);
}
.dropzone--drag { border-color: var(--accent); background: var(--bg-sunken); }
.dropzone--busy { border-style: solid; }

.dz-title { font-size: 15px; font-weight: 600; margin-bottom: 6px; }
.dz-sub { font-size: 12.5px; max-width: 42ch; margin: 0 auto 18px; line-height: 1.55; }

.actions { display: flex; gap: 8px; justify-content: center; flex-wrap: wrap; }
.note { margin-top: 12px; }
.tiny { font-size: 11px; }

.hidden-input { display: none; }

.picking { display: flex; flex-direction: column; align-items: center; gap: 12px; }
.picking p { font-size: 13px; color: var(--text-muted); margin: 0; }
.spinner {
  width: 24px;
  height: 24px;
  border: 2px solid var(--border-strong);
  border-top-color: var(--accent);
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }

.error {
  margin-top: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--danger);
  border-radius: var(--radius);
  background: var(--danger-soft);
  color: var(--on-danger-soft);
  font-size: 12.5px;
}

.recent { margin-top: 28px; }
.recent ul { list-style: none; margin: 8px 0 0; padding: 0; }
.recent li {
  display: flex;
  align-items: center;
  gap: 6px;
  border-bottom: 1px solid var(--border);
}
.recent li:last-child { border-bottom: none; }

.snap {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 9px 8px;
  border: none;
  border-radius: var(--radius-sm);
  background: none;
  text-align: left;
  transition: background var(--dur) var(--ease);
  min-width: 0;
}
.snap:hover:not(:disabled) { background: var(--bg-sunken); }
.snap:disabled { opacity: 0.5; cursor: not-allowed; }
.snap-name {
  font-size: 13px;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
