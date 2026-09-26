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

  it('finds the owning table, splitting on the last #', () => {
    expect(anchorTableId(columnAnchor('a/b', 'c'))).toBe('a/b')
    expect(anchorTableId({ kind: 'lineage', id: 'a/b#c#d' })).toBe('a/b#c')
    expect(anchorTableId(tableAnchor('a/b'))).toBe('a/b')
    expect(anchorTableId(domainAnchor('sales'))).toBeNull()
    expect(anchorTableId(relationshipAnchor('r1'))).toBeNull()
  })
})
