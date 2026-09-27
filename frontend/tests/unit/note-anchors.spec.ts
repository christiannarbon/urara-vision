/** Note anchor IDs. */

import { describe, expect, it } from 'vitest'

import {
  anchorKey,
  anchorTableId,
  columnAnchor,
  domainAnchor,
  lineageAnchor,
  relationshipAnchor,
  tableAnchor,
} from '../../src/notes/anchors'

describe('anchors', () => {
  it('builds the IDs the backend expects', () => {
    expect(tableAnchor('a/b')).toEqual({ kind: 'table', id: 'a/b' })
    expect(columnAnchor('a/b', 'c')).toEqual({ kind: 'column', id: 'a/b#c' })
    expect(lineageAnchor('a/b', 'c')).toEqual({ kind: 'lineage', id: 'a/b#c' })
    expect(relationshipAnchor('r1')).toEqual({ kind: 'relationship', id: 'r1' })
    expect(domainAnchor('sales')).toEqual({ kind: 'domain', id: 'sales' })
    expect(anchorKey(columnAnchor('a/b', 'c'))).toBe('column:a/b#c')
  })

  it('finds the owning table among the known ones, whatever # the names hold', () => {
    const known = new Set(['a/b', 'sales/fact', 'x/t#1'])
    expect(anchorTableId(columnAnchor('a/b', 'c'), known)).toBe('a/b')
    expect(anchorTableId(columnAnchor('sales/fact', 'amount#usd'), known)).toBe('sales/fact')
    expect(anchorTableId(lineageAnchor('x/t#1', 'id'), known)).toBe('x/t#1')
    expect(anchorTableId(columnAnchor('gone/t', 'c'), known)).toBeNull()
    expect(anchorTableId(tableAnchor('a/b'), known)).toBe('a/b')
    expect(anchorTableId(domainAnchor('sales'), known)).toBeNull()
    expect(anchorTableId(relationshipAnchor('r1'), known)).toBeNull()
  })

})
