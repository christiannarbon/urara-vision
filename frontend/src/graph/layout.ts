/**
 * The layouts the canvas can arrange a model with.
 *
 * No single layout suits every way a warehouse gets modelled, which is the
 * whole reason there is a choice here:
 *
 *   Force has no reading direction, which is right for a star schema -- nothing
 *   about a fact and its dimensions is upstream or downstream of anything else.
 *   It is also the only layout that groups, because fcose lays compound parents
 *   out as units and never lets two of them intersect; that is what keeps the
 *   domain hulls apart.
 *
 *   Layered ranks nodes along the direction of their joins. A snowflake's
 *   normalisation depth and a Data Vault's hub-link-satellite tiers are both
 *   genuinely layered structures, and seeing them as a flat cloud loses the one
 *   thing worth reading.
 *
 *   Radial puts the busiest tables in the middle and works outwards by degree.
 *   That is a star schema drawn the way it is usually drawn on a whiteboard,
 *   and it puts Data Vault hubs where a reader expects to find them.
 *
 * Neither dagre nor concentric understands compound nodes, so grouping is a
 * force-mode feature and the UI says so rather than silently dropping it.
 */

import type { MessageKey } from '../i18n'

export type LayoutMode = 'force' | 'layered' | 'radial'

export interface LayoutSpec {
  id: LayoutMode
  /**
   * Catalogue keys rather than words. This table is built once at module
   * load, so a name settled here would outlive the language it was chosen in;
   * the control resolves both when it renders.
   */
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

/**
 * Cytoscape options for a layout.
 *
 * count is the number of nodes being laid out, and only decides whether the
 * arrangement animates: animating from the random seed positions means showing
 * the wrong picture for the length of the tween. A focused view is small enough
 * that the motion reads as helpful; the whole model is not.
 */
export function layoutOptions(mode: LayoutMode, count: number): Record<string, unknown> {
  const animate = count <= 40
  switch (mode) {
    case 'layered':
      return {
        name: 'dagre',
        rankDir: 'TB',
        ranker: 'network-simplex',
        // Ranks are separated far more than nodes within a rank: the vertical
        // gap is the thing carrying meaning here, and a shallow one reads as
        // an accident rather than as a level.
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
        // Degree, not any notion of role: it puts a fact at the centre of its
        // star and a hub at the centre of its vault without either layout
        // needing to know what a fact or a hub is.
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
        // Dimensions want holding close to the fact they hang off; the
        // repulsion is kept modest so separate domains still form visible
        // clusters. The same setting does the right thing for a Data Vault's
        // satellites around their hub.
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
