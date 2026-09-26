<script setup lang="ts">
/** One note, with its actions and (for a top-level note) its replies. */
import { computed, nextTick, ref } from 'vue'

import { ApiError } from '../../api/client'
import { Perm } from '../../auth/permissions'
import { useI18n } from '../../i18n'
import { useAuth } from '../../stores/auth'
import { useNotes } from '../../stores/notes'
import ConfirmDialog from '../ConfirmDialog.vue'
import type { Note } from '../../api/types'

const props = defineProps<{ note: Note; isReply?: boolean }>()

const { t, locale } = useI18n()
const auth = useAuth()
const notes = useNotes()

const canWrite = computed(() => auth.can(Perm.NoteWrite))
const canChange = computed(
  () => (!!props.note.authorId && props.note.authorId === auth.user?.id) || auth.can(Perm.NoteModerate),
)
const edited = computed(() => Date.parse(props.note.updatedAt) - Date.parse(props.note.createdAt) > 1000)

const expanded = ref(false)
const collapsed = computed(() => !!props.note.resolvedAt && !expanded.value)

const busy = ref(false)
const error = ref('')

const mode = ref<'view' | 'edit' | 'reply'>('view')
const draft = ref('')
const editor = ref<HTMLTextAreaElement | null>(null)

const units: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 31536000],
  ['month', 2592000],
  ['day', 86400],
  ['hour', 3600],
  ['minute', 60],
]

function relative(iso: string): string {
  const secs = (Date.parse(iso) - Date.now()) / 1000
  if (Number.isNaN(secs)) return iso
  const fmt = new Intl.RelativeTimeFormat(locale.value, { numeric: 'auto' })
  for (const [unit, size] of units) {
    if (Math.abs(secs) >= size) return fmt.format(Math.round(secs / size), unit)
  }
  return fmt.format(0, 'second')
}

async function run(fn: () => Promise<unknown>): Promise<boolean> {
  busy.value = true
  error.value = ''
  try {
    await fn()
    return true
  } catch (e) {
    error.value = e instanceof ApiError && !e.key ? e.message : t('notes.error')
    return false
  } finally {
    busy.value = false
  }
}

function start(next: 'edit' | 'reply') {
  mode.value = next
  draft.value = next === 'edit' ? props.note.body : ''
  error.value = ''
  void nextTick(() => editor.value?.focus())
}

function cancel() {
  mode.value = 'view'
  draft.value = ''
}

async function submit() {
  const body = draft.value.trim()
  if (!body || busy.value) return
  const ok = await run(() =>
    mode.value === 'edit'
      ? notes.editBody(props.note.id, body)
      : notes.add({ kind: props.note.anchorKind, id: props.note.anchorId }, body, props.note.id),
  )
  if (ok) cancel()
}

function onEditorKey(e: KeyboardEvent) {
  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
    e.preventDefault()
    void submit()
  } else if (e.key === 'Escape') {
    // Closes the editor, not the whole thread.
    e.stopPropagation()
    cancel()
  }
}

const confirming = ref(false)

async function confirmDelete() {
  if (await run(() => notes.remove(props.note.id))) confirming.value = false
}
</script>

