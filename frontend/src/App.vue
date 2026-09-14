<script setup lang="ts">
/** Application shell: token gate, topbar, banners and the routed view. */
import { computed, nextTick, onMounted, onBeforeUnmount, ref, watch, watchEffect } from 'vue'
import { storeToRefs } from 'pinia'
import { RouterView, useRoute, useRouter } from 'vue-router'

import ApiTokenGate from './components/ApiTokenGate.vue'
import LanguagePicker from './components/LanguagePicker.vue'
import SettingsDialog from './components/SettingsDialog.vue'
import ThemePicker from './components/ThemePicker.vue'
import { useI18n } from './i18n'
import { useChat } from './stores/chat'
import { useFeatures } from './stores/features'
import { useUi } from './stores/ui'
import { useWorkspace } from './stores/workspace'

const { t, tn } = useI18n()

// index.html carries an English title for the first paint; from here the tab
// follows the reader's language like everything else.
watchEffect(() => {
  document.title = t('app.title')
})

const route = useRoute()
const router = useRouter()

const store = useWorkspace()
const {
  snapshot,
  selectedId,
  statusMessage,
  error,
  hasSnapshot,
  parseFailures,
  findings,
  hasDiagnostics,
  needsParseNotice,
  authRequired,
} = storeToRefs(store)

const tokenGate = ref<InstanceType<typeof ApiTokenGate> | null>(null)
const tokenChecking = ref(false)

/** Hands the token to the store and tells the gate if it was refused. */
async function onTokenSubmit(token: string) {
  tokenChecking.value = true
  try {
    if (await store.submitApiToken(token)) void features.load()
    else tokenGate.value?.markRejected()
  } finally {
    tokenChecking.value = false
  }
}

const ui = useUi()
const { searchOpen, diagnosticsOpen } = storeToRefs(ui)

const chat = useChat()
const { open: chatOpen } = storeToRefs(chat)

const features = useFeatures()
const { chatEnabled } = storeToRefs(features)
const settingsOpen = ref(false)
const settingsButton = ref<HTMLButtonElement | null>(null)

function closeSettings() {
  settingsOpen.value = false
  void nextTick(() => settingsButton.value?.focus())
}

watch(chatEnabled, (on) => {
  if (!on) chat.closePanel()
})

/** The Diagnostics button carries a marker rather than a count: the number of
 *  findings says nothing about whether any of them matter, and a big number
 *  reads as a failure when most findings are merely worth a look. */
const marker = computed(() => {
  if (parseFailures.value.length) return 'error'
  if (findings.value.length) return 'warning'
  return null
})

const parseNotice = computed(() => tn('banner.parseFailures', parseFailures.value.length))

/** Spelled out separately so the sentence agrees in number. */
const parseNoticeDetail = computed(() =>
  tn('banner.parseFailuresDetail', parseFailures.value.length),
)

function reviewParseFailures() {
  ui.showDiagnostics()
  store.acknowledgeParseFailures()
}

// Home, not "has a snapshot": a not-found project has none and still needs a way back.
const atHome = computed(() => route.name === 'home')

onMounted(() => {
  void features.load()
  window.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))

function onKeydown(e: KeyboardEvent) {
  const inField =
    e.target instanceof HTMLElement &&
    /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName)

  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault()
    if (hasSnapshot.value) searchOpen.value = true
    return
  }
  if (e.key === '/' && !inField && hasSnapshot.value) {
    e.preventDefault()
    searchOpen.value = true
    return
  }
  if (e.key === 'Escape' && !searchOpen.value && !settingsOpen.value) {
    if (chatOpen.value) chat.closePanel()
    else if (diagnosticsOpen.value) diagnosticsOpen.value = false
    else if (selectedId.value) void store.select(null)
  }
}

function backToPicker() {
  void router.push('/')
}
</script>

