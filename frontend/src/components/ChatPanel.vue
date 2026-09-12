<script setup lang="ts">
/** Ask questions about the loaded model, and read the answers. */
import { computed, nextTick, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'

import { renderAnswer } from '../chat/markdown'
import { useI18n } from '../i18n'
import { useChat } from '../stores/chat'
import { useWorkspace } from '../stores/workspace'

const { t } = useI18n()
const chat = useChat()
const workspace = useWorkspace()

const { messages, pending, open, lastToolCalls, truncated, errorKey, errorDetail, failedQuestion } =
  storeToRefs(chat)

const draft = ref('')
const list = ref<HTMLElement | null>(null)
const composer = ref<HTMLTextAreaElement | null>(null)

const error = computed(() => (errorKey.value ? t(errorKey.value) : errorDetail.value))

const pendingLabel = computed(() => {
  const tool = lastToolCalls.value.at(-1)?.name
  return tool ? t('chat.thinkingTool', { tool }) : t('chat.thinking')
})

/** A real table from the snapshot, so the suggestion is worth clicking. A fact
 *  table if there is one; no suggestion at all when nothing is loaded. */
const sampleTable = computed(() => {
  const tables = workspace.tables
  if (!tables.length) return null
  return (tables.find((tb) => tb.kind === 'fact') ?? tables[0]).name
})

const suggestions = computed(() => {
  const out = [t('chat.suggestion.diagnostics'), t('chat.suggestion.conformed')]
  if (sampleTable.value) out.push(t('chat.suggestion.table', { table: sampleTable.value }))
  return out
})

// Rendered once per message rather than in the template, which would re-parse
// every answer on each keystroke in the composer.
const rendered = computed(() =>
  messages.value.map((m) => ({
    ...m,
    html: m.role === 'assistant' ? renderAnswer(m.content) : '',
  })),
)

function bareName(id: string): string {
  return id.slice(id.lastIndexOf('/') + 1)
}

/** True for the trailing user message whose turn failed. */
function isFailed(index: number): boolean {
  return failedQuestion.value !== null && index === messages.value.length - 1
}

function send() {
  const question = draft.value.trim()
  if (!question || pending.value) return
  draft.value = ''
  void chat.ask(question)
}

function onKeydown(e: KeyboardEvent) {
  // Enter sends; Shift+Enter falls through to the textarea's own newline.
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    send()
  }
}

function ask(question: string) {
  if (pending.value) return
  void chat.ask(question)
}

function focusTable(id: string) {
  void workspace.focusOn(id)
}

