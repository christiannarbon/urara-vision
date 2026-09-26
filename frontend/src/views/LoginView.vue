<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { ApiError } from '../api/client'
import { useI18n } from '../i18n'
import { safeNext } from '../router'
import { useAuth } from '../stores/auth'

const { t, tn } = useI18n()
const auth = useAuth()
const route = useRoute()
const router = useRouter()

const username = ref('')
const password = ref('')
const busy = ref(false)
const error = ref('')
const input = ref<HTMLInputElement | null>(null)

void nextTick(() => input.value?.focus())

function messageFor(e: unknown): string {
  if (!(e instanceof ApiError)) return t('error.unknown')
  if (e.status === 401) return t('auth.invalid')
  if (e.status === 429) {
    const seconds = Number(e.headers?.get('Retry-After') ?? 60)
    return tn('auth.tooMany', Math.max(1, Math.ceil(seconds / 60)))
  }
  return e.key ? t(e.key, { status: e.status }) : e.message
}

async function submit() {
  if (!username.value.trim() || !password.value || busy.value) return
  busy.value = true
  error.value = ''
  try {
    await auth.login(username.value.trim(), password.value)
    await router.replace(safeNext(route.query.next))
  } catch (e) {
    error.value = messageFor(e)
    password.value = ''
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="gate">
    <form class="card" @submit.prevent="submit">
      <h1>{{ t('auth.title') }}</h1>

      <label class="field">
        <span class="label">{{ t('auth.username') }}</span>
        <input
          ref="input"
          v-model="username"
          name="username"
          autocomplete="username"
          autocapitalize="none"
          spellcheck="false"
          :aria-invalid="!!error"
        />
      </label>

      <label class="field">
        <span class="label">{{ t('auth.password') }}</span>
        <input
          v-model="password"
          name="password"
          type="password"
          autocomplete="current-password"
          :aria-invalid="!!error"
        />
      </label>

      <p v-if="error" class="error" role="alert">{{ error }}</p>

      <button class="btn btn--primary" type="submit" :disabled="!username.trim() || !password || busy">
        {{ busy ? t('auth.signingIn') : t('auth.signIn') }}
      </button>
    </form>
  </div>
</template>

<style scoped>
.gate {
  display: grid;
  place-items: center;
  min-height: 100%;
  padding: 40px 20px;
  background: var(--bg);
}

.card {
  width: 100%;
  max-width: 440px;
  padding: 30px 26px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-lg);
  background: var(--panel);
}

h1 { font-size: 18px; margin-bottom: 20px; }

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
.field input[aria-invalid='true'] { border-color: var(--danger); }

.error {
  margin-bottom: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--danger);
  border-radius: var(--radius);
  background: var(--danger-soft);
  color: var(--on-danger-soft);
  font-size: 12.5px;
}

.btn[type='submit'] { width: 100%; justify-content: center; }
</style>
