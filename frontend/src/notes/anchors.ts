/** Note anchors, in the ID formats backend/internal/notes expects. */

import type { AnchorKind } from '../api/types'

export type Anchor = { kind: AnchorKind; id: string }

export const anchorKey = (a: Anchor) => `${a.kind}:${a.id}`
export const tableAnchor = (tableId: string): Anchor => ({ kind: 'table', id: tableId })
export const columnAnchor = (tableId: string, column: string): Anchor => ({ kind: 'column', id: `${tableId}#${column}` })
export const lineageAnchor = (tableId: string, column: string): Anchor => ({ kind: 'lineage', id: `${tableId}#${column}` })
export const relationshipAnchor = (relationshipId: string): Anchor => ({ kind: 'relationship', id: relationshipId })
export const domainAnchor = (domainId: string): Anchor => ({ kind: 'domain', id: domainId })

/** The table a column or lineage anchor belongs to; splits on the last '#' as the backend does. */
export function anchorTableId(a: Anchor): string | null {
  if (a.kind === 'table') return a.id
  if (a.kind !== 'column' && a.kind !== 'lineage') return null
  const i = a.id.lastIndexOf('#')
  return i > 0 ? a.id.slice(0, i) : null
}
