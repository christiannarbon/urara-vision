/**
 * Shared knowledge about ingest diagnostic codes.
 *
 * Diagnostics fall into two groups that deserve very different treatment:
 *
 *   - Parse failures. The document was dropped, so whatever it documented is
 *     simply absent from the model. Nothing downstream can be trusted to be
 *     complete, so the reader is told immediately.
 *   - Model findings. Every document parsed; the resolver just noticed
 *     something that may or may not be a real problem — an unresolved
 *     reference, a conformed dimension that has drifted, a join key naming
 *     columns nobody declares. These are worth a look, not an interruption.
 */

import type { Diagnostic } from './api/types'
import { translate as t } from './i18n'
import type { MessageKey } from './i18n'

/** Codes meaning a document could not be turned into model data at all. */
export const PARSE_FAILURE_CODES = new Set(['empty_document', 'unrecognised_document'])

export function isParseFailure(code: string): boolean {
  return PARSE_FAILURE_CODES.has(code)
}

export function splitDiagnostics(diagnostics: Diagnostic[]) {
  const unparsed: Diagnostic[] = []
  const findings: Diagnostic[] = []
  for (const d of diagnostics) {
    if (isParseFailure(d.code)) unparsed.push(d)
    else findings.push(d)
  }
  return { unparsed, findings }
}

/**
 * The codes this frontend has an explanation for.
 *
 * Not the codes that exist: the backend can add one without a frontend
 * release, and `codeLabel` falls back to the slug for anything not listed
 * here. The two `diagnostic.<code>.title` / `.blurb` entries every code below
 * needs are checked against the catalogue at compile time, in `labelKeys`.
 */
export const DIAGNOSTIC_CODES = [
  'unresolved_reference',
  'cross_domain_reference',
  'conformed_drift',
  'unmatched_join_key',
  'undocumented_lineage',
  'narrative_reference',
  'isolated_fact',
  'missing_domain_index',
  'empty_domain',
  'no_columns',
  'name_filename_mismatch',
  'unrecognised_document',
  'empty_document',
] as const

export type DiagnosticCode = (typeof DIAGNOSTIC_CODES)[number]

const EXPLAINED = new Set<string>(DIAGNOSTIC_CODES)

/** The `satisfies` is the check: a code with no catalogue entry fails here. */
function labelKeys(code: DiagnosticCode) {
  return {
    title: `diagnostic.${code}.title` satisfies MessageKey,
    blurb: `diagnostic.${code}.blurb` satisfies MessageKey,
  }
}

/**
 * A readable title and explanation for a diagnostic code, so a reader does not
 * have to infer what a machine-generated slug means.
 *
 * A code with no entry keeps its own slug, spaced out. That is deliberately
 * not translated: the alternative is showing nothing for a finding the backend
 * knows about and this build does not.
 */
export function codeLabel(code: string): { title: string; blurb: string } {
  if (!EXPLAINED.has(code)) return { title: code.replace(/_/g, ' '), blurb: '' }
  const keys = labelKeys(code as DiagnosticCode)
  return { title: t(keys.title), blurb: t(keys.blurb) }
}
