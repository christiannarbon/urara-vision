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

/** Human-readable explanations, so a reader does not have to infer what a
 *  machine-generated code means. */
export const CODE_LABELS: Record<string, { title: string; blurb: string }> = {
  unresolved_reference: {
    title: 'Unresolved references',
    blurb:
      'A relationship points at a table with no document. Either the document is missing or the name is wrong.',
  },
  cross_domain_reference: {
    title: 'Cross-domain references',
    blurb:
      'A table joins a dimension that has no document in its own domain, so it was bound to a conformed instance elsewhere.',
  },
  conformed_drift: {
    title: 'Conformed drift',
    blurb:
      'The same table name is documented differently in different domains. Conformed dimensions should agree.',
  },
  unmatched_join_key: {
    title: 'Unmatched join keys',
    blurb: 'A join key names columns that neither table documents.',
  },
  undocumented_lineage: {
    title: 'Undocumented lineage',
    blurb:
      'Columns whose Source Table cell holds prose ("not available", "N/A") rather than a model name. Those columns are excluded from the lineage graph.',
  },
  narrative_reference: {
    title: 'Prose references',
    blurb: 'A relationship cell holds prose rather than a table name, so it could not be drawn.',
  },
  isolated_fact: {
    title: 'Isolated facts',
    blurb: 'A fact table declares no resolvable relationship to any dimension.',
  },
  missing_domain_index: {
    title: 'Missing domain index',
    blurb: 'A directory has table documents but no index document.',
  },
  empty_domain: {
    title: 'Empty domains',
    blurb: 'A domain index exists but its directory has no table documents.',
  },
  no_columns: {
    title: 'Tables without columns',
    blurb: 'A table document declares no columns.',
  },
  name_filename_mismatch: {
    title: 'Name / filename mismatches',
    blurb: 'The declared table name differs from its file name.',
  },
  unrecognised_document: {
    title: 'Unrecognised documents',
    blurb:
      'A markdown file matched neither a table nor a domain index layout, so it was skipped entirely and nothing it describes reached the model.',
  },
  empty_document: {
    title: 'Empty documents',
    blurb: 'A markdown file had no content, so it was skipped entirely.',
  },
}

export function codeLabel(code: string) {
  return CODE_LABELS[code] ?? { title: code.replace(/_/g, ' '), blurb: '' }
}
