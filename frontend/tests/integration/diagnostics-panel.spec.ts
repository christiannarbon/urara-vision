/**
 * The Diagnostics panel, mounted.
 *
 * This component is the reason the tool pays for itself, and its rules are all
 * about presentation: dropped documents above findings whatever their severity,
 * grouping by code, and a readable label for a code the frontend has never
 * seen. Those are only observable by rendering it.
 */

import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import DiagnosticsPanel from '../../src/components/DiagnosticsPanel.vue'
import type { Diagnostic } from '../../src/api/types'

function diag(over: Partial<Diagnostic> & { code: string }): Diagnostic {
  return { severity: 'warning', message: over.code + ' message', ...over }
}

function panel(diagnostics: Diagnostic[], open = true) {
  return mount(DiagnosticsPanel, { props: { open, diagnostics } })
}

describe('visibility', () => {
  it('renders nothing when closed', () => {
    const w = panel([diag({ code: 'unresolved_reference' })], false)
    expect(w.find('[role="region"]').exists()).toBe(false)
  })

  it('renders the drawer when open', () => {
    const w = panel([])
    expect(w.find('[role="region"]').exists()).toBe(true)
    expect(w.text()).toContain('Diagnostics')
  })
})

describe('empty state', () => {
  it('says so plainly rather than showing an empty list', () => {
    const w = panel([])
    expect(w.text()).toContain('Nothing to report')
  })
})

describe('sectioning', () => {
  it('puts dropped documents above findings, despite their lower severity', () => {
    // The ordering rule that matters: a skipped document is only a warning, but
    // it is the one finding that means the model is incomplete.
    const w = panel([
      diag({ code: 'unresolved_reference', severity: 'error' }),
      diag({ code: 'empty_document', severity: 'warning' }),
    ])
    const text = w.text()
    expect(text.indexOf('could not be parsed')).toBeGreaterThanOrEqual(0)
    expect(text.indexOf('could not be parsed')).toBeLessThan(text.indexOf('Model findings'))
  })

  it('shows only the sections that have something in them', () => {
    const findingsOnly = panel([diag({ code: 'conformed_drift' })])
    expect(findingsOnly.text()).toContain('Model findings')
    expect(findingsOnly.text()).not.toContain('could not be parsed')

    const droppedOnly = panel([diag({ code: 'empty_document' })])
    expect(droppedOnly.text()).toContain('could not be parsed')
    expect(droppedOnly.text()).not.toContain('Model findings')
  })
})

describe('grouping', () => {
  it('collapses repeated codes into one group with a count', () => {
    const w = panel([
      diag({ code: 'unresolved_reference', message: 'a' }),
      diag({ code: 'unresolved_reference', message: 'b' }),
      diag({ code: 'unresolved_reference', message: 'c' }),
      diag({ code: 'conformed_drift', message: 'd' }),
    ])
    const groups = w.findAll('.group')
    expect(groups).toHaveLength(2)
    expect(w.text()).toContain('Unresolved references')
    expect(w.text()).toContain('3')
  })

  it('labels an unknown code readably instead of showing the raw slug', () => {
    // The backend can add a diagnostic code without a frontend release; the
    // group heading has to stay readable when there is no label for it.
    const w = panel([diag({ code: 'some_new_backend_code', message: 'the message' })])
    const heading = w.find('.gtitle').text()
    expect(heading).toBe('some new backend code')
    expect(heading).not.toContain('_')
    // No blurb exists for an unknown code, so none is rendered.
    expect(w.find('.blurb').exists()).toBe(false)
  })

  it('lists every message in a group', () => {
    const w = panel([
      diag({ code: 'conformed_drift', message: 'first drift' }),
      diag({ code: 'conformed_drift', message: 'second drift' }),
    ])
    expect(w.text()).toContain('first drift')
    expect(w.text()).toContain('second drift')
  })

  it('collapses and expands a group on click', async () => {
    const w = panel([diag({ code: 'conformed_drift', message: 'the drift' })])
    expect(w.text()).toContain('the drift')

    await w.find('.ghead').trigger('click')
    expect(w.text()).not.toContain('the drift')

    await w.find('.ghead').trigger('click')
    expect(w.text()).toContain('the drift')
  })
})

describe('severity filter', () => {
  it('counts each severity', () => {
    const w = panel([
      diag({ code: 'a', severity: 'error' }),
      diag({ code: 'b', severity: 'warning' }),
      diag({ code: 'c', severity: 'warning' }),
      diag({ code: 'd', severity: 'info' }),
    ])
    const chips = w.findAll('.fchip').map((c) => c.text())
    expect(chips[0]).toContain('4')
    expect(chips[1]).toContain('1')
    expect(chips[2]).toContain('2')
    expect(chips[3]).toContain('1')
  })

  it('narrows the list to one severity', async () => {
    const w = panel([
      diag({ code: 'unresolved_reference', severity: 'error', message: 'an error' }),
      diag({ code: 'conformed_drift', severity: 'warning', message: 'a warning' }),
    ])
    expect(w.text()).toContain('an error')
    expect(w.text()).toContain('a warning')

    // The chips are all, error, warning, info in order.
    await w.findAll('.fchip')[1].trigger('click')
    expect(w.text()).toContain('an error')
    expect(w.text()).not.toContain('a warning')
  })
})

describe('events', () => {
  it('emits close', async () => {
    const w = panel([])
    await w.find('[aria-label="Close diagnostics"]').trigger('click')
    expect(w.emitted('close')).toHaveLength(1)
  })

  it('emits select for a diagnostic that names a table', async () => {
    const w = panel([
      diag({ code: 'isolated_fact', message: 'lonely', tableId: 'domain_one/fact_primary' }),
    ])
    await w.find('.msg.linkish').trigger('click')
    expect(w.emitted('select')).toEqual([['domain_one/fact_primary']])
  })

  it('does not offer navigation for a diagnostic with no table', () => {
    // A dropped document has no table to jump to, so the message must not look
    // clickable.
    const w = panel([diag({ code: 'empty_document', docPath: 'domain_one/blank.md' })])
    expect(w.find('.msg.linkish').exists()).toBe(false)
    expect(w.text()).toContain('domain_one/blank.md')
  })
})
