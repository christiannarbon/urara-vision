/** The layouts the canvas can arrange a model with. */

import type { MessageKey } from '../i18n'

export type LayoutMode = 'force' | 'layered' | 'radial'

export interface LayoutSpec {
  id: LayoutMode
  /** Catalogue keys rather than words. */
  labelKey: MessageKey
  /** Shown as the control's tooltip: what this layout is for. */
  hintKey: MessageKey
  /** Whether domain clustering survives this layout. */
  grouping: boolean
}

export const LAYOUTS: LayoutSpec[] = [
  { id: 'force', labelKey: 'layout.force', hintKey: 'layout.force.hint', grouping: true },
  { id: 'layered', labelKey: 'layout.layered', hintKey: 'layout.layered.hint', grouping: false },
  { id: 'radial', labelKey: 'layout.radial', hintKey: 'layout.radial.hint', grouping: false },
]

const byId = new Map(LAYOUTS.map((l) => [l.id, l]))

/** Whether a layout keeps domain clustering. Unknown modes are treated as force. */
export function supportsGrouping(mode: LayoutMode): boolean {
  return byId.get(mode)?.grouping ?? true
}

/** Cytoscape options for a layout. */
export function layoutOptions(mode: LayoutMode, count: number): Record<string, unknown> {
  const animate = count <= 40
  switch (mode) {
    case 'layered':
      return {
        name: 'dagre',
        rankDir: 'TB',
        ranker: 'network-simplex',
        // Ranks are separated far more than nodes within a rank: the vertical gap is the thing
        // carrying…
        rankSep: 110,
        nodeSep: 46,
        edgeSep: 14,
        animate,
        animationDuration: 320,
        fit: true,
        padding: 46,
        nodeDimensionsIncludeLabels: true,
      }
    case 'radial':
      return {
        name: 'concentric',
        // Degree, not any notion of role: it puts a fact at the centre of its star and a hub at
        // the centre…
        concentric: (n: { degree: (includeLoops?: boolean) => number }) => n.degree(false),
        levelWidth: () => 2,
        minNodeSpacing: 34,
        animate,
        animationDuration: 320,
        fit: true,
        padding: 46,
        nodeDimensionsIncludeLabels: true,
        avoidOverlap: true,
      }
    default:
      return {
        name: 'fcose',
        quality: 'proof',
        animate,
        animationDuration: 320,
        randomize: true,
        fit: true,
        padding: 46,
        nodeDimensionsIncludeLabels: true,
        uniformNodeDimensions: false,
        packComponents: true,
        // Dimensions want holding close to the fact they hang off; the repulsion is kept modest so…
        nodeRepulsion: 7000,
        idealEdgeLength: 105,
        edgeElasticity: 0.42,
        gravity: 0.3,
        gravityRange: 3.2,
        // Pulls each compound's children in on themselves, which makes for a
        // tighter hull and more air between neighbouring domains.
        gravityCompound: 1.4,
        gravityRangeCompound: 1.5,
        nestingFactor: 0.12,
        numIter: 2500,
      }
  }
}
