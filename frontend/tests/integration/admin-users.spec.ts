/** The /admin/users page, against a mocked API. */

import { flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { User } from '../../src/api/types'
import { messages as en } from '../../src/i18n/messages/en'

vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return {
    ...actual,
    api: {
      features: vi.fn(),
      listProjects: vi.fn(),
      listUsers: vi.fn(),
      createUser: vi.fn(),
      updateUser: vi.fn(),
      resetPassword: vi.fn(),
      deleteUser: vi.fn(),
    },
  }
})

const { api, ApiError } = await import('../../src/api/client')
const { mountApp, ADMIN_PERMISSIONS } = await import('../helpers/mountApp')

function user(id: string, username: string, role: string): User {
  return {
    id, username, displayName: username, role,
    createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
  }
}

// seedAuth signs in as u-admin.
const USERS = [user('u-admin', 'admin', 'admin'), user('u-vera', 'vera', 'viewer')]
const GOOD = 'a-long-enough-password'

let w: VueWrapper

async function page(permissions = ADMIN_PERMISSIONS) {
  w = await mountApp('/admin/users', { attachTo: document.body, permissions })
  await flushPromises()
  return w
}

const row = (username: string) => w.find(`tr[data-user="${username}"]`)
const button = (text: string) => w.findAll('button').find((b) => b.text() === text)!

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.mocked(api.features).mockResolvedValue({ chat: { available: false, enabled: false } })
  vi.mocked(api.listProjects).mockResolvedValue({ projects: [] })
  vi.mocked(api.listUsers).mockResolvedValue({ users: USERS })
  vi.mocked(api.updateUser).mockResolvedValue(USERS[1])
  vi.mocked(api.createUser).mockResolvedValue(USERS[1])
  vi.mocked(api.resetPassword).mockResolvedValue(undefined)
  vi.mocked(api.deleteUser).mockResolvedValue(undefined)
})

afterEach(() => w?.unmount())

describe('the list', () => {
  it('marks the current user, who has no delete action', async () => {
    await page()
    expect(row('admin').text()).toContain(en['users.you'])
    expect(row('admin').find('[aria-label="Delete admin"]').exists()).toBe(false)
    expect(row('vera').text()).not.toContain(en['users.you'])
    expect(row('vera').find('[aria-label="Delete vera"]').exists()).toBe(true)
  })

  it('hides delete without user.delete', async () => {
    await page(ADMIN_PERMISSIONS.filter((p) => p !== 'user.delete'))
    expect(row('vera').find('[aria-label="Delete vera"]').exists()).toBe(false)
  })
})

describe('changing a role', () => {
  it('calls updateUser', async () => {
    await page()
    await row('vera').find('select').setValue('creator')
    await flushPromises()
    expect(api.updateUser).toHaveBeenCalledWith('u-vera', { role: 'creator' })
  })

  it('reverts the select and shows the message on a 409', async () => {
    vi.mocked(api.updateUser).mockRejectedValue(
      new ApiError('at least one admin must remain', 409, undefined, { error: 'at least one admin must remain' }),
    )
    await page()
    const select = row('admin').find('select')
    await select.setValue('viewer')
    await flushPromises()

    expect((select.element as HTMLSelectElement).value).toBe('admin')
    expect(w.find('.banner').text()).toContain('at least one admin must remain')
  })
})

describe('creating a user', () => {
  async function fill(password: string, confirm: string) {
    await button(en['users.new']).trigger('click')
    const form = w.find('form[role="dialog"]')
    await form.find('input[name="username"]').setValue('cody')
    await form.find('input[name="displayName"]').setValue('Cody')
    await form.find('select[name="role"]').setValue('creator')
    await form.find('input[name="password"]').setValue(password)
    await form.find('input[name="confirm"]').setValue(confirm)
    await form.trigger('submit')
    await flushPromises()
    return form
  }

  it('checks the confirmation before calling the API', async () => {
    await page()
    const form = await fill(GOOD, GOOD + 'x')
    expect(form.find('[role="alert"]').text()).toBe(en['users.form.mismatch'])
    expect(api.createUser).not.toHaveBeenCalled()
  })

  it('checks the length before calling the API', async () => {
    await page()
    await fill('short', 'short')
    expect(api.createUser).not.toHaveBeenCalled()
  })

  it('creates, closes and reloads', async () => {
    await page()
    await fill(GOOD, GOOD)
    expect(api.createUser).toHaveBeenCalledWith({
      username: 'cody', displayName: 'Cody', role: 'creator', password: GOOD,
    })
    expect(w.find('form[role="dialog"]').exists()).toBe(false)
    expect(api.listUsers).toHaveBeenCalledTimes(2)
  })

  it('shows a conflict as the server words it', async () => {
    vi.mocked(api.createUser).mockRejectedValue(new ApiError('username already exists', 409))
    await page()
    const form = await fill(GOOD, GOOD)
    expect(form.find('[role="alert"]').text()).toBe('username already exists')
  })
})

describe('resetting a password', () => {
  it('sends the new password', async () => {
    await page()
    await row('vera').findAll('button').find((b) => b.text() === en['users.resetPassword'])!.trigger('click')
    const form = w.find('form[role="dialog"]')
    expect(form.find('input[name="username"]').exists()).toBe(false)
    await form.find('input[name="password"]').setValue(GOOD)
    await form.find('input[name="confirm"]').setValue(GOOD)
    await form.trigger('submit')
    await flushPromises()

    expect(api.resetPassword).toHaveBeenCalledWith('u-vera', GOOD)
    expect(w.text()).toContain(en['users.passwordReset'])
  })
})

describe('deleting a user', () => {
  it('asks first, then deletes', async () => {
    await page()
    await row('vera').find('[aria-label="Delete vera"]').trigger('click')
    expect(w.text()).toContain('Delete vera? Their sessions and conversations are removed.')
    expect(api.deleteUser).not.toHaveBeenCalled()

    const confirm = w.findAll('button').filter((b) => b.text() === en['users.delete']).at(-1)!
    await confirm.trigger('click')
    await flushPromises()
    expect(api.deleteUser).toHaveBeenCalledWith('u-vera')
  })
})

describe('the route', () => {
  it('sends a user without user.manage home', async () => {
    await page(['chat.use', 'note.write', 'project.view'])
    expect(w.vm.$route.path).toBe('/')
    expect(w.find('.banner').text()).toContain(en['access.denied'])
    expect(api.listUsers).not.toHaveBeenCalled()
  })
})