<template>
  <!-- The token gate replaces the shell entirely: nothing behind it can load
       until the backend accepts a token. -->
  <ApiTokenGate v-if="authRequired" ref="tokenGate" :busy="tokenChecking" @submit="onTokenSubmit" />

  <div v-else class="app">
    <header class="topbar">
      <button
        type="button"
        class="brand"
        :class="{ 'brand--link': !atHome }"
        :disabled="atHome"
        :title="atHome ? undefined : t('topbar.home')"
        @click="backToPicker"
      >
        <span class="mark" aria-hidden="true" />
        <span class="brand-name">Urara Vision</span>
      </button>

      <div v-if="hasSnapshot" class="snap-label">
        <span class="snap-name">{{ snapshot?.name }}</span>
        <span v-if="statusMessage" class="faint tiny">{{ statusMessage }}</span>
      </div>

      <div class="spacer" />

      <template v-if="hasSnapshot">
        <button class="btn btn--ghost btn--sm" :title="t('topbar.search.title')" @click="searchOpen = true">
          {{ t('topbar.search') }} <kbd>⌘K</kbd>
        </button>
        <button
          class="btn btn--ghost btn--sm"
          :aria-expanded="diagnosticsOpen"
          :title="hasDiagnostics ? t('topbar.diagnostics.titleAttention') : t('topbar.diagnostics.title')"
          @click="ui.toggleDiagnostics"
        >
          {{ t('topbar.diagnostics') }}
          <span
            v-if="marker"
            class="mark-flag"
            :class="`mark-flag--${marker}`"
            :aria-label="t('topbar.diagnostics.flag')"
            >!</span
          >
        </button>
        <button
          v-if="chatEnabled"
          class="btn btn--ghost btn--sm"
          :aria-expanded="chatOpen"
          :title="chatOpen ? t('chat.close') : t('chat.open')"
          @click="ui.toggleChat"
        >
          <span class="glyph" aria-hidden="true">◗</span>
          {{ chatOpen ? t('chat.close') : t('chat.open') }}
        </button>
        <button class="btn btn--ghost btn--sm" @click="backToPicker">
          {{ t('topbar.newIngest') }}
        </button>
      </template>

      <LanguagePicker />
      <ThemePicker />
      <button
        ref="settingsButton"
        class="btn btn--ghost btn--sm"
        aria-haspopup="dialog"
        :title="t('settings.open')"
        @click="settingsOpen = true"
      >
        {{ t('settings.open') }}
      </button>
    </header>

    <p v-if="error" class="banner" role="alert">
      {{ error }}
      <button class="btn btn--ghost btn--sm" @click="store.dismissError">
        {{ t('banner.dismiss') }}
      </button>
    </p>

    <p v-if="hasSnapshot && needsParseNotice" class="banner banner--notice" role="alert">
      <span>
        <strong>{{ parseNotice }}</strong>
        {{ parseNoticeDetail }}
      </span>
      <span class="banner-actions">
        <button class="btn btn--ghost btn--sm" @click="reviewParseFailures">
          {{ t('banner.review') }}
        </button>
        <button class="btn btn--ghost btn--sm" @click="store.acknowledgeParseFailures">
          {{ t('banner.dismiss') }}
        </button>
      </span>
    </p>

    <RouterView />

    <SettingsDialog v-if="settingsOpen" @close="closeSettings" />
  </div>
</template>

<style scoped>
.app {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.topbar {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 0 var(--space-3);
  height: 46px;
  flex: none;
  background: var(--panel);
  border-bottom: 1px solid var(--border);
}

/* The wordmark doubles as the way home: from a loaded model it returns to the
   picker, where the previous ingests are listed. */
.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0;
  border: 0;
  background: none;
  color: inherit;
  border-radius: var(--radius-sm);
}
.brand--link { cursor: pointer; }
.brand--link:hover .brand-name { color: var(--accent); }
.brand:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; }
.mark {
  width: 14px;
  height: 14px;
  border-radius: var(--radius-sm);
  background: var(--fact);
  box-shadow: 6px 0 0 -3px var(--dim);
}
.brand-name {
  font-family: var(--font-display);
  font-size: 14px;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.snap-label {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-left: 14px;
  padding-left: 14px;
  border-left: 1px solid var(--border);
  min-width: 0;
}
.snap-name {
  font-size: 12.5px;
  color: var(--text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 34ch;
}
.tiny { font-size: 11px; }

.spacer { flex: 1; }

kbd {
  margin-left: var(--space-1);
  padding: 0 var(--space-1);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-sm);
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--text-faint);
}

/* A marker, not a tally: it says "look in here", not "you have 47 problems". */
/* The only visual the topbar had for an open panel was aria-expanded; giving
   it a style marks Diagnostics as well as Chat. */
.topbar .btn[aria-expanded='true'] {
  background: var(--bg-sunken);
  border-color: var(--border-strong);
  color: var(--text);
}
.glyph { font-size: 11px; color: var(--text-faint); }
.btn[aria-expanded='true'] .glyph { color: var(--accent); }

.mark-flag {
  display: inline-grid;
  place-items: center;
  margin-left: 5px;
  width: 14px;
  height: 14px;
  border-radius: var(--radius-full);
  font-size: 10px;
  font-weight: 700;
  line-height: 1;
}
.mark-flag--error { background: var(--danger); color: var(--on-danger); }
.mark-flag--warning { background: var(--warning); color: var(--on-warning); }

.banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin: 0;
  padding: 8px 14px;
  flex: none;
  background: var(--danger-soft);
  color: var(--on-danger-soft);
  border-bottom: 1px solid var(--danger);
  font-size: 12.5px;
}
.banner--notice {
  background: var(--warning-soft);
  color: var(--on-warning-soft);
  border-bottom-color: var(--warning);
}
.banner-actions { display: flex; gap: 4px; flex: none; }

@media (max-width: 900px) {
  .snap-label { display: none; }
}
</style>
