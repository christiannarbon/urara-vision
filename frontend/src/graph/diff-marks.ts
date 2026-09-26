/** Which graph elements a diff marks. Removed things are not on the newer graph. */

import type { DiffResult, GraphData } from '../api/types'

export type Mark = 'added' | 'changed'

export interface DiffMarks {
  nodes: Map<string, Mark>
  edges: Map<string, Mark>
}

const key = (...parts: string[]) => parts.join('|')

export function buildDiffMarks(result: DiffResult, graph: GraphData): DiffMarks {
  const nodes = new Map<string, Mark>()
  for (const t of result.tables) {
    if (t.change !== 'removed') nodes.set(t.id, t.change)
  }

  // Graph edges are direction-normalised (one-to-many is reversed), so each
  // link is indexed both ways. A link without columns matches on endpoints.
  const links = new Map<string, string>()
  for (const l of graph.links) {
    if (l.type !== 'joins') continue
    if (l.fromColumn || l.toColumn) {
      links.set(key(l.source, l.target, l.fromColumn ?? '', l.toColumn ?? ''), l.id)
      links.set(key(l.target, l.source, l.toColumn ?? '', l.fromColumn ?? ''), l.id)
    } else {
      links.set(key(l.source, l.target), l.id)
      links.set(key(l.target, l.source), l.id)
    }
  }

  const edges = new Map<string, Mark>()
  for (const r of result.relationships) {
    // Unresolved joins have no target node, so no link.
    if (r.change === 'removed' || !r.toTableId) continue
    const id =
      links.get(key(r.fromTableId, r.toTableId, r.fromColumn, r.toColumn)) ??
      links.get(key(r.fromTableId, r.toTableId))
    if (id) edges.set(id, r.change)
  }
  return { nodes, edges }
}
