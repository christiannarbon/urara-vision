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

  // The role vocabulary. Mirrors graph/roles.ts, which mirrors the backend.
  // A role the documents brought with them keeps its own word and is not here.
  'role.fact': 'Fact',
  'role.factless': 'Factless fact',
  'role.dimension': 'Dimension',
  'role.outrigger': 'Outrigger',
  'role.bridge': 'Bridge',
  'role.junk': 'Junk dimension',
  'role.degenerate': 'Degenerate dimension',
  'role.hub': 'Hub',
  'role.link': 'Link',
  'role.satellite': 'Satellite',
  'role.pit': 'Point-in-time',
  'role.entity': 'Entity',
  'role.associative': 'Associative',
  'role.lookup': 'Lookup',
  'role.reference': 'Reference',
  'role.unknown': 'Unknown',

  // The three layouts
  'layout.force': 'Force',
  'layout.force.hint': 'Free arrangement with no reading direction. Groups tables by domain.',
  'layout.layered': 'Layered',
  'layout.layered.hint':
    'Ranks tables along their joins — normalisation depth, or Data Vault tiers. Cannot group by domain.',
  'layout.radial': 'Radial',
  'layout.radial.hint': 'Busiest tables at the centre, working outwards. Cannot group by domain.',

  // Left rail
  'rail.label': 'Filters and tables',
  'rail.stats.tables': 'tables',
  'rail.stats.domains': 'domains',
  'rail.stats.columns': 'columns',
  'rail.stats.sources': 'sources',
  'rail.view': 'View',
  'rail.view.label': 'View mode',
  'rail.view.whole': 'Whole model',
  'rail.view.focused': 'Focused',
  'rail.view.focused.title': 'Show only the selected table and its neighbours',
  'rail.view.focused.disabled': 'Select a table first',
  'rail.layout.label': 'Layout',
  'rail.depth': 'Depth',
  'rail.showSources': 'Show upstream source models',
  'rail.crossDomainOnly': 'Cross-domain joins only',
  'rail.filters': 'Filters',
  'rail.clear': 'Clear',
  'rail.role.fromDocuments': 'Role read from the documents: {role}',
  'rail.tables': 'Tables ({n})',
  'rail.tables.filter': 'Filter tables…',
  'rail.tables.filter.label': 'Filter tables',
  'rail.conformed': 'Conformed: also defined in other domains',
  'rail.noMatches': 'No tables match.',

  // The canvas
  'canvas.label': 'Table relationship graph',
  'canvas.building': 'Building graph…',
  'canvas.layingOut': 'Laying out…',
  'canvas.empty.title': 'Nothing to draw',
  'canvas.empty.body': 'No tables match the current filters.',
  'canvas.controls': 'Graph controls',
  'canvas.zoomIn': 'Zoom in',
  'canvas.zoomOut': 'Zoom out',
  'canvas.fit': 'Fit to view',
  'canvas.relayout': 'Re-run layout',
  'canvas.group': 'Group by domain',
  'canvas.ungroup': 'Ungroup domains',
  'canvas.group.unavailable': 'Grouping is only available in the {layout} layout',
  'canvas.legend': 'Legend',
  'canvas.legend.source': 'Source model',
  'canvas.legend.crossDomain': 'Cross-domain join',
  'canvas.legend.cluster': 'Domain cluster',

  // Detail pane
  'detail.label': 'Table detail',
  'detail.loading': 'Loading table…',
  'detail.sourceOnly':
    'This is an upstream source model referenced by column lineage. It has no table document of its own in this snapshot.',
  'detail.empty': 'Select a table in the graph to see its description, columns and lineage.',
  'detail.empty.hint': 'Double-click a node to centre the graph on it.',
  'detail.conformed': 'conformed',
  'detail.close': 'Close detail',
  'detail.focusHere': 'Focus graph here',
  'detail.tab.overview': 'overview',
  'detail.tab.columns': 'columns',
  'detail.tab.joins': 'joins',
  'detail.tab.lineage': 'lineage',
  'detail.grain': 'Grain',
  'detail.type': 'Type',
  'detail.domain': 'Domain',
  'detail.updated': 'Updated',
  'detail.layer': 'Layer',
  'detail.source': 'Source',
  'detail.keys': 'Keys',
  'detail.alsoDefinedIn': 'Also defined in',
  'detail.alsoDefinedIn.note':
    'This table name appears in other domains. Definitions may differ — check the diagnostics panel for drift.',
  'detail.notes': 'Notes & caveats',
  'detail.columns.filter': 'Filter columns…',
  'detail.columns.filter.label': 'Filter columns',
  'detail.columns.noMatches': 'No columns match that filter.',
  'detail.columns.from': 'from',
  'detail.columns.derived': 'derived',
  'detail.joins.declared': 'Declared by this table',
  'detail.joins.none': 'This table declares no relationships.',
  'detail.joins.crossDomain': 'cross-domain',
  'detail.joins.boundTo': 'Bound to {target}; also defined in {alternatives}',
  'detail.joins.unresolved': 'Unresolved references',
  'detail.joins.noDocument': 'no document',
  'detail.joins.prose': 'prose',
  'detail.joins.referencedBy': 'Referenced by',
  'detail.lineage.upstream': 'Upstream source models',
  'detail.lineage.none': 'No column-level lineage is documented for this table.',
  'detail.lineage.columns.one': '{n} col',
  'detail.lineage.columns.other': '{n} cols',
  'detail.lineage.more': '+{n} more',
  'detail.siblings': 'Shares sources with',
  'detail.siblings.note':
    'These tables read from at least one of the same upstream models, so an upstream change is likely to affect them too.',
  'detail.siblings.shared.one': '{n} shared source',
  'detail.siblings.shared.other': '{n} shared sources',

  // Search overlay
  'search.label': 'Search tables',
  'search.placeholder': 'Search tables, columns, descriptions…',
  'search.input.label': 'Search',
  'search.hint': 'Type to search across table names, grains, descriptions and column names.',
  'search.searching': 'Searching…',
  'search.noMatches': 'No matches.',
  'search.key.navigate': 'navigate',
  'search.key.open': 'open',
  'search.key.close': 'close',

  // Diagnostics panel
  'diagnostics.label': 'Documentation diagnostics',
  'diagnostics.title': 'Diagnostics',
  'diagnostics.subtitle': 'Problems found while parsing and resolving the documentation.',
  'diagnostics.close': 'Close diagnostics',
  'diagnostics.severity.all': 'all',
  'diagnostics.severity.error': 'error',
  'diagnostics.severity.warning': 'warning',
  'diagnostics.severity.info': 'info',
  'diagnostics.empty':
    'Nothing to report — every document parsed and every reference resolved cleanly.',
  'diagnostics.unparsed': 'Documents that could not be parsed',
  'diagnostics.unparsed.note':
    'These files were skipped entirely, so nothing they describe reached the model.',
  'diagnostics.findings': 'Model findings',
  'diagnostics.findings.note':
    'Everything below parsed cleanly; the resolver noticed something worth checking.',

  // What each diagnostic code means. The backend can add a code without a
  // frontend release, and one with no entry here falls back to its own slug --
  // so this list is the codes we can explain, not the codes that exist.
  'diagnostic.unresolved_reference.title': 'Unresolved references',
  'diagnostic.unresolved_reference.blurb':
    'A relationship points at a table with no document. Either the document is missing or the name is wrong.',
  'diagnostic.cross_domain_reference.title': 'Cross-domain references',
  'diagnostic.cross_domain_reference.blurb':
    'A table joins a dimension that has no document in its own domain, so it was bound to a conformed instance elsewhere.',
  'diagnostic.conformed_drift.title': 'Conformed drift',
  'diagnostic.conformed_drift.blurb':
    'The same table name is documented differently in different domains. Conformed dimensions should agree.',
  'diagnostic.unmatched_join_key.title': 'Unmatched join keys',
  'diagnostic.unmatched_join_key.blurb': 'A join key names columns that neither table documents.',
  'diagnostic.undocumented_lineage.title': 'Undocumented lineage',
  'diagnostic.undocumented_lineage.blurb':
    'Columns whose Source Table cell holds prose ("not available", "N/A") rather than a model name. Those columns are excluded from the lineage graph.',
  'diagnostic.narrative_reference.title': 'Prose references',
  'diagnostic.narrative_reference.blurb':
    'A relationship cell holds prose rather than a table name, so it could not be drawn.',
  'diagnostic.isolated_fact.title': 'Isolated facts',
  'diagnostic.isolated_fact.blurb':
    'A fact table declares no resolvable relationship to any dimension.',
  'diagnostic.missing_domain_index.title': 'Missing domain index',
  'diagnostic.missing_domain_index.blurb':
    'A directory has table documents but no index document.',
  'diagnostic.empty_domain.title': 'Empty domains',
  'diagnostic.empty_domain.blurb': 'A domain index exists but its directory has no table documents.',
  'diagnostic.no_columns.title': 'Tables without columns',
  'diagnostic.no_columns.blurb': 'A table document declares no columns.',
  'diagnostic.name_filename_mismatch.title': 'Name / filename mismatches',
  'diagnostic.name_filename_mismatch.blurb':
    'The declared table name differs from its file name.',
  'diagnostic.unrecognised_document.title': 'Unrecognised documents',
  'diagnostic.unrecognised_document.blurb':
    'A markdown file matched neither a table nor a domain index layout, so it was skipped entirely and nothing it describes reached the model.',
  'diagnostic.empty_document.title': 'Empty documents',
  'diagnostic.empty_document.blurb': 'A markdown file had no content, so it was skipped entirely.',

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
