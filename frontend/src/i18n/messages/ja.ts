/**
 * Japanese.
 *
 * Typed as `Messages`, so leaving a key untranslated is a compile error rather
 * than a string that silently reverts to English in front of a reader.
 *
 * Two conventions the rest of this file follows:
 *
 *   Domain vocabulary stays in the reader's language, not in transliteration.
 *   A dimension table is a ディメンションテーブル because that is what the
 *   documents these models come from call one; writing ディメンション for the
 *   role and テーブル for the noun everywhere else would read as two
 *   vocabularies.
 *
 *   Counted strings still carry a `.one` and an `.other`, and they are the
 *   same sentence. Japanese does not mark plural, so `plural` below answers
 *   `other` for every number and the `.one` entries are never read -- they
 *   exist because the key set is shared, and the duplication is cheaper than a
 *   catalogue type that has to describe which locales count.
 */
import type { Messages } from './en'

export const messages: Messages = {
  'locale.en': 'English',
  'locale.ja': '日本語',
}

/** Japanese has no plural forms: one table and forty tables read alike. */
export function plural(_n: number): 'one' | 'other' {
  return 'other'
}