/** Grows the composer to about five rows, then lets it scroll. */
const MAX_COMPOSER_PX = 108
function autosize() {
  const el = composer.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${Math.min(el.scrollHeight, MAX_COMPOSER_PX)}px`
}

watch(draft, () => void nextTick(autosize))

function scrollToBottom() {
  void nextTick(() => {
    const el = list.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

watch([() => messages.value.length, pending], scrollToBottom)

watch(open, (isOpen) => {
  if (!isOpen) return
  void nextTick(() => composer.value?.focus())
})
</script>

<template>
  <div
    v-if="open"
    class="drawer"
    role="region"
    :aria-label="t('chat.title')"
    @keydown.esc="chat.closePanel()"
  >
    <header class="head">
      <h2>{{ t('chat.title') }}</h2>
      <div class="head-actions">
        <button
          class="btn btn--ghost btn--sm"
          :disabled="pending || !messages.length"
          @click="chat.reset()"
        >
          {{ t('chat.newConversation') }}
        </button>
        <button
          class="btn btn--ghost btn--sm"
          :aria-label="t('chat.close')"
          @click="chat.closePanel()"
        >
          ✕
        </button>
      </div>
    </header>

    <p v-if="chat.notice" class="banner" role="status">
      {{ t(chat.notice) }}
      <button class="btn btn--ghost btn--sm" @click="chat.dismissNotice()">
        {{ t('banner.dismiss') }}
      </button>
    </p>

    <div ref="list" class="body" role="log" aria-live="polite">
      <div v-if="!messages.length && !pending" class="empty">
        <h3>{{ t('chat.emptyTitle') }}</h3>
        <p class="faint tiny">{{ t('chat.emptyHint') }}</p>
        <ul class="suggestions">
          <li v-for="s in suggestions" :key="s">
            <button class="suggestion" @click="ask(s)">{{ s }}</button>
          </li>
        </ul>
      </div>

      <div
        v-for="(m, i) in rendered"
        :key="`${m.role}-${m.ordinal}-${i}`"
        class="turn"
        :class="`turn--${m.role}`"
      >
        <p v-if="m.role === 'user'" class="bubble" :class="{ 'bubble--failed': isFailed(i) }">
          {{ m.content }}
        </p>

        <template v-else>
          <!-- The only v-html here, and safe only because renderAnswer escapes first. -->
          <div class="answer" v-html="m.html" />
          <template v-if="m.citations.length">
            <h4 class="cite-head">{{ t('chat.citations') }}</h4>
            <ul class="cites">
              <li v-for="id in m.citations" :key="id">
                <button
                  class="cite"
                  :title="t('chat.citationTitle', { id })"
                  :aria-label="t('chat.citationTitle', { id })"
                  @click="focusTable(id)"
                >
                  {{ bareName(id) }}
                </button>
              </li>
            </ul>
          </template>
        </template>
      </div>

      <p v-if="truncated && !pending" class="note faint tiny">{{ t('chat.truncated') }}</p>

      <p v-if="pending" class="thinking" :aria-label="pendingLabel">
        <span class="dots" aria-hidden="true"><i /><i /><i /></span>
        {{ pendingLabel }}
      </p>

      <p v-if="error" class="banner banner--error" role="alert">
        {{ error }}
        <button v-if="failedQuestion" class="btn btn--ghost btn--sm" @click="chat.retry()">
          {{ t('chat.retry') }}
        </button>
      </p>
    </div>

    <form class="composer" @submit.prevent="send">
      <textarea
        ref="composer"
        v-model="draft"
        class="input"
        rows="1"
        :placeholder="t('chat.placeholder')"
        :disabled="pending"
        :aria-label="t('chat.placeholder')"
        @keydown="onKeydown"
        @input="autosize"
      />
      <button class="btn btn--primary btn--sm" type="submit" :disabled="pending || !draft.trim()">
        {{ t('chat.send') }}
      </button>
    </form>
  </div>
</template>

<style scoped>
.drawer {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--panel);
  border-left: 1px solid var(--border);
  overflow: hidden;
}

.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  padding: 13px 15px 11px;
  border-bottom: 1px solid var(--border);
}
.head h2 { font-size: 14px; }
.head-actions { display: flex; gap: 4px; flex: none; }
.tiny { font-size: 11px; }

.banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  margin: 0;
  padding: 8px 15px;
  border-bottom: 1px solid var(--border);
  background: var(--info-soft);
  color: var(--on-info-soft);
  font-size: 12px;
}
.banner--error { background: var(--danger-soft); color: var(--on-danger-soft); }

.body {
  flex: 1;
  overflow-y: auto;
  padding: 12px 15px 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.empty { padding: 24px 0; text-align: center; }
.empty h3 { font-size: 13px; }
.empty p { margin: 4px 0 14px; line-height: 1.5; }

.suggestions { list-style: none; margin: 0; padding: 0; display: grid; gap: 6px; }
.suggestion {
  width: 100%;
  padding: 7px 10px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  background: var(--panel);
  color: var(--text-muted);
  font-size: 12px;
  text-align: left;
  line-height: 1.45;
}
.suggestion:hover { color: var(--text); border-color: var(--accent); }

.turn { display: flex; flex-direction: column; gap: 6px; }
.turn--user { align-items: flex-end; }

.bubble {
  max-width: 85%;
  margin: 0;
  padding: 7px 11px;
  border-radius: var(--radius-lg);
  background: var(--panel-raised);
  color: var(--text);
  font-size: 12.5px;
  line-height: 1.5;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.bubble--failed { border: 1px solid var(--danger); }

.answer {
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--text);
  overflow-wrap: anywhere;
}
.answer :deep(p) { margin: 0 0 8px; }
.answer :deep(p:last-child) { margin-bottom: 0; }
.answer :deep(ul),
.answer :deep(ol) { margin: 0 0 8px; padding-left: 18px; }
.answer :deep(li) { margin: 2px 0; }
.answer :deep(code) {
  padding: 1px 4px;
  border-radius: var(--radius-sm);
  background: var(--bg-sunken);
  font-family: var(--font-mono);
  font-size: 11.5px;
}
.answer :deep(pre) {
  margin: 0 0 8px;
  padding: 9px 11px;
  border-radius: var(--radius);
  background: var(--bg-sunken);
  overflow-x: auto;
}
.answer :deep(pre code) { padding: 0; background: none; }
.answer :deep(a) { color: var(--accent); }

.cite-head {
  margin: 2px 0 0;
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--text-faint);
}
.cites { list-style: none; margin: 0; padding: 0; display: flex; flex-wrap: wrap; gap: 4px; }
.cite {
  padding: 1px 7px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-full);
  background: var(--bg-sunken);
  color: var(--text-muted);
  font-size: 10.5px;
  font-family: var(--font-mono);
}
.cite:hover { color: var(--accent-contrast); background: var(--accent); border-color: var(--accent); }

.note { margin: 0; line-height: 1.5; }

.thinking {
  display: flex;
  align-items: center;
  gap: 7px;
  margin: 0;
  color: var(--text-muted);
  font-size: 12px;
}
.dots { display: inline-flex; gap: 3px; }
.dots i {
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: var(--accent);
  animation: blink 1.2s var(--ease) infinite;
}
.dots i:nth-child(2) { animation-delay: 0.2s; }
.dots i:nth-child(3) { animation-delay: 0.4s; }

/* base.css already flattens every animation under prefers-reduced-motion; the
   label alone still says a turn is running. */
@keyframes blink {
  0%, 80%, 100% { opacity: 0.25; }
  40% { opacity: 1; }
}

.composer {
  display: flex;
  align-items: flex-end;
  gap: 6px;
  padding: 10px 15px;
  border-top: 1px solid var(--border);
}
.input {
  flex: 1;
  min-height: 32px;
  max-height: 108px;
  padding: 7px 9px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  background: var(--bg);
  color: var(--text);
  font-family: inherit;
  font-size: 12.5px;
  line-height: 1.5;
  resize: none;
  overflow-y: auto;
}
.input:disabled { opacity: 0.6; }
</style>
