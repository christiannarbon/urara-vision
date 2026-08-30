/**
 * The source catalogue. Every string the interface says starts here.
 *
 * Keys are flat and dotted rather than nested, which buys two things a nested
 * object cannot: `t('topbar.search')` is checked against the real key set at
 * compile time, and a translation is a plain `Record<MessageKey, string>` that
 * fails to compile the moment it is missing an entry. Nesting would make both
 * of those a runtime concern.
 *
 * The dot prefix names the surface the string appears on -- `topbar.`, not
 * `App.` -- so a string moving between components does not move between keys.
 *
 * Placeholders are `{name}`, filled by the params passed to `t`.
 *
 * Counted strings come in `.one` / `.other` pairs and are read with `tn`,
 * which asks the active locale which variant a number takes. English needs
 * both; Japanese does not mark plural at all and answers `other` every time.
 */
export const messages = {
  'locale.en': 'English',
  'locale.ja': '日本語',
} as const

export type MessageKey = keyof typeof messages

/**
 * The shape every translation has to fill. A `Record` rather than
 * `typeof messages`: the English entries are literal types, and a translation
 * is emphatically not the same literal.
 */
export type Messages = Record<MessageKey, string>

/**
 * Which variant of a counted string a number takes.
 *
 * English is the two-form case the pair spellings assume, so this is the
 * identity the catalogue was written against.
 */
export function plural(n: number): 'one' | 'other' {
  return n === 1 ? 'one' : 'other'
}
