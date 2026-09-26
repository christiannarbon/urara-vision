/** Turning a diff into graph marks. */

import { describe, expect, it } from 'vitest'

import type { DiffResult, GraphData, RelationshipDiff } from '../../src/api/types'
import { buildDiffMarks } from '../../src/graph/diff-marks'

const zero = { added: 0, removed: 0, changed: 0 }

function result(parts: Partial<DiffResult>): DiffResult {
  return {
    project: 'p',
    from: { version: '1', snapshotId: 's1' },
    to: { version: '2', snapshotId: 's2' },
    summary: { domains: zero, tables: zero, columns: zero, relationships: zero, lineage: zero },
    domains: [],
    tables: [],
    relationships: [],
    lineage: [],
    ...parts,
  }
}

function rel(from: string, to: string, fromColumn: string, toColumn: string, change: RelationshipDiff['change']): RelationshipDiff {
  return { fromTableId: from, toTableId: to, targetRef: to.split('/')[1] ?? '', fromColumn, toColumn, change, fields: [] }
}

const graph: GraphData = {
  nodes: [],
  links: [
    { id: 'e1', source: 'a/fact', target: 'a/dim', type: 'joins', fromColumn: 'dim_id', toColumn: 'dim_id' },
    // Declared one-to-many from a/dim, so the backend reversed it.
    { id: 'e2', source: 'a/child', target: 'a/dim', type: 'joins', fromColumn: 'dim_id', toColumn: 'id' },
    { id: 'e3', source: 'a/x', target: 'a/y', type: 'joins' },
    { id: 'l1', source: 'a/fact', target: 'src.raw', type: 'derived_from', columns: ['dim_id'] },
  ],
}

describe('buildDiffMarks', () => {
  it('marks added and changed tables, not removed ones', () => {
    const m = buildDiffMarks(
      result({
        tables: [
          { id: 'a/new', domainId: 'a', change: 'added', fields: [], columns: [] },
          { id: 'a/old', domainId: 'a', change: 'removed', fields: [], columns: [] },
          { id: 'a/fact', domainId: 'a', change: 'changed', fields: [{ field: 'grain', from: 'x', to: 'y' }], columns: [] },
        ],
      }),
      graph,
    )
    expect([...m.nodes]).toEqual([
      ['a/new', 'added'],
      ['a/fact', 'changed'],
    ])
  })

  it('marks a column-only change as changed', () => {
    const m = buildDiffMarks(
      result({
        tables: [
          {
            id: 'a/dim', domainId: 'a', change: 'changed', fields: [],
            columns: [{ name: 'dim_id', change: 'changed', fields: [{ field: 'type', from: 'INT64', to: 'STRING' }] }],
          },
        ],
      }),
      graph,
    )
    expect(m.nodes.get('a/dim')).toBe('changed')
  })

  it('matches relationships by endpoints and columns, in either direction', () => {
    const m = buildDiffMarks(
      result({
        relationships: [
          rel('a/fact', 'a/dim', 'dim_id', 'dim_id', 'added'),
          rel('a/dim', 'a/child', 'id', 'dim_id', 'changed'),
          rel('a/x', 'a/y', 'x_id', 'y_id', 'changed'),
        ],
      }),
      graph,
    )
    expect(m.edges).toEqual(
      new Map([
        ['e1', 'added'],
        ['e2', 'changed'],
        ['e3', 'changed'],
      ]),
    )
  })

  it('ignores relationships with no matching link', () => {
    const m = buildDiffMarks(
      result({
        relationships: [
          rel('a/fact', 'a/dim', 'other', 'other', 'added'),
          { ...rel('a/fact', '', 'z', 'z', 'added'), targetRef: 'dim_missing' },
          rel('a/fact', 'a/dim', 'dim_id', 'dim_id', 'removed'),
        ],
      }),
      graph,
    )
    expect(m.edges.size).toBe(0)
  })
})
