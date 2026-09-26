<script setup lang="ts">
/** Create a user, or set an existing user's password. */
import { computed, nextTick, onMounted, ref } from 'vue'

import type { Role, User } from '../api/types'
import { useI18n } from '../i18n'
import { describeError, useUsers } from '../stores/users'

// Bytes, as the server counts them.
const MIN_PASSWORD_BYTES = 12

const props = defineProps<{ mode: 'create' | 'password'; user?: User }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'done'): void }>()

const { t } = useI18n()
const store = useUsers()

const username = ref('')
const displayName = ref('')
const role = ref<Role>('viewer')
const password = ref('')
const confirm = ref('')
const saving = ref(false)
const error = ref('')
const dialog = ref<HTMLElement | null>(null)

const title = computed(() =>
  props.mode === 'create'
    ? t('users.form.createTitle')
    : t('users.form.passwordTitle', { name: props.user?.displayName || props.user?.username || '' }),
)

const complete = computed(
  () => !!password.value && !!confirm.value && (props.mode === 'password' || !!username.value),
)

onMounted(() => void nextTick(() => dialog.value?.querySelector('input')?.focus()))

async function submit() {
  if (saving.value) return
  if (password.value !== confirm.value) {
    error.value = t('users.form.mismatch')
    return
  }
  if (new TextEncoder().encode(password.value).length < MIN_PASSWORD_BYTES) {
    error.value = t('users.form.tooShort', { n: MIN_PASSWORD_BYTES })
    return
  }
  saving.value = true
  error.value = ''
  try {
    if (props.mode === 'create') {
      await store.create({
        username: username.value,
        displayName: displayName.value,
        role: role.value,
        password: password.value,
      })
    } else if (props.user) {
      await store.resetPassword(props.user.id, password.value)
    }
    emit('done')
  } catch (e) {
    error.value = describeError(e)
  } finally {
    saving.value = false
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.stopPropagation()
    if (!saving.value) emit('close')
  } else if (e.key === 'Tab') {
    const items = dialog.value?.querySelectorAll<HTMLElement>('input, select, button:not([disabled])')
    if (!items?.length) return
    const first = items[0]
    const last = items[items.length - 1]
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault()
      last.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first.focus()
    }
  }
}
</script>

<template>
  <div class="backdrop" @click.self="!saving && emit('close')">
    <form
      ref="dialog"
      class="card"
      role="dialog"
      aria-modal="true"
      aria-labelledby="user-form-title"
      @keydown="onKeydown"
      @submit.prevent="submit"
    >
      <h1 id="user-form-title">{{ title }}</h1>

      <template v-if="mode === 'create'">
        <label class="field">
          <span class="label">{{ t('users.form.username') }}</span>
          <input v-model="username" name="username" autocomplete="off" />
        </label>
        <label class="field">
          <span class="label">{{ t('users.form.displayName') }}</span>
          <input v-model="displayName" name="displayName" autocomplete="off" />
        </label>
        <label class="field">
          <span class="label">{{ t('users.form.role') }}</span>
          <select v-model="role" name="role">
            <option value="viewer">{{ t('users.role.viewer') }}</option>
            <option value="creator">{{ t('users.role.creator') }}</option>
            <option value="admin">{{ t('users.role.admin') }}</option>
          </select>
        </label>
      </template>

      <label class="field">
        <span class="label">{{ t('users.form.password') }}</span>
        <input v-model="password" name="password" type="password" autocomplete="new-password" />
      </label>
      <label class="field">
        <span class="label">{{ t('users.form.confirm') }}</span>
        <input v-model="confirm" name="confirm" type="password" autocomplete="new-password" />
      </label>

      <p v-if="error" class="error" role="alert">{{ error }}</p>

      <div class="actions">
        <button type="button" class="btn btn--ghost btn--sm" :disabled="saving" @click="emit('close')">
          {{ t('users.form.cancel') }}
        </button>
        <button type="submit" class="btn btn--primary btn--sm" :disabled="!complete || saving">
          {{ saving ? t('users.form.saving') : mode === 'create' ? t('users.form.create') : t('users.form.save') }}
        </button>
      </div>
    </form>
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
.field input,
.field select {
  width: 100%;
  padding: 9px 11px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  background: var(--bg-sunken);
  color: var(--text);
  font-family: inherit;
  font-size: 13px;
}
.field input:focus-visible,
.field select:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }

.error {
  margin-bottom: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--danger);
  border-radius: var(--radius);
  background: var(--danger-soft);
  color: var(--on-danger-soft);
  font-size: 12.5px;
}
.actions { display: flex; justify-content: flex-end; gap: var(--space-2); }
</style>
