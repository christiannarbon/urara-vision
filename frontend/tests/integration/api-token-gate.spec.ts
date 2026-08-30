/**
 * The token gate, mounted, in both languages.
 *
 * The gate is the first thing a reader of a protected instance sees, and it is
 * the whole page rather than a banner -- there is nothing behind it to fall
 * back on. It is therefore the surface worth asserting the localisation on:
 * that the strings come from the catalogue rather than the template, and that
 * changing the language changes them without a remount.
 */

import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'

import ApiTokenGate from '../../src/components/ApiTokenGate.vue'
import { messages as en } from '../../src/i18n/messages/en'
import { messages as ja } from '../../src/i18n/messages/ja'
import { setLocale } from '../../src/i18n'

afterEach(() => setLocale('en'))

describe('the gate', () => {
  it('reads from the catalogue rather than the template', () => {
    const w = mount(ApiTokenGate, { props: { busy: false } })
    expect(w.text()).toContain(en['gate.title'])
    expect(w.text()).toContain(en['gate.storage'])
    expect(w.find('input').attributes('placeholder')).toBe(en['gate.placeholder'])
  })

  it('names the environment variable outside the translated sentence', () => {
    // API_TOKEN is the name of a real variable, so it stays put in every
    // language; the sentence is split around it rather than interpolated.
    const w = mount(ApiTokenGate, { props: { busy: false } })
    expect(w.find('code').text()).toBe('API_TOKEN')
  })

  it('follows a language change without remounting', async () => {
    const w = mount(ApiTokenGate, { props: { busy: false } })
    setLocale('ja')
    await nextTick()
    expect(w.text()).toContain(ja['gate.title'])
    expect(w.text()).not.toContain(en['gate.title'])
    expect(w.find('code').text()).toBe('API_TOKEN')
  })

  it('labels the submit button for the state it is in', async () => {
    const w = mount(ApiTokenGate, { props: { busy: false } })
    expect(w.find('button[type="submit"]').text()).toBe(en['gate.continue'])
    await w.setProps({ busy: true })
    expect(w.find('button[type="submit"]').text()).toBe(en['gate.checking'])
  })

  it('shows the rejection notice in the active language', async () => {
    setLocale('ja')
    const w = mount(ApiTokenGate, { props: { busy: false } })
    ;(w.vm as unknown as { markRejected: () => void }).markRejected()
    await nextTick()
    expect(w.find('[role="alert"]').text()).toBe(ja['gate.rejected'])
  })
})
