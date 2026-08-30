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

  'app.title': 'Urara Vision — data model explorer',

  // Topbar
  'topbar.home': 'Back to your ingests',
  'topbar.search': 'Search',
  'topbar.search.title': 'Search (⌘K)',
  'topbar.diagnostics': 'Diagnostics',
  'topbar.diagnostics.title': 'Diagnostics',
  'topbar.diagnostics.titleAttention': 'Diagnostics — something may need a look',
  'topbar.diagnostics.flag': 'Diagnostics need attention',
  'topbar.newIngest': 'New ingest',

  // The banners under the topbar
  'banner.dismiss': 'Dismiss',
  'banner.review': 'Review',
  'banner.parseFailures.one': '1 document could not be parsed and was skipped.',
  'banner.parseFailures.other': '{n} documents could not be parsed and were skipped.',
  // Spelled out separately so the sentence agrees in number with the one above.
  'banner.parseFailuresDetail.one': 'Anything it documents is missing from this model.',
  'banner.parseFailuresDetail.other': 'Anything they document is missing from this model.',

  // Theme picker
  'theme.label': 'Theme',
  'theme.current': 'Theme: {name}',
  'theme.paintings': 'Paintings',

  // API token gate
  'gate.title': 'This instance needs a token',
  // Split around the <code>API_TOKEN</code> the sentence names, rather than
  // interpolated as markup. The break falls before the token in both
  // languages, so neither reads as a translation of the other's word order.
  'gate.intro.before':
    'The API is protected by a shared token. Ask whoever runs this deployment for it — it is the',
  'gate.intro.after': 'the backend was started with.',
  'gate.field': 'API token',
  'gate.placeholder': 'Paste the token',
  'gate.rejected': 'That token was not accepted. Check it for a missing or trailing character.',
  'gate.checking': 'Checking…',
  'gate.continue': 'Continue',
  'gate.storage': "It is kept in this browser's local storage so you are not asked on every visit.",
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
