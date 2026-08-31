/** Locale choice, catalogue lookup and placeholder substitution. */
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { messages as en, plural as enPlural } from '../../src/i18n/messages/en'
import { messages as ja, plural as jaPlural } from '../../src/i18n/messages/ja'

/**
 * The module decides its locale once, at import. Detection is therefore tested
 * against a fresh copy each time rather than by reaching into the live one.
 */
async function freshI18n() {
  vi.resetModules()
  return import('../../src/i18n')
}

/** Replaces the read-only `navigator.languages` for one assertion. */
function withLanguages(langs: string[]) {
  vi.spyOn(navigator, 'languages', 'get').mockReturnValue(langs)
  vi.spyOn(navigator, 'language', 'get').mockReturnValue(langs[0] ?? 'en')
}

beforeEach(() => {
  localStorage.clear()
  document.documentElement.removeAttribute('lang')
})

describe('the catalogues', () => {
  it('translate every key the source catalogue declares', () => {
    expect(Object.keys(ja).sort()).toEqual(Object.keys(en).sort())
  })

  it('leave no entry empty', () => {
    const blank = Object.entries(ja).filter(([, v]) => !v.trim())
    expect(blank).toEqual([])
  })

  it('name each language in that language', () => {
    // A picker that says "Japanese" is no use to a reader who cannot read the
    // word "Japanese", so these two are the same in every catalogue.
    expect(en['locale.ja']).toBe(ja['locale.ja'])
    expect(en['locale.en']).toBe(ja['locale.en'])
  })
})

describe('plural selection', () => {
  it('is two-form in English', () => {
    expect(enPlural(1)).toBe('one')
    expect(enPlural(0)).toBe('other')
    expect(enPlural(2)).toBe('other')
  })

  it('is one-form in Japanese', () => {
    expect(jaPlural(1)).toBe('other')
    expect(jaPlural(42)).toBe('other')
  })
})

describe('detectLocale', () => {
  it('honours a stored choice over the browser', async () => {
    localStorage.setItem('relviz.locale', 'en')
    withLanguages(['ja-JP', 'ja'])
    const { locale } = (await freshI18n()).useI18n()
    expect(locale.value).toBe('en')
  })

  it('matches the browser on the primary subtag', async () => {
    withLanguages(['ja-JP'])
    const { locale } = (await freshI18n()).useI18n()
    expect(locale.value).toBe('ja')
  })

  it('walks past languages it does not have', async () => {
    withLanguages(['fr-FR', 'de', 'ja'])
    const { locale } = (await freshI18n()).useI18n()
    expect(locale.value).toBe('ja')
  })

  it('falls back to English when nothing matches', async () => {
    withLanguages(['fr-FR', 'de'])
    const { locale } = (await freshI18n()).useI18n()
    expect(locale.value).toBe('en')
  })

  it('ignores a stored value that is not a language it has', async () => {
    localStorage.setItem('relviz.locale', 'kl')
    withLanguages(['fr'])
    const { locale } = (await freshI18n()).useI18n()
    expect(locale.value).toBe('en')
  })
})

describe('setLocale', () => {
  it('persists the choice and marks the document', async () => {
    withLanguages(['en'])
    const mod = await freshI18n()
    const { setLocale, locale } = mod.useI18n()
    setLocale('ja')
    // The watcher runs on the next tick, so let the scheduler drain.
    await Promise.resolve()
    expect(locale.value).toBe('ja')
    expect(localStorage.getItem('relviz.locale')).toBe('ja')
    expect(document.documentElement.getAttribute('lang')).toBe('ja')
  })

  it('survives storage it is not allowed to write', async () => {
    withLanguages(['en'])
    vi.spyOn(globalThis.localStorage, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    const { setLocale, locale } = (await freshI18n()).useI18n()
    expect(() => setLocale('ja')).not.toThrow()
    await Promise.resolve()
    expect(locale.value).toBe('ja')
  })
})

describe('translate', () => {
  it('answers in the active language', async () => {
    withLanguages(['en'])
    const { t, setLocale } = (await freshI18n()).useI18n()
    expect(t('locale.ja')).toBe('日本語')
    setLocale('ja')
    expect(t('locale.ja')).toBe('日本語')
  })

  it('leaves a message with no placeholders alone', async () => {
    const { t } = (await freshI18n()).useI18n()
    expect(t('locale.en', { n: 3 })).toBe('English')
  })
})

describe('interpolate', () => {
  it('fills every placeholder it has a param for', async () => {
    const { interpolate } = await freshI18n()
    expect(interpolate('{n} of {total} tables', { n: 3, total: 40 })).toBe('3 of 40 tables')
  })

  it('fills a placeholder that appears more than once', async () => {
    const { interpolate } = await freshI18n()
    expect(interpolate('{name} joins {name}', { name: 'orders' })).toBe('orders joins orders')
  })

  it('leaves a placeholder it has no param for standing', async () => {
    // Visible rather than blank: a stray {count} reads as the bug it is.
    const { interpolate } = await freshI18n()
    expect(interpolate('{n} of {total}', { n: 3 })).toBe('3 of {total}')
  })
})