<template>
  <article class="note" :class="{ 'note--reply': isReply, 'note--resolved': note.resolvedAt }">
    <button v-if="collapsed" type="button" class="collapsed" @click="expanded = true">
      <span aria-hidden="true">✓</span> {{ t('notes.item.resolvedBy', { name: note.resolvedByName || '—' }) }}
    </button>

    <template v-else>
      <header class="head">
        <span class="author">{{ note.authorName }}</span>
        <time class="faint" :datetime="note.createdAt" :title="new Date(note.createdAt).toLocaleString(locale)">
          {{ relative(note.createdAt) }}
        </time>
        <span v-if="edited" class="faint">· {{ t('notes.item.edited') }}</span>
        <button v-if="note.resolvedAt" type="button" class="linkish faint" @click="expanded = false">
          {{ t('notes.item.resolvedBy', { name: note.resolvedByName || '—' }) }}
        </button>
      </header>

      <div v-if="mode === 'edit'" class="editor">
        <textarea ref="editor" v-model="draft" class="input" rows="3" :disabled="busy" @keydown="onEditorKey" />
        <div class="row">
          <button type="button" class="btn btn--ghost btn--sm" :disabled="busy" @click="cancel">{{ t('notes.item.cancel') }}</button>
          <button type="button" class="btn btn--primary btn--sm" :disabled="busy || !draft.trim()" @click="submit">
            {{ t('notes.item.save') }}
          </button>
        </div>
      </div>
      <p v-else class="body">{{ note.body }}</p>

      <div v-if="mode !== 'edit'" class="actions">
        <template v-if="!isReply && canWrite">
          <button type="button" class="linkish" :disabled="busy" @click="start('reply')">{{ t('notes.item.reply') }}</button>
          <button type="button" class="linkish" :disabled="busy" @click="run(() => notes.setResolved(note.id, !note.resolvedAt))">
            {{ note.resolvedAt ? t('notes.item.reopen') : t('notes.item.resolve') }}
          </button>
        </template>
        <template v-if="canChange">
          <button type="button" class="linkish" :disabled="busy" @click="start('edit')">{{ t('notes.item.edit') }}</button>
          <button type="button" class="linkish danger" :disabled="busy" @click="confirming = true">{{ t('notes.item.delete') }}</button>
        </template>
      </div>

      <p v-if="error" class="error" role="alert">{{ error }}</p>

      <div v-if="note.replies?.length" class="replies">
        <NoteItem v-for="r in note.replies" :key="r.id" :note="r" is-reply />
      </div>

      <div v-if="mode === 'reply'" class="editor">
        <textarea
          ref="editor"
          v-model="draft"
          class="input"
          rows="2"
          :placeholder="t('notes.composer.replyPlaceholder')"
          :disabled="busy"
          @keydown="onEditorKey"
        />
        <div class="row">
          <button type="button" class="btn btn--ghost btn--sm" :disabled="busy" @click="cancel">{{ t('notes.item.cancel') }}</button>
          <button type="button" class="btn btn--primary btn--sm" :disabled="busy || !draft.trim()" @click="submit">
            {{ t('notes.item.reply') }}
          </button>
        </div>
      </div>
    </template>

    <ConfirmDialog
      :open="confirming"
      :title="t('notes.delete.title')"
      :message="isReply ? t('notes.delete.message') : t('notes.delete.withReplies')"
      :confirm-label="t('notes.delete.confirm')"
      :busy="busy"
      danger
      @confirm="confirmDelete"
      @cancel="confirming = false"
    />
  </article>
</template>

<style scoped>
.note { padding: var(--space-2) 0; }
.note + .note { border-top: 1px solid var(--border); }
.note--reply { margin-left: var(--space-4); padding-left: var(--space-3); border-left: 2px solid var(--border); }
.note--reply + .note--reply { border-top: 0; }

.head { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--space-1) var(--space-2); font-size: 12px; }
.author { font-weight: 600; color: var(--text); }

.body {
  margin: var(--space-1) 0 0;
  font-size: 13px;
  line-height: 1.5;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.actions { display: flex; flex-wrap: wrap; gap: var(--space-3); margin-top: var(--space-1); }
.linkish {
  padding: 0;
  background: none;
  border: 0;
  font-size: 12px;
  color: var(--text-muted);
  cursor: pointer;
}
.linkish:hover:not(:disabled), .linkish:focus-visible { color: var(--accent); text-decoration: underline; }
.linkish.danger:hover:not(:disabled) { color: var(--danger); }

.collapsed {
  width: 100%;
  padding: var(--space-1) 0;
  background: none;
  border: 0;
  text-align: left;
  font-size: 12px;
  color: var(--text-faint);
  cursor: pointer;
}
.collapsed:hover { color: var(--text-muted); }

.replies { margin-top: var(--space-2); }

.editor { margin-top: var(--space-2); }
.editor textarea { width: 100%; resize: vertical; font: inherit; font-size: 13px; }
.row { display: flex; justify-content: flex-end; gap: var(--space-2); margin-top: var(--space-1); }

.error { margin: var(--space-1) 0 0; font-size: 12px; color: var(--danger); }
</style>
