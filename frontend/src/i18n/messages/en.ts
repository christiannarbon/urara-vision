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

  // The entry screen
  'welcome.title': 'Data model visualiser',
  'welcome.intro':
    'Point it at a directory of table documentation. It parses the markdown, resolves the relationships between tables, and draws the model as a graph you can explore.',
  'welcome.working': 'Working…',
  'welcome.dropzone.title': 'Select your documentation directory',
  // Split around the <code>.md</code> it names, as the gate's sentence is.
  'welcome.dropzone.hint.before': 'Every',
  'welcome.dropzone.hint.after':
    'file beneath it is read in your browser and sent to the parser. Nothing is written back to disk.',
  'welcome.choose': 'Choose folder…',
  'welcome.useFileInput': 'Use file input instead',
  'welcome.noPicker':
    'Your browser does not expose the directory picker, so the file input is used. It behaves the same way.',
  'welcome.recent': 'Previous ingests',
  'welcome.stats.tables.one': '{n} table',
  'welcome.stats.tables.other': '{n} tables',
  'welcome.stats.domains.one': '{n} domain',
  'welcome.stats.domains.other': '{n} domains',
  'welcome.delete.title': 'Delete this snapshot',
  'welcome.delete.label': 'Delete snapshot',

  // Reading a directory
  'picker.scanning': 'Scanning directory…',
  'picker.reading': 'Reading documents…',
  'picker.read.one': 'Read {n} document…',
  'picker.read.other': 'Read {n} documents…',
  'picker.readOf': 'Read {n} of {total} documents…',
  'picker.error.open': 'Could not open the directory picker.',
  'picker.error.read': 'Failed to read the selected directory.',
  'picker.error.noMarkdown': 'That directory contains no markdown files.',
  'picker.error.tooManyFiles':
    'This directory holds more than {max} markdown files. Select a narrower subtree.',
  'picker.error.tooLarge': 'The selected documents exceed the {mb} MB upload limit.',

  // What the workspace is doing, and what went wrong
  'status.parsing.one': 'Parsing {n} document…',
  'status.parsing.other': 'Parsing {n} documents…',
  'status.loading': 'Loading model…',
  'error.unknown': 'Something went wrong.',
  'error.unreachable': 'Cannot reach the backend. Check that the API is running and reachable.',
  'error.tokenRejected': 'This API needs a token, and the one supplied was not accepted.',
  'error.requestFailed': 'Request failed with status {status}.',

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
