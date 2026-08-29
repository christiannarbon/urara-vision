<script setup lang="ts">
/** Collects the API token when the backend answers 401.
 *
 *  This stands in front of everything rather than appearing as a banner: with
 *  no accepted token there is no snapshot list, no graph and nothing to look
 *  at, so a dismissible message would only leave the reader on a blank shell.
 */
import { nextTick, ref, watch } from 'vue'

const props = defineProps<{ busy?: boolean }>()
const emit = defineEmits<{ (e: 'submit', token: string): void }>()

const token = ref('')
const rejected = ref(false)
const input = ref<HTMLInputElement | null>(null)

void nextTick(() => input.value?.focus())

// Any edit clears the rejection notice, so it describes the value that was
// actually refused rather than whatever is in the field now.
watch(token, () => {
  rejected.value = false
})

function submit() {
  const t = token.value.trim()
  if (!t || props.busy) return
  emit('submit', t)
}

/** Called by the parent when the backend refused the token. */
function markRejected() {
  rejected.value = true
  void nextTick(() => input.value?.select())
}

defineExpose({ markRejected })
</script>

<template>
  <div class="gate">
    <form class="card" @submit.prevent="submit">
      <h1>This instance needs a token</h1>
      <p class="muted">
        The API is protected by a shared token. Ask whoever runs this deployment for it —
        it is the <code>API_TOKEN</code> the backend was started with.
      </p>

      <label class="field">
        <span class="label">API token</span>
        <input
          ref="input"
          v-model="token"
          type="password"
          autocomplete="off"
          spellcheck="false"
          :aria-invalid="rejected"
          placeholder="Paste the token"
        />
      </label>

      <p v-if="rejected" class="error" role="alert">
        That token was not accepted. Check it for a missing or trailing character.
      </p>

      <button class="btn btn--primary" type="submit" :disabled="!token.trim() || busy">
        {{ busy ? 'Checking…' : 'Continue' }}
      </button>

      <p class="faint tiny note">
        It is kept in this browser's local storage so you are not asked on every visit.
      </p>
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

h1 { font-size: 18px; margin-bottom: 8px; }
.card > .muted { font-size: 13px; line-height: 1.6; margin-bottom: 20px; }

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
.note { margin-top: 14px; text-align: center; }
.tiny { font-size: 11px; }
</style>
