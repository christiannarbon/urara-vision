/** Signing in and out through the real client over a stubbed fetch. */

import { flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { mountApp } from '../helpers/mountApp'
import { messages as en } from '../../src/i18n/messages/en'

interface Call {
  method: string
  path: string
  headers: Headers
  credentials?: RequestCredentials
}

let calls: Call[] = []
let signedIn = false
/** Answers for paths the defaults below do not cover. */
let overrides: Record<string, () => Response> = {}

const USER = {
  id: 'u1', username: 'alice', displayName: 'Alice', role: 'viewer',
  createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
}

function json(status: number, body: unknown, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } })
}

function respond(method: string, path: string): Response {
  const key = `${method} ${path}`
  if (overrides[key]) return overrides[key]()
  switch (key) {
    case 'POST /api/v1/auth/login':
      signedIn = true
      return json(200, { user: USER })
    case 'POST /api/v1/auth/logout':
      signedIn = false
      return new Response(null, { status: 204 })
    case 'GET /api/v1/auth/me':
      return signedIn
        ? json(200, { user: USER, kind: 'user', permissions: ['chat.use', 'note.write', 'project.view'] })
        : json(401, { error: 'not signed in' })
    case 'GET /api/v1/features':
      return json(200, { chat: { available: false, enabled: false } })
    case 'GET /api/v1/projects':
      return json(200, { projects: [] })
  }
  return json(404, { error: 'not found' })
}

beforeEach(() => {
  setActivePinia(createPinia())
  calls = []
  signedIn = false
  overrides = {}
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    const method = (init?.method ?? 'GET').toUpperCase()
    const path = String(url).split('?')[0]
    calls.push({ method, path, headers: new Headers(init?.headers), credentials: init?.credentials })
    return respond(method, path)
  }))
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

async function signIn(w: VueWrapper, username: string, password: string) {
  await w.find('input[autocomplete="username"]').setValue(username)
  await w.find('input[autocomplete="current-password"]').setValue(password)
  await w.find('form').trigger('submit')
  await flushPromises()
}

describe('signing in', () => {
  it('sends a signed-out reader on a protected route to login, with next', async () => {
    const w = await mountApp('/projects/x', { signedIn: false })
    const route = w.vm.$router.currentRoute.value
    expect(route.name).toBe('login')
    expect(route.query.next).toBe('/projects/x')
    expect(w.find('.topbar').exists()).toBe(false)
    w.unmount()
  })

  it('goes to next after a successful login', async () => {
    const w = await mountApp('/projects/x', { signedIn: false })
    await signIn(w, 'Alice', 'alice-password-123')
    expect(w.vm.$router.currentRoute.value.fullPath).toBe('/projects/x')
    expect(w.find('.topbar').text()).toContain('Alice')
    w.unmount()
  })

  it('ignores a next that leaves the site', async () => {
    const w = await mountApp('/login?next=//evil.com', { signedIn: false })
    await signIn(w, 'alice', 'alice-password-123')
    expect(w.vm.$router.currentRoute.value.fullPath).toBe('/')
    w.unmount()
  })

  it('shows the generic message for a wrong password and stays on login', async () => {
    overrides['POST /api/v1/auth/login'] = () => json(401, { error: 'invalid username or password' })
    const w = await mountApp('/', { signedIn: false })
    await signIn(w, 'alice', 'wrong-password-123')
    expect(w.vm.$router.currentRoute.value.name).toBe('login')
    expect(w.find('[role="alert"]').text()).toBe(en['auth.invalid'])
    w.unmount()
  })

  it('says how long to wait when rate-limited', async () => {
    overrides['POST /api/v1/auth/login'] = () => json(429, { error: 'too many' }, { 'Retry-After': '540' })
    const w = await mountApp('/', { signedIn: false })
    await signIn(w, 'alice', 'wrong-password-123')
    expect(w.find('[role="alert"]').text()).toBe('Too many attempts; try again in 9 minutes.')
    w.unmount()
  })
})

