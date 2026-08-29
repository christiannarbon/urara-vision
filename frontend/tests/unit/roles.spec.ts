/** How a role id becomes a shape, a label and a colour. */
import { describe, expect, it } from 'vitest'

import { roleColor, roleLabel, roleSpec, rolesPresent } from '../../src/graph/roles'

const FACT = '#0f766e'
const DIM = '#b45309'

describe('roleSpec', () => {
  it('describes the built-in vocabularies', () => {
    expect(roleSpec('fact').label).toBe('Fact')
    expect(roleSpec('fact').family).toBe('kimball')
    expect(roleSpec('satellite').family).toBe('vault')
    expect(roleSpec('associative').family).toBe('relational')
  })

  it('gives every built-in role its own shape', () => {
    const ids = ['fact', 'factless', 'dimension', 'outrigger', 'bridge', 'junk', 'degenerate',
      'hub', 'link', 'satellite', 'pit', 'entity', 'associative', 'lookup', 'reference']
    const shapes = ids.map((id) => roleSpec(id).shape)
    expect(new Set(shapes).size).toBe(shapes.length)
  })

  it('invents a spec for a role read from the documents', () => {
    const s = roleSpec('anchor')
    expect(s.id).toBe('anchor')
    expect(s.label).toBe('Anchor')
    expect(s.family).toBe('other')
    expect(s.shape).toBeTruthy()
  })

  it('is stable: the same unknown role always looks the same', () => {
    expect(roleSpec('anchor')).toEqual(roleSpec('anchor'))
  })

  it('separates two unknown roles from each other', () => {
    const a = roleSpec('anchor')
    const b = roleSpec('tie_table')
    expect(a.shape !== b.shape || a.hueShift !== b.hueShift).toBe(true)
  })

  it('falls back to unknown for an empty kind', () => {
    expect(roleSpec('').id).toBe('unknown')
  })
})

describe('roleLabel', () => {
  it('reads a slug as a sentence', () => {
    expect(roleLabel('point_in_time')).toBe('Point in time')
    expect(roleLabel('tie-table')).toBe('Tie table')
  })
})

describe('rolesPresent', () => {
  it('deduplicates and orders by the vocabulary', () => {
    const got = rolesPresent(['dimension', 'fact', 'dimension', 'hub']).map((r) => r.id)
    expect(got).toEqual(['fact', 'dimension', 'hub'])
  })

  it('puts roles read from the documents after the built-in ones', () => {
    const got = rolesPresent(['anchor', 'fact']).map((r) => r.id)
    expect(got).toEqual(['fact', 'anchor'])
  })

  it('ignores empty kinds rather than inventing a role for them', () => {
    expect(rolesPresent(['', 'fact']).map((r) => r.id)).toEqual(['fact'])
  })
})

describe('roleColor', () => {
  it('leaves fact and dimension on the theme tokens untouched', () => {
    expect(roleColor(roleSpec('fact'), FACT, DIM)).toBe(FACT)
    expect(roleColor(roleSpec('dimension'), FACT, DIM)).toBe(DIM)
  })

  it('shifts every other role off one of the two', () => {
    for (const id of ['factless', 'outrigger', 'hub', 'satellite', 'entity', 'lookup']) {
      const c = roleColor(roleSpec(id), FACT, DIM)
      expect(c).toMatch(/^hsl\(/)
      expect(c).not.toBe(FACT)
      expect(c).not.toBe(DIM)
    }
  })

  it('keeps lightness inside the readable band', () => {
    // Both ends matter: near-white loses the node's white border, near-black
    // stops reading as the theme's colour at all.
    for (const base of ['#ffffff', '#000000', FACT, DIM]) {
      for (const id of ['factless', 'reference', 'junk', 'pit']) {
        const c = roleColor(roleSpec(id), base, base)
        const l = Number(/,\s*([\d.]+)%\)$/.exec(c)?.[1] ?? NaN)
        expect(l).toBeGreaterThanOrEqual(24)
        expect(l).toBeLessThanOrEqual(72)
      }
    }
  })

  it('falls back to the theme colour when a token cannot be read', () => {
    // Wrong for the role, but right for the theme, which is the better failure.
    expect(roleColor(roleSpec('hub'), 'var(--oops)', 'var(--oops)')).toBe('var(--oops)')
  })

  it('accepts three-digit hex', () => {
    expect(roleColor(roleSpec('hub'), '#0a8', '#0a8')).toMatch(/^hsl\(/)
  })
})
