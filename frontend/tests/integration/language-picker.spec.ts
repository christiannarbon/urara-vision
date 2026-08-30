/**
 * The language picker, mounted.
 *
 * Everything about this control is behaviour a template cannot show: that it
 * lists every language the app has, names each in its own script, marks the
 * active one, and changes the language of the app around it when a reader
 * picks one.
 */

import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'

import LanguagePicker from '../../src/components/LanguagePicker.vue'
import { LOCALES, setLocale, translate, useI18n } from '../../src/i18n'
import { messages as en } from '../../src/i18n/messages/en'
import { messages as ja } from '../../src/i18n/messages/ja'

afterEach(() => setLocale('en'))

async function openMenu() {
  const w = mount(LanguagePicker)
  await w.find('button').trigger('click')
  return w
}

describe('the trigger', () => {
  it('shows the language currently in use', async () => {
    expect(mount(LanguagePicker).find('.name').text()).toBe(en['locale.en'])
    setLocale('ja')
    await nextTick()
    expect(mount(LanguagePicker).find('.name').text()).toBe(ja['locale.ja'])
  })

  it('is closed until it is asked to open', () => {
    const w = mount(LanguagePicker)
    expect(w.find('[role="listbox"]').exists()).toBe(false)
    expect(w.find('button').attributes('aria-expanded')).toBe('false')
  })
})

describe('the menu', () => {
  it('offers every language the app has', async () => {
    const w = await openMenu()
    expect(w.findAll('[role="option"]')).toHaveLength(LOCALES.length)
  })

  it('names each language in its own script, not the reader\'s', async () => {
    // "Japanese" is no use to somebody who cannot read the word "Japanese".
    // Read off the name element: the selected option also carries a tick.
    const w = await openMenu()
    expect(w.findAll('.opt-name').map((o) => o.text())).toEqual([
      en['locale.en'],
      ja['locale.ja'],
    ])
  })

  it('marks each option with the language it is written in', async () => {
    // Browsers pick font fallback and line-breaking from this.
    const w = await openMenu()
    expect(w.findAll('[role="option"]').map((o) => o.attributes('lang'))).toEqual([...LOCALES])
  })

  it('marks the active language as selected', async () => {
    setLocale('ja')
    const w = await openMenu()
    const options = w.findAll('[role="option"]')
    expect(options[0].attributes('aria-selected')).toBe('false')
    expect(options[1].attributes('aria-selected')).toBe('true')
  })
})

describe('choosing a language', () => {
  it('changes the language of the app', async () => {
    const w = await openMenu()
    await w.findAll('[role="option"]')[1].trigger('click')

    expect(useI18n().locale.value).toBe('ja')
    expect(translate('topbar.diagnostics')).toBe(ja['topbar.diagnostics'])
  })

  it('closes the menu behind it', async () => {
    const w = await openMenu()
    await w.findAll('[role="option"]')[1].trigger('click')
    expect(w.find('[role="listbox"]').exists()).toBe(false)
  })

  it('persists the choice for the next visit', async () => {
    const w = await openMenu()
    await w.findAll('[role="option"]')[1].trigger('click')
    await nextTick()
    expect(localStorage.getItem('relviz.locale')).toBe('ja')
  })

  it('stamps the choice onto the document', async () => {
    const w = await openMenu()
    await w.findAll('[role="option"]')[1].trigger('click')
    await nextTick()
    expect(document.documentElement.getAttribute('lang')).toBe('ja')
  })
})

describe('dismissal', () => {
  it('closes on Escape', async () => {
    const w = await openMenu()
    await w.find('.picker').trigger('keydown', { key: 'Escape' })
    expect(w.find('[role="listbox"]').exists()).toBe(false)
  })

  it('does not let that Escape reach the shell behind it', async () => {
    // App.vue clears the selection on Escape; closing a menu should not.
    const w = await openMenu()
    let bubbled = false
    const onKey = () => {
      bubbled = true
    }
    document.addEventListener('keydown', onKey)
    const event = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })
    w.find('.picker').element.dispatchEvent(event)
    document.removeEventListener('keydown', onKey)
    expect(bubbled).toBe(false)
  })
})
