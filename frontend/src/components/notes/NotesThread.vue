<script setup lang="ts">
/** The notes on one anchor, and a composer for a new one. */
import { computed, nextTick, onMounted, ref } from 'vue'

import { ApiError } from '../../api/client'
import { Perm } from '../../auth/permissions'
import { useI18n } from '../../i18n'
import { anchorKey } from '../../notes/anchors'
import { useAuth } from '../../stores/auth'
import { useNotes } from '../../stores/notes'
import NoteItem from './NoteItem.vue'
import type { Anchor } from '../../notes/anchors'

const MAX_RUNES = 4000
const COUNTER_FROM = 3500

const props = defineProps<{ anchor: Anchor }>()
const emit = defineEmits<{ (e: 'close'): void }>()

const { t } = useI18n()
const auth = useAuth()
const notes = useNotes()

const canWrite = computed(() => auth.can(Perm.NoteWrite))
const thread = computed(() => notes.threads.get(anchorKey(props.anchor)) ?? [])

const loading = ref(false)
const loadFailed = ref(false)

async function load() {
  loading.value = true
  loadFailed.value = false
  try {
    await notes.loadThread(props.anchor)
  } catch {
    loadFailed.value = true
  } finally {
    loading.value = false
  }
}

const draft = ref('')
const pending = ref(false)
const error = ref('')
const composer = ref<HTMLTextAreaElement | null>(null)

// Trimmed, as the server counts it.
const runes = computed(() => [...draft.value.trim()].length)
const canSubmit = computed(() => !pending.value && draft.value.trim() !== '' && runes.value <= MAX_RUNES)

async function submit() {
  if (!canSubmit.value) return
  pending.value = true
  error.value = ''
  try {
    await notes.add(props.anchor, draft.value.trim())
    draft.value = ''
  } catch (e) {
    error.value = e instanceof ApiError && !e.key ? e.message : t('notes.error')
  } finally {
    pending.value = false
    void nextTick(() => composer.value?.focus())
  }
}

function onComposerKey(e: KeyboardEvent) {
  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
    e.preventDefault()
    void submit()
  }
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.stopPropagation()
    emit('close')
  }
}

onMounted(() => {
  composer.value?.focus()
  void load()
})
</script>

<template>
  <section class="thread" role="dialog" :aria-label="t('notes.thread.title')" @keydown="onKey">
    <header class="top">
      <h2>{{ t('notes.thread.title') }}</h2>
      <button type="button" class="close" :aria-label="t('notes.thread.close')" @click="emit('close')">×</button>
    </header>

    <div class="list">
      <p v-if="loading && !thread.length" class="faint state">{{ t('notes.thread.loading') }}</p>
      <p v-else-if="loadFailed" class="state error" role="alert">
        {{ t('notes.thread.failed') }}
        <button type="button" class="btn btn--ghost btn--sm" @click="load">{{ t('notes.thread.retry') }}</button>
      </p>
      <p v-else-if="!thread.length" class="faint state">{{ t('notes.thread.empty') }}</p>
      <NoteItem v-for="n in thread" :key="n.id" :note="n" />
    </div>

    <form v-if="canWrite" class="composer" @submit.prevent="submit">
      <textarea
        ref="composer"
        v-model="draft"
        class="input"
        rows="3"
        :placeholder="t('notes.composer.placeholder')"
        :disabled="pending"
        @keydown="onComposerKey"
      />
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <div class="row">
        <span class="faint hint">{{ t('notes.composer.hint') }}</span>
        <span v-if="runes > COUNTER_FROM" class="counter" :class="{ over: runes > MAX_RUNES }" aria-live="polite">
          {{ t('notes.composer.counter', { n: runes, max: MAX_RUNES }) }}
        </span>
        <button type="submit" class="btn btn--primary btn--sm" :disabled="!canSubmit">{{ t('notes.composer.add') }}</button>
      </div>
    </form>
  </section>
</template>

<style scoped>
.thread { display: flex; flex-direction: column; flex: 1 1 auto; min-height: 0; font-size: 13px; }

.top { display: flex; align-items: center; justify-content: space-between; margin-bottom: var(--space-2); }
h2 { margin: 0; font-size: 13px; font-weight: 600; }
.close {
  padding: 0 var(--space-1);
  background: none;
  border: 0;
  font-size: 18px;
  line-height: 1;
  color: var(--text-faint);
  cursor: pointer;
}
.close:hover { color: var(--text); }

.list { flex: 1 1 auto; min-height: 0; overflow-y: auto; }
.state { margin: var(--space-2) 0; font-size: 12px; }

.composer { margin-top: var(--space-2); padding-top: var(--space-2); border-top: 1px solid var(--border); }
.composer textarea { width: 100%; resize: vertical; font: inherit; font-size: 13px; }
.row { display: flex; align-items: center; gap: var(--space-2); margin-top: var(--space-1); }
.hint { font-size: 11px; margin-right: auto; }
.counter { font-size: 11px; color: var(--text-muted); }
.counter.over { color: var(--danger); }

.error { margin: var(--space-1) 0 0; font-size: 12px; color: var(--danger); }
</style>
