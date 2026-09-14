/** The settings dialog, and the chat UI following the server's switch. */

import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Features } from '../../src/api/types'

const features = vi.fn<() => Promise<Features>>()
const patchSettings = vi.fn<(body: { chatEnabled: boolean }) => Promise<Features>>()

vi.mock('../../src/api/client', async () => {
  const actual = await vi.importActual<typeof import('../../src/api/client')>('../../src/api/client')
  return {
    ...actual,
    api: new Proxy(
      {},
      {
        get: (_, prop) => {
          if (prop === 'features') return features
          if (prop === 'patchSettings') return patchSettings
          return vi.fn().mockResolvedValue({ snapshots: [] })
        },
      },
    ),
  }
})

const App = (await import('../../src/App.vue')).default
const SettingsDialog = (await import('../../src/components/SettingsDialog.vue')).default
const ApiTokenGate = (await import('../../src/components/ApiTokenGate.vue')).default
const { useWorkspace } = await import('../../src/stores/workspace')
const { useChat } = await import('../../src/stores/chat')
const { useFeatures } = await import('../../src/stores/features')
const { messages: en } = await import('../../src/i18n/messages/en')
const { messages: ja } = await import('../../src/i18n/messages/ja')
const { setLocale } = await import('../../src/i18n')

const STATS = {
  domains: 1,
  tables: 1,
  columns: 1,
  relationships: 0,
  lineageEdges: 0,
  sourceTables: 0,
  conformed: 0,
  filesParsed: 1,
  filesSkipped: 0,
  diagnostics: 0,
}

const state = (available: boolean, enabled: boolean): Features => ({ chat: { available, enabled } })

beforeEach(() => {
  setActivePinia(createPinia())
  features.mockReset()
  patchSettings.mockReset()
})

afterEach(() => setLocale('en'))

async function mountApp(f: Features) {
  features.mockResolvedValue(f)
  useWorkspace().snapshot = { id: 's1', name: 's1', sourceLabel: 'docs', createdAt: '', stats: STATS }
  const w = mount(App, { global: { stubs: { GraphCanvas: true } }, attachTo: document.body })
  await flushPromises()
  return w
}

const chatButton = (w: Awaited<ReturnType<typeof mountApp>>) =>
  w.findAll('.topbar button').find((b) => /chat/i.test(b.attributes('title') ?? ''))

describe('the chat button', () => {
  it('is absent when chat is switched off', async () => {
    const w = await mountApp(state(true, false))
    expect(chatButton(w)).toBeUndefined()
    w.unmount()
  })

  it('is present when chat is on', async () => {
    const w = await mountApp(state(true, true))
    expect(chatButton(w)).toBeDefined()
    w.unmount()
  })

  it('closes an open panel when chat is switched off', async () => {
    const w = await mountApp(state(true, true))
    const chat = useChat()
    chat.openPanel()
    await nextTick()
    expect(w.find('.composer').exists()).toBe(true)

    patchSettings.mockResolvedValue(state(true, false))
    await useFeatures().setChatEnabled(false)
    await nextTick()

    expect(chat.open).toBe(false)
    expect(w.find('.composer').exists()).toBe(false)
    w.unmount()
  })
})

describe('after the token gate', () => {
  it('reloads features once the token is accepted', async () => {
    features.mockResolvedValue(state(true, true))
    useWorkspace().authRequired = true
    const w = mount(App, { global: { stubs: { GraphCanvas: true } } })
    await flushPromises()

    w.findComponent(ApiTokenGate).vm.$emit('submit', 'x'.repeat(32))
    await flushPromises()

    expect(features).toHaveBeenCalledTimes(2)
    expect(useFeatures().chatEnabled).toBe(true)
    w.unmount()
  })
})

describe('the dialog', () => {
  function mountDialog(f: Features) {
    const store = useFeatures()
    store.chatAvailable = f.chat.available
    store.chatEnabled = f.chat.enabled
    return mount(SettingsDialog, { attachTo: document.body })
  }

  it('disables the switch when chat is not deployed', () => {
    const w = mountDialog(state(false, false))
    expect(w.find('[role="switch"]').attributes('disabled')).toBeDefined()
    expect(w.text()).toContain(en['settings.chat.notDeployed'])
    w.unmount()
  })

  it('switches chat off through the server', async () => {
    patchSettings.mockResolvedValue(state(true, false))
    const w = mountDialog(state(true, true))
    const sw = w.find('[role="switch"]')
    expect(sw.attributes('aria-checked')).toBe('true')

    await sw.trigger('click')
    await flushPromises()

    expect(patchSettings).toHaveBeenCalledWith({ chatEnabled: false })
    expect(sw.attributes('aria-checked')).toBe('false')
    w.unmount()
  })

  it('says so when the switch could not be saved', async () => {
    patchSettings.mockRejectedValue(new Error('boom'))
    const w = mountDialog(state(true, true))
    await w.find('[role="switch"]').trigger('click')
    await flushPromises()
    expect(w.find('[role="alert"]').text()).toBe(en['settings.chat.failed'])
    w.unmount()
  })

  it('takes focus when it opens', () => {
    const w = mountDialog(state(true, true))
    expect(document.activeElement).toBe(w.find('[role="dialog"]').element)
    w.unmount()
  })

  it('closes on Escape and on the close button', async () => {
    const w = mountDialog(state(true, true))
    await w.find('[role="dialog"]').trigger('keydown', { key: 'Escape' })
    await w.find('.head button').trigger('click')
    expect(w.emitted('close')).toHaveLength(2)
    w.unmount()
  })

  it('keeps Tab inside the dialog', async () => {
    const w = mountDialog(state(true, true))
    const dialog = w.find('[role="dialog"]')
    const sw = w.find('[role="switch"]').element as HTMLElement
    const close = w.find('.head button').element as HTMLElement

    await dialog.trigger('keydown', { key: 'Tab', shiftKey: true })
    expect(document.activeElement).toBe(sw)

    await w.find('[role="switch"]').trigger('keydown', { key: 'Tab' })
    expect(document.activeElement).toBe(close)
    w.unmount()
  })

  it('is translated', async () => {
    setLocale('ja')
    const w = mountDialog(state(false, false))
    await nextTick()
    expect(w.text()).toContain(ja['settings.title'])
    expect(w.text()).toContain(ja['settings.chat.notDeployed'])
    w.unmount()
  })
})

describe('opening from the topbar', () => {
  it('opens the dialog, and Escape closes it without closing chat', async () => {
    const w = await mountApp(state(true, true))
    const chat = useChat()
    chat.openPanel()

    await w.findAll('.topbar button').find((b) => b.attributes('title') === en['settings.open'])!.trigger('click')
    const dialog = w.find('[role="dialog"][aria-labelledby="settings-title"]')
    expect(dialog.exists()).toBe(true)

    await dialog.trigger('keydown', { key: 'Escape' })
    expect(w.find('[aria-labelledby="settings-title"]').exists()).toBe(false)
    expect(chat.open).toBe(true)

    await nextTick()
    const settingsButton = w.findAll('.topbar button').find((b) => b.attributes('title') === en['settings.open'])!
    expect(document.activeElement).toBe(settingsButton.element)
    expect(settingsButton.attributes('aria-haspopup')).toBe('dialog')
    w.unmount()
  })
})
