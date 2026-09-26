<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'

import { api, ApiError } from '../api/client'
import { Perm } from '../auth/permissions'
import { useI18n } from '../i18n'
import { useAuth } from '../stores/auth'

const { t } = useI18n()
const auth = useAuth()
const router = useRouter()

const name = computed(() => auth.user?.displayName || auth.user?.username || '')

const open = ref(false)
const root = ref<HTMLElement | null>(null)
const trigger = ref<HTMLButtonElement | null>(null)
const menu = ref<HTMLElement | null>(null)

function onDocPointer(e: PointerEvent) {
  if (!root.value?.contains(e.target as Node)) open.value = false
}

watch(open, async (v) => {
  if (v) {
    document.addEventListener('pointerdown', onDocPointer)
    await nextTick()
    menu.value?.querySelector<HTMLElement>('[role="menuitem"]')?.focus()
  } else {
    document.removeEventListener('pointerdown', onDocPointer)
  }
})

onBeforeUnmount(() => document.removeEventListener('pointerdown', onDocPointer))

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && open.value) {
    e.stopPropagation()
    open.value = false
  }
}

function openUsers() {
  open.value = false
  void router.push({ name: 'admin-users' })
}

async function logout() {
  open.value = false
  await auth.logout()
  await router.replace({ name: 'login' })
}

// Change password dialog.
const dialogOpen = ref(false)
const dialog = ref<HTMLElement | null>(null)
const current = ref('')
const next = ref('')
const confirm = ref('')
const saving = ref(false)
const error = ref('')
const done = ref(false)

function openDialog() {
  open.value = false
  current.value = next.value = confirm.value = error.value = ''
  done.value = false
  dialogOpen.value = true
  void nextTick(() => dialog.value?.querySelector('input')?.focus())
}

function closeDialog() {
  dialogOpen.value = false
  void nextTick(() => trigger.value?.focus())
}

function onDialogKey(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.stopPropagation()
    closeDialog()
  } else if (e.key === 'Tab') {
    trapTab(e)
  }
}

function trapTab(e: KeyboardEvent) {
  const items = dialog.value?.querySelectorAll<HTMLElement>('input, button:not([disabled])')
  if (!items?.length) return
  const first = items[0]
  const last = items[items.length - 1]
  const active = document.activeElement
  if (e.shiftKey && active === first) {
    e.preventDefault()
    last.focus()
  } else if (!e.shiftKey && active === last) {
    e.preventDefault()
    first.focus()
  }
}

async function changePassword() {
  if (saving.value) return
  if (next.value !== confirm.value) {
    error.value = t('auth.password.mismatch')
    return
  }
  saving.value = true
  error.value = ''
  try {
    await api.changePassword(current.value, next.value)
    done.value = true
  } catch (e) {
    // 400 carries the server's policy message, shown as-is.
    if (e instanceof ApiError && e.status === 401) error.value = t('auth.password.wrongCurrent')
    else error.value = e instanceof ApiError && !e.key ? e.message : t('error.unknown')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div v-if="auth.user" ref="root" class="picker" @keydown="onKey">
    <button
      ref="trigger"
      class="btn btn--ghost btn--sm trigger"
      aria-haspopup="menu"
      :aria-expanded="open"
      :title="t('auth.menu', { name })"
      @click="open = !open"
    >
      <span class="name">{{ name }}</span>
      <span class="caret" aria-hidden="true">▾</span>
    </button>

    <div v-if="open" ref="menu" class="menu" role="menu">
      <button v-if="auth.can(Perm.UserManage)" class="opt" role="menuitem" @click="openUsers">
        {{ t('auth.users') }}
      </button>
      <button class="opt" role="menuitem" @click="openDialog">{{ t('auth.password.change') }}</button>
      <button class="opt" role="menuitem" @click="logout">{{ t('auth.logout') }}</button>
    </div>

    <div v-if="dialogOpen" class="backdrop" @click.self="closeDialog">
      <form
        ref="dialog"
        class="card"
        role="dialog"
        aria-modal="true"
        aria-labelledby="password-title"
        @keydown="onDialogKey"
        @submit.prevent="changePassword"
      >
        <h1 id="password-title">{{ t('auth.password.change') }}</h1>

        <template v-if="!done">
          <label class="field">
            <span class="label">{{ t('auth.password.current') }}</span>
            <input v-model="current" type="password" autocomplete="current-password" />
          </label>
          <label class="field">
            <span class="label">{{ t('auth.password.new') }}</span>
            <input v-model="next" type="password" autocomplete="new-password" />
          </label>
          <label class="field">
            <span class="label">{{ t('auth.password.confirm') }}</span>
            <input v-model="confirm" type="password" autocomplete="new-password" />
          </label>

          <p v-if="error" class="error" role="alert">{{ error }}</p>

          <div class="actions">
            <button type="button" class="btn btn--ghost btn--sm" @click="closeDialog">
              {{ t('auth.cancel') }}
            </button>
            <button type="submit" class="btn btn--primary btn--sm" :disabled="!current || !next || !confirm || saving">
              {{ saving ? t('auth.saving') : t('auth.password.save') }}
            </button>
          </div>
        </template>

        <template v-else>
          <p class="muted" role="status">{{ t('auth.password.changed') }}</p>
          <div class="actions">
            <button type="button" class="btn btn--primary btn--sm" @click="closeDialog">
              {{ t('settings.close') }}
            </button>
          </div>
        </template>
      </form>
    </div>
  </div>
</template>

<style scoped>
.picker { position: relative; }

.trigger { gap: var(--space-2); }
.name {
  max-width: 16ch;
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
.opt {
  display: block;
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: none;
  border: 0;
  border-radius: var(--radius);
  text-align: left;
  font-size: 13px;
  color: var(--text);
}
.opt:hover, .opt:focus-visible { background: var(--bg-sunken); }

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
  max-width: 400px;
  padding: 22px 24px 24px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-lg);
  background: var(--panel);
}
h1 { font-size: 18px; margin-bottom: var(--space-4); }

.field { display: block; margin-bottom: var(--space-3); }
.label {
  display: block;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--text-muted);
  margin-bottom: 6px;
}
.field input {
  width: 100%;
  padding: 9px 11px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  background: var(--bg-sunken);
  color: var(--text);
  font-family: inherit;
  font-size: 13px;
}
.field input:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }

.error {
  margin-bottom: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--danger);
  border-radius: var(--radius);
  background: var(--danger-soft);
  color: var(--on-danger-soft);
  font-size: 12.5px;
}
.muted { font-size: 13px; margin-bottom: var(--space-3); }
.actions { display: flex; justify-content: flex-end; gap: var(--space-2); }
</style>
