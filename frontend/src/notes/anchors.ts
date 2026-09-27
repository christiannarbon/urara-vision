/** Note anchors, in the ID formats backend/internal/notes expects. */

import type { AnchorKind } from '../api/types'

export type Anchor = { kind: AnchorKind; id: string }

export const anchorKey = (a: Anchor) => `${a.kind}:${a.id}`
export const tableAnchor = (tableId: string): Anchor => ({ kind: 'table', id: tableId })
export const columnAnchor = (tableId: string, column: string): Anchor => ({ kind: 'column', id: `${tableId}#${column}` })
export const lineageAnchor = (tableId: string, column: string): Anchor => ({ kind: 'lineage', id: `${tableId}#${column}` })
export const relationshipAnchor = (relationshipId: string): Anchor => ({ kind: 'relationship', id: relationshipId })
export const domainAnchor = (domainId: string): Anchor => ({ kind: 'domain', id: domainId })

/** The known table a column or lineage anchor belongs to. Names may contain '#', so each split is tried, longest table first. */
export function anchorTableId(a: Anchor, tableIds: { has(id: string): boolean }): string | null {
  if (a.kind === 'table') return a.id
  if (a.kind !== 'column' && a.kind !== 'lineage') return null
  for (let i = a.id.lastIndexOf('#'); i > 0; i = a.id.lastIndexOf('#', i - 1)) {
    const prefix = a.id.slice(0, i)
    if (tableIds.has(prefix)) return prefix
  }
  return null
}