describe('the user menu', () => {
  it('moves focus into the menu and back to the trigger after the dialog', async () => {
    const w = await mountApp('/projects/x', { signedIn: false, attachTo: document.body })
    await signIn(w, 'alice', 'alice-password-123')

    const trigger = w.find('.topbar [aria-haspopup="menu"]')
    await trigger.trigger('click')
    await flushPromises()
    const change = w.findAll('[role="menuitem"]').find((b) => b.text() === en['auth.password.change'])!
    expect(document.activeElement).toBe(change.element)

    await change.trigger('click')
    await flushPromises()
    await w.find('[role="dialog"]').trigger('keydown', { key: 'Escape' })
    await flushPromises()
    expect(w.find('[role="dialog"]').exists()).toBe(false)
    expect(document.activeElement).toBe(trigger.element)
    w.unmount()
  })

  it('keeps Tab inside the password dialog', async () => {
    const w = await mountApp('/projects/x', { signedIn: false, attachTo: document.body })
    await signIn(w, 'alice', 'alice-password-123')
    await w.find('.topbar [aria-haspopup="menu"]').trigger('click')
    await w.findAll('[role="menuitem"]').find((b) => b.text() === en['auth.password.change'])!.trigger('click')
    await flushPromises()

    const dialog = w.find('[role="dialog"]')
    const inputs = dialog.findAll('input')
    const cancel = dialog.findAll('button').find((b) => b.text() === en['auth.cancel'])!
    // The submit button is disabled while the fields are empty, so Cancel is last.
    ;(cancel.element as HTMLElement).focus()
    await dialog.trigger('keydown', { key: 'Tab' })
    expect(document.activeElement).toBe(inputs[0].element)

    ;(inputs[0].element as HTMLElement).focus()
    await dialog.trigger('keydown', { key: 'Tab', shiftKey: true })
    expect(document.activeElement).toBe(cancel.element)
    w.unmount()
  })
})

describe('the session', () => {
  it('returns to login when a request answers 401 mid-session', async () => {
    const w = await mountApp('/projects/x', { signedIn: false })
    await signIn(w, 'alice', 'alice-password-123')

    overrides['GET /api/v1/projects'] = () => json(401, { error: 'not signed in' })
    await w.vm.$router.push('/')
    await flushPromises()

    const route = w.vm.$router.currentRoute.value
    expect(route.name).toBe('login')
    expect(route.query.next).toBe('/')
    w.unmount()
  })

  it('logs out to the login page, and Back does not show the app', async () => {
    const w = await mountApp('/projects/x', { signedIn: false, attachTo: document.body })
    await signIn(w, 'alice', 'alice-password-123')

    await w.find('.topbar [aria-haspopup="menu"]').trigger('click')
    const logout = w.findAll('[role="menuitem"]').find((b) => b.text() === en['auth.logout'])
    await logout!.trigger('click')
    await flushPromises()
    expect(w.vm.$router.currentRoute.value.name).toBe('login')

    w.vm.$router.back()
    await flushPromises()
    expect(w.vm.$router.currentRoute.value.name).toBe('login')
    w.unmount()
  })

  it('marks unsafe requests with X-Requested-With and sends the cookie on all', async () => {
    const w = await mountApp('/projects/x', { signedIn: false })
    await signIn(w, 'alice', 'alice-password-123')

    const login = calls.find((c) => c.path === '/api/v1/auth/login')!
    const me = calls.find((c) => c.path === '/api/v1/auth/me')!
    expect(login.headers.get('X-Requested-With')).toBe('urara')
    expect(me.headers.has('X-Requested-With')).toBe(false)
    expect(calls.every((c) => c.credentials === 'same-origin')).toBe(true)
    expect(calls.some((c) => c.headers.has('Authorization'))).toBe(false)
    w.unmount()
  })
})
