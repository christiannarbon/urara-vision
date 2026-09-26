<script setup lang="ts">
/** User administration: list, create, change role, reset password, delete. */
import { computed, nextTick, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'

import type { Role, User } from '../api/types'
import { Perm } from '../auth/permissions'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import UserFormDialog from '../components/UserFormDialog.vue'
import { useI18n } from '../i18n'
import { useAuth } from '../stores/auth'
import { describeError, useUsers } from '../stores/users'

const { t, locale } = useI18n()
const auth = useAuth()
const store = useUsers()
const { users, loading, error } = storeToRefs(store)

const ROLES: Role[] = ['viewer', 'creator', 'admin']

const notice = ref('')
const actionError = ref('')
const form = ref<{ mode: 'create' | 'password'; user?: User } | null>(null)
const pendingDelete = ref<User | null>(null)
const deleting = ref(false)
let opener: HTMLElement | null = null

onMounted(() => void store.load())

const isSelf = (u: User) => u.id === auth.user?.id
const nameOf = (u: User) => u.displayName || u.username

function formatDate(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleDateString(locale.value)
}

async function changeRole(u: User, e: Event) {
  const select = e.target as HTMLSelectElement
  actionError.value = notice.value = ''
  try {
    await store.update(u.id, { role: select.value as Role })
  } catch (err) {
    // The list was not reloaded, so the select still shows the refused choice.
    select.value = u.role
    actionError.value = describeError(err)
  }
}

function openForm(mode: 'create' | 'password', user?: User) {
  opener = document.activeElement as HTMLElement | null
  actionError.value = notice.value = ''
  form.value = { mode, user }
}

function closeForm() {
  form.value = null
  void nextTick(() => {
    if (opener?.isConnected) opener.focus()
    opener = null
  })
}

function formDone() {
  if (form.value?.mode === 'password') notice.value = t('users.passwordReset')
  closeForm()
}

const deleteMessage = computed(() =>
  pendingDelete.value ? t('users.delete.message', { name: nameOf(pendingDelete.value) }) : '',
)

async function confirmDelete() {
  const u = pendingDelete.value
  if (!u) return
  deleting.value = true
  actionError.value = notice.value = ''
  try {
    await store.remove(u.id)
  } catch (err) {
    actionError.value = describeError(err)
  } finally {
    deleting.value = false
    pendingDelete.value = null
  }
}
</script>

<template>
  <main class="main">
    <div class="page">
      <header class="head">
        <h1>{{ t('users.title') }}</h1>
        <button class="btn btn--primary btn--sm" @click="openForm('create')">{{ t('users.new') }}</button>
      </header>

      <p v-if="error || actionError" class="banner" role="alert">{{ actionError || error }}</p>
      <p v-if="notice" class="banner banner--ok" role="status">{{ notice }}</p>

      <p v-if="loading && !users.length" class="muted">{{ t('users.loading') }}</p>

      <table v-else class="users">
        <thead>
          <tr>
            <th scope="col">{{ t('users.col.username') }}</th>
            <th scope="col">{{ t('users.col.displayName') }}</th>
            <th scope="col">{{ t('users.col.role') }}</th>
            <th scope="col">{{ t('users.col.created') }}</th>
            <th scope="col"><span class="sr-only">{{ t('users.col.actions') }}</span></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="u in users" :key="u.id" :data-user="u.username">
            <td>
              {{ u.username }}
              <span v-if="isSelf(u)" class="tag">{{ t('users.you') }}</span>
            </td>
            <td>{{ u.displayName }}</td>
            <td>
              <select
                class="input"
                :value="u.role"
                :aria-label="t('users.role.label', { name: nameOf(u) })"
                @change="changeRole(u, $event)"
              >
                <option v-for="r in ROLES" :key="r" :value="r">{{ t(`users.role.${r}`) }}</option>
              </select>
            </td>
            <td class="faint">{{ formatDate(u.createdAt) }}</td>
            <td class="actions">
              <button class="btn btn--ghost btn--sm" @click="openForm('password', u)">
                {{ t('users.resetPassword') }}
              </button>
              <button
                v-if="!isSelf(u) && auth.can(Perm.UserDelete)"
                class="btn btn--ghost btn--sm"
                :aria-label="t('users.delete.named', { name: nameOf(u) })"
                @click="pendingDelete = u"
              >
                {{ t('users.delete') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <UserFormDialog v-if="form" :mode="form.mode" :user="form.user" @close="closeForm" @done="formDone" />
    <ConfirmDialog
      :open="pendingDelete !== null"
      :title="t('users.delete.title')"
      :message="deleteMessage"
      :confirm-label="t('users.delete')"
      danger
      :busy="deleting"
      @confirm="confirmDelete"
      @cancel="pendingDelete = null"
    />
  </main>
</template>

<style scoped>
.main { flex: 1; min-height: 0; overflow-y: auto; }
.page { max-width: 880px; margin: 0 auto; padding: 20px 16px 40px; display: flex; flex-direction: column; gap: 16px; }
.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.head h1 { font-size: 18px; margin: 0; }
.banner {
  margin: 0;
  padding: 8px 12px;
  border-radius: var(--radius-sm);
  background: var(--danger-soft);
  color: var(--on-danger-soft);
  font-size: 12.5px;
}
.banner--ok { background: var(--bg-sunken); color: var(--text); }
.users { width: 100%; border-collapse: collapse; font-size: 13px; }
.users th {
  text-align: left;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--text-muted);
  padding: 6px 8px;
  border-bottom: 1px solid var(--border);
}
.users td { padding: 8px; border-bottom: 1px solid var(--border); vertical-align: middle; }
.actions { text-align: right; white-space: nowrap; }
.tag {
  margin-left: 6px;
  padding: 1px 6px;
  border-radius: var(--radius-sm);
  background: var(--bg-sunken);
  font-size: 11px;
  color: var(--text-muted);
}
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
}
</style>
