/**
 * Reading a document's own languages.
 *
 * The rules are the backend's, implemented twice on purpose, so the cases the
 * format was specified with are pinned on this side too -- in both directions,
 * since a project that documents in Japanese reads them the other way round.
 * `backend/tests/unit/i18ntext` asserts the same table.
 */

import { afterEach, describe, expect, it } from 'vitest'

import { documentText, setDocumentLanguages, splitter } from '../../src/i18n/content'
import { setLocale } from '../../src/i18n'
import type { ProjectMeta } from '../../src/api/types'

const EN_FIRST = { primary: 'EN', supported: ['EN', 'JP'] }
const JP_FIRST = { primary: 'JP', supported: ['EN', 'JP'] }

/** A manifest as a snapshot carries it. */
function manifest(primary: string, supported: string[]): ProjectMeta {
  return {
    project: { name: 'p', version: '0.1.0', description: '' },
    internationalization: { primary, supported, type: 'inline' },
  }
}

afterEach(() => {
  setLocale('en')
  setDocumentLanguages(null)
})

describe('a project that documents in English', () => {
  const s = splitter(EN_FIRST)

  it.each([
    ['This is a column [JP] これはコラムです。', 'This is a column', 'これはコラムです。'],
    // Untranslated: everyone reads the one language it has.
    ['This is a column', 'This is a column', 'This is a column'],
    // Untagged text is primary text whatever script it is in. Nothing guesses
    // a language from the characters.
    ['これはコラムです。', 'これはコラムです。', 'これはコラムです。'],
  ])('reads %j as %j in English and %j in Japanese', (text, en, jp) => {
    expect(s.select(text, ['EN'])).toBe(en)
    expect(s.select(text, ['JA', 'JP'])).toBe(jp)
  })
})

describe('a project that documents in Japanese', () => {
  const s = splitter(JP_FIRST)

  it.each([
    ['これはコラムです。 [EN] This is a column', 'This is a column', 'これはコラムです。'],
    ['This is a column', 'This is a column', 'This is a column'],
    ['これはコラムです。', 'これはコラムです。', 'これはコラムです。'],
  ])('reads %j as %j in English and %j in Japanese', (text, en, jp) => {
    expect(s.select(text, ['EN'])).toBe(en)
    expect(s.select(text, ['JA', 'JP'])).toBe(jp)
  })
})

describe('what counts as a tag', () => {
  it('is only a language the project declares', () => {
    const s = splitter(EN_FIRST)
    const text = 'Pending [TBD] and cited [1] but translated [JP] 翻訳済み'
    expect(s.select(text, ['EN'])).toBe('Pending [TBD] and cited [1] but translated')
    expect(s.select(text, ['JP'])).toBe('翻訳済み')
  })

  it('is matched whatever case it is written in', () => {
    const s = splitter({ primary: 'en', supported: ['en', 'jp'] })
    expect(s.select('English [jp] 日本語', ['JP'])).toBe('日本語')
  })

  it('is nothing at all for a snapshot that declares no languages', () => {
    const text = 'This is a column [JP] これはコラムです。'
    expect(splitter({ primary: '', supported: [] }).select(text, ['EN'])).toBe(text)
  })
})

describe('falling back', () => {
  const s = splitter(EN_FIRST)

  it('gives the primary language when the field has nothing in the one asked for', () => {
    expect(s.select('English [JP] 日本語', ['FR'])).toBe('English')
  })

  it('treats a tag with nothing after it as absent', () => {
    expect(s.select('English [JP]', ['JP'])).toBe('English')
  })

  it('gives what the field does carry when the primary language is missing', () => {
    expect(s.select('[JP] 日本語だけ', ['EN'])).toBe('日本語だけ')
  })

  it('returns blank only for a field that was blank', () => {
    expect(s.select('   ', ['EN'])).toBe('')
  })
})

describe('the shape of a field', () => {
  it('keeps a paragraph, trimming only its ends', () => {
    const s = splitter(EN_FIRST)
    const text = 'First line.\nSecond line.\n\n[JP] 一行目。\n二行目。\n'
    expect(s.select(text, ['EN'])).toBe('First line.\nSecond line.')
    expect(s.select(text, ['JP'])).toBe('一行目。\n二行目。')
  })

  it('joins a language tagged twice rather than dropping half of it', () => {
    const s = splitter(EN_FIRST)
    expect(s.select('English. [JP] 日本語。 [JP] 続き。', ['JP'])).toBe('日本語。 続き。')
  })
})

describe('the language on screen', () => {
  it('picks the document language the interface language answers to', () => {
    setDocumentLanguages(manifest('EN', ['EN', 'JP']))
    const text = 'This is a column [JP] これはコラムです。'

    expect(documentText(text)).toBe('This is a column')
    setLocale('ja')
    // "JP" is not a language code, but it is what people write for Japanese,
    // so the Japanese interface answers to it.
    expect(documentText(text)).toBe('これはコラムです。')
  })

  it('answers to JA as readily as JP', () => {
    setDocumentLanguages(manifest('EN', ['EN', 'JA']))
    setLocale('ja')
    expect(documentText('English [JA] 日本語')).toBe('日本語')
  })

  it('reads a snapshot with no manifest exactly as it did before', () => {
    setDocumentLanguages(undefined)
    const text = 'This is a column [JP] これはコラムです。'
    expect(documentText(text)).toBe(text)
  })

  it('has nothing to say about an empty field', () => {
    setDocumentLanguages(manifest('EN', ['EN', 'JP']))
    expect(documentText('')).toBe('')
    expect(documentText(null)).toBe('')
    expect(documentText(undefined)).toBe('')
  })
})
