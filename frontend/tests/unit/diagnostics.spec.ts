/**
 * Diagnostic classification.
 *
 * The split between "a document was dropped" and "the resolver noticed
 * something" decides whether the reader is interrupted, so it is worth pinning
 * down. The code list must also stay in step with the backend: a code the
 * frontend does not know about still has to render.
 */

import { afterEach, describe, expect, it } from 'vitest'

import {
  DIAGNOSTIC_CODES,
  PARSE_FAILURE_CODES,
  codeLabel,
  isParseFailure,
  splitDiagnostics,
} from '../../src/diagnostics'
import { setLocale } from '../../src/i18n'
import { messages as ja } from '../../src/i18n/messages/ja'
import type { Diagnostic } from '../../src/api/types'

function diag(code: string, severity: Diagnostic['severity'] = 'warning'): Diagnostic {
  return { severity, code, message: code }
}

describe('isParseFailure', () => {
  it('is true only for the codes meaning the document was dropped', () => {
    expect(isParseFailure('empty_document')).toBe(true)
    expect(isParseFailure('unrecognised_document')).toBe(true)
  })

  it('is false for findings about documents that parsed', () => {
    for (const code of [
      'unresolved_reference',
      'conformed_drift',
      'unmatched_join_key',
      'isolated_fact',
      'undocumented_lineage',
    ]) {
      expect(isParseFailure(code)).toBe(false)
    }
  })

  it('is false for an unknown code, so a new finding is not an interruption', () => {
    expect(isParseFailure('some_new_backend_code')).toBe(false)
  })
})

describe('splitDiagnostics', () => {
  it('separates dropped documents from model findings', () => {
    const { unparsed, findings } = splitDiagnostics([
      diag('empty_document'),
      diag('unresolved_reference', 'error'),
      diag('unrecognised_document', 'info'),
      diag('conformed_drift'),
    ])
    expect(unparsed.map((d) => d.code)).toEqual(['empty_document', 'unrecognised_document'])
    expect(findings.map((d) => d.code)).toEqual(['unresolved_reference', 'conformed_drift'])
  })

  it('preserves order within each bucket', () => {
    const { findings } = splitDiagnostics([diag('a'), diag('b'), diag('c')])
    expect(findings.map((d) => d.code)).toEqual(['a', 'b', 'c'])
  })

  it('returns two empty buckets for no diagnostics', () => {
    expect(splitDiagnostics([])).toEqual({ unparsed: [], findings: [] })
  })
})

describe('codeLabel', () => {
  afterEach(() => setLocale('en'))

  it('gives a title and an explanation for every known code', () => {
    for (const code of DIAGNOSTIC_CODES) {
      const { title, blurb } = codeLabel(code)
      expect(title).not.toBe('')
      expect(blurb).not.toBe('')
    }
  })

  it('gives both in every language, not only the one it was written in', () => {
    setLocale('ja')
    for (const code of DIAGNOSTIC_CODES) {
      expect(codeLabel(code).title).toBe(ja[`diagnostic.${code}.title`])
      expect(codeLabel(code).blurb).toBe(ja[`diagnostic.${code}.blurb`])
    }
  })

  it('labels an unknown code readably rather than showing the raw slug', () => {
    // The backend can add a diagnostic code without a frontend release.
    const { title, blurb } = codeLabel('some_new_backend_code')
    expect(title).toBe('some new backend code')
    expect(blurb).toBe('')
  })

  it('leaves an unknown code as its own slug in every language', () => {
    // Better a slug than nothing: the alternative is a finding the backend
    // knows about disappearing from the panel in one language.
    setLocale('ja')
    expect(codeLabel('some_new_backend_code').title).toBe('some new backend code')
  })

  it('has a label for both parse-failure codes, which are the ones shown first', () => {
    for (const code of PARSE_FAILURE_CODES) {
      expect(DIAGNOSTIC_CODES).toContain(code)
    }
  })
})
