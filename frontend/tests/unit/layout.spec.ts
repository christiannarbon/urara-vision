/** The layouts the canvas offers and what each one can carry. */
import { afterEach, describe, expect, it } from 'vitest'

import { LAYOUTS, layoutOptions, supportsGrouping, type LayoutMode } from '../../src/graph/layout'
import { setLocale, translate } from '../../src/i18n'
import { messages as ja } from '../../src/i18n/messages/ja'

describe('LAYOUTS', () => {
  afterEach(() => setLocale('en'))

  it('offers force, layered and radial', () => {
    expect(LAYOUTS.map((l) => l.id)).toEqual(['force', 'layered', 'radial'])
  })

  it('names force first, since it is the default', () => {
    expect(LAYOUTS[0].id).toBe('force')
  })

  it('carries catalogue keys rather than words, so a name is never frozen', () => {
    setLocale('ja')
    expect(LAYOUTS.map((l) => translate(l.labelKey))).toEqual([
      ja['layout.force'],
      ja['layout.layered'],
      ja['layout.radial'],
    ])
  })
})

describe('supportsGrouping', () => {
  it('allows grouping only in force', () => {
    // Neither dagre nor concentric understands compound nodes, so the domain
    // hulls cannot survive those two.
    expect(supportsGrouping('force')).toBe(true)
    expect(supportsGrouping('layered')).toBe(false)
    expect(supportsGrouping('radial')).toBe(false)
  })

  it('treats an unrecognised mode as force rather than dropping the grouping', () => {
    expect(supportsGrouping('nonsense' as LayoutMode)).toBe(true)
  })
})

describe('layoutOptions', () => {
  it('maps each mode onto its cytoscape layout', () => {
    expect(layoutOptions('force', 10).name).toBe('fcose')
    expect(layoutOptions('layered', 10).name).toBe('dagre')
    expect(layoutOptions('radial', 10).name).toBe('concentric')
  })

  it('animates a small graph and not a large one', () => {
    for (const mode of ['force', 'layered', 'radial'] as LayoutMode[]) {
      expect(layoutOptions(mode, 10).animate).toBe(true)
      expect(layoutOptions(mode, 400).animate).toBe(false)
    }
  })

  it('fits and pads every layout the same, so switching does not jump the view', () => {
    for (const mode of ['force', 'layered', 'radial'] as LayoutMode[]) {
      const o = layoutOptions(mode, 10)
      expect(o.fit).toBe(true)
      expect(o.padding).toBe(46)
    }
  })

  it('ranks the radial layout by degree, not by role', () => {
    const concentric = layoutOptions('radial', 10).concentric as (n: unknown) => number
    expect(concentric({ degree: () => 7 })).toBe(7)
  })
})
