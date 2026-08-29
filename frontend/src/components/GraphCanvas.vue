<script setup lang="ts">
/**
 * Cytoscape-backed relationship graph.
 *
 * Nodes are grouped into compound parents -- one per domain, one per source
 * dataset -- which is what keeps the clusters from overlapping: fcose lays
 * compounds out as units and never lets two of them intersect. The parents
 * themselves are invisible; the visible cluster is a convex hull drawn on a
 * canvas layer underneath the graph, so the outline hugs the nodes instead of
 * boxing them in.
 *
 * Cytoscape draws to a canvas, so it cannot resolve CSS custom properties
 * itself. The palette is read off the document element and the stylesheet is
 * rebuilt whenever the theme changes.
 *
 * Three layouts are offered because no single one suits every way a warehouse
 * gets modelled. Force is the default and the only one that groups: a star
 * schema has no natural reading direction, and clusters are what make a model
 * of several domains legible. Layered ranks the graph along its joins, which is
 * what a snowflake's normalisation depth and a Data Vault's hub-link-satellite
 * tiers actually are. Radial puts the busiest tables at the centre, which is a
 * classic star seen the way it is usually drawn.
 */
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import cytoscape from 'cytoscape'
import dagre from 'cytoscape-dagre'
import fcose from 'cytoscape-fcose'

import type { Domain, GraphData, GraphNode } from '../api/types'
import {
  convexHull,
  ringBounds,
  ringTopAt,
  traceInflatedHull,
  fitPetal,
  placePetal,
  tracePetal,
  petalTop,
  type Petal,
  type Point,
} from '../graph/hull'
import { resolvePetalOverlaps } from '../graph/petal-overlap'
import { layoutOptions, supportsGrouping, type LayoutMode } from '../graph/layout'
import { rolesPresent, roleColor, type RoleSpec } from '../graph/roles'
import {
  canvasTheme,
  domainColor,
  domainIndex,
  paletteFamily,
  sourceColor,
  SAKURA_ART,
  type ClusterColor,
  type Theme,
} from '../graph/palette'
import { useTheme } from '../composables/useTheme'

cytoscape.use(fcose)
cytoscape.use(dagre)

const props = defineProps<{
  data: GraphData
  domains: Domain[]
  selectedId: string | null
  loading: boolean
  layoutMode: LayoutMode
}>()

const emit = defineEmits<{
  (e: 'select', id: string | null): void
  (e: 'focus', id: string): void
}>()

const container = ref<HTMLDivElement | null>(null)
const hullCanvas = ref<HTMLCanvasElement | null>(null)
const { art } = useTheme()

let cy: cytoscape.Core | null = null
let resizeObserver: ResizeObserver | null = null

const zoomLevel = ref(1)
const hoveredId = ref<string | null>(null)
// True while a layout is still running. Without this the canvas shows nodes at
// their pre-layout positions -- a cramped blob in the corner -- for as long as
// fcose takes to solve.
const laying = ref(false)
/**
 * Bumped once the new theme's custom properties are committed. The legend is
 * plain DOM reading resolved token values, so it has nothing to react to on its
 * own; this is what tells it to read them again.
 */
const paletteTick = ref(0)
/** Grouping is the default view; turning it off gives the old free layout. */
const clustered = ref(true)

/**
 * Whether clusters are actually drawn. The toggle is the reader's preference
 * and survives a trip through a layout that cannot honour it, so switching back
 * to Force restores the grouping they had rather than silently losing it.
 */
const grouping = computed(() => clustered.value && supportsGrouping(props.layoutMode))

const isEmpty = computed(() => props.data.nodes.length === 0)
const busy = computed(() => props.loading || laying.value)

/** Palette slot per domain, in the order the snapshot lists them. */
const domainSlot = computed(() => domainIndex(props.domains.map((d) => d.id)))

/** How far the hull is inflated past the node bounds, in model units. */
const HULL_PAD = 22
/** Compound padding. Kept above HULL_PAD so a hull stays inside its own box. */
const CLUSTER_PADDING = 34
/**
 * Clearance a petal keeps from the nodes it holds. Well short of HULL_PAD: a
 * petal is roomy through its belly whatever it is told, so padding it like the
 * plain hull only pushes the prongs further out.
 */
const PETAL_PAD = 8
/**
 * Clearance kept between two neighbouring petals, in model units.
 *
 * This is the setting that decides how much the sakura theme spreads a graph
 * out relative to the others: petals are far larger than the hulls they replace
 * and separating them means moving whole clusters, so every unit here is space
 * taken from the middle of the canvas. Enough to read as a deliberate gap, not
 * enough to pull a star schema apart.
 */
const PETAL_GAP = 16

/**
 * The roles this graph actually contains. Driving the stylesheet off what is on
 * screen rather than off the built-in vocabulary is what lets a role the
 * documents named themselves get a shape and a colour of its own.
 */
const presentRoles = computed<RoleSpec[]>(() =>
  rolesPresent(props.data.nodes.filter((n) => n.type !== 'source').map((n) => n.kind ?? '')),
)

/** Reads the resolved value of a CSS custom property. */
function token(name: string, fallback: string): string {
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return v || fallback
}

function buildStyle(): cytoscape.StylesheetJson {
  const fact = token('--fact', '#0f766e')
  const dim = token('--dim', '#b45309')
  const source = token('--source', '#475569')
  const edge = token('--edge', '#c3bcb1')
  const edgeCross = token('--edge-cross', '#b45309')
  const text = token('--text', '#1c1917')
  const textMuted = token('--text-muted', '#6b6259')
  const stroke = token('--node-stroke', '#ffffff')
  const accent = token('--accent', '#0f766e')
  const danger = token('--danger', '#b91c1c')

  return [
    {
      selector: 'node',
      style: {
        'background-color': dim,
        'border-width': 2,
        'border-color': stroke,
        label: 'data(label)',
        color: text,
        'font-family': token('--font-mono', 'monospace'),
        'font-size': 10,
        'font-weight': 500,
        'text-valign': 'bottom',
        'text-halign': 'center',
        'text-margin-y': 5,
        'text-wrap': 'wrap',
        'text-max-width': '110px',
        'min-zoomed-font-size': 7,
        width: 'data(size)',
        height: 'data(size)',
        'transition-property': 'background-color, border-color, opacity',
        'transition-duration': 140,
      },
    },
    // One rule per role actually present, rather than the whole vocabulary:
    // shape carries the role and colour is derived from the theme's two role
    // hues. See graph/roles.ts. A role the documents invented gets a rule here
    // just the same, which is what stops an unfamiliar vocabulary rendering as
    // a canvas of identical circles.
    ...presentRoles.value.map((r) => ({
      selector: `node[kind = "${r.id}"]`,
      style: { shape: r.shape, 'background-color': roleColor(r, fact, dim) },
    })),
    {
      selector: 'node[type = "source"]',
      style: {
        shape: 'hexagon',
        'background-color': source,
        'background-opacity': 0.55,
        color: textMuted,
        'font-size': 9,
      },
    },
    {
      selector: 'node[?conformed]',
      // A solid ring in the conformed hue; a double border renders as a noisy
      // starburst at small node sizes.
      style: { 'border-width': 3, 'border-color': token('--conformed', '#7c2d12') },
    },
    // The compound parents exist only to hold their children together. The
    // outline the reader sees is drawn on the hull layer instead, and the box
    // must not swallow taps meant for the background.
    {
      selector: 'node.cluster',
      style: {
        shape: 'round-rectangle',
        'background-opacity': 0,
        'border-width': 0,
        'text-opacity': 0,
        events: 'no',
        padding: CLUSTER_PADDING,
      },
    },
    {
      selector: 'edge',
      style: {
        width: 1.4,
        'line-color': edge,
        'target-arrow-color': edge,
        'target-arrow-shape': 'triangle',
        'arrow-scale': 0.75,
        'curve-style': 'bezier',
        'control-point-step-size': 34,
        opacity: 0.85,
        'transition-property': 'line-color, opacity, width',
        'transition-duration': 140,
      },
    },
    {
      selector: 'edge[type = "derived_from"]',
      style: {
        'line-style': 'dashed',
        'line-dash-pattern': [5, 4],
        'line-color': source,
        'target-arrow-color': source,
        opacity: 0.5,
        width: 1,
      },
    },
    {
      selector: 'edge[?crossDomain]',
      style: { 'line-color': edgeCross, 'target-arrow-color': edgeCross, width: 2 },
    },
    {
      selector: 'edge[resolution = "conformed"]',
      style: { 'line-style': 'dashed', 'line-dash-pattern': [7, 3] },
    },
    // Selection and hover emphasis.
    {
      selector: 'node.is-selected',
      style: {
        'border-width': 4,
        'border-color': accent,
        'z-index': 30,
        'font-size': 12,
        'font-weight': 700,
      },
    },
    {
      selector: 'node.is-neighbour',
      style: { 'border-color': accent, 'border-width': 3, 'z-index': 20 },
    },
    {
      selector: 'edge.is-incident',
      style: { 'line-color': accent, 'target-arrow-color': accent, width: 2.6, opacity: 1, 'z-index': 20 },
    },
    { selector: '.is-dimmed', style: { opacity: 0.12 } },
    { selector: 'node.is-orphan', style: { 'border-color': danger, 'border-style': 'dotted' } },
  ] as unknown as cytoscape.StylesheetJson
}

/** Node diameter grows with connectivity so hubs read as hubs. */
function nodeSize(degree: number, type: string): number {
  if (type === 'source') return 16
  const base = 22
  return Math.min(base + Math.sqrt(Math.max(degree, 0)) * 7, 58)
}

/**
 * The cluster a node belongs to: its domain, or -- for an upstream model, which
 * has no domain of its own -- the dataset it was read from.
 */
function clusterOf(n: GraphNode): { id: string; label: string; kind: 'domain' | 'source' } | null {
  if (n.type === 'source') {
    return n.dataset ? { id: `cluster:src:${n.dataset}`, label: n.dataset, kind: 'source' } : null
  }
  return n.domainId ? { id: `cluster:dom:${n.domainId}`, label: n.domainId, kind: 'domain' } : null
}

function toElements(data: GraphData): cytoscape.ElementDefinition[] {
  const present = new Set(data.nodes.map((n) => n.id))
  const els: cytoscape.ElementDefinition[] = []

  // Parents must be added before their children.
  if (grouping.value) {
    const seen = new Map<string, { label: string; kind: 'domain' | 'source' }>()
    for (const n of data.nodes) {
      const c = clusterOf(n)
      if (c && !seen.has(c.id)) seen.set(c.id, { label: c.label, kind: c.kind })
    }
    for (const [id, c] of seen) {
      els.push({
        group: 'nodes',
        classes: 'cluster',
        data: { id, label: c.label, clusterKind: c.kind },
      })
    }
  }

  for (const n of data.nodes) {
    const c = grouping.value ? clusterOf(n) : null
    els.push({
      group: 'nodes',
      data: {
        id: n.id,
        parent: c?.id,
        label: n.label,
        type: n.type,
        kind: n.kind ?? '',
        domainId: n.domainId ?? '',
        conformed: !!n.conformed,
        degree: n.degree,
        size: nodeSize(n.degree, n.type),
        columnCount: n.columnCount ?? 0,
        grain: n.grain ?? '',
        dataset: n.dataset ?? '',
      },
    })
  }

  for (const l of data.links) {
    // Guard against an edge whose endpoint was filtered out; Cytoscape throws
    // on a dangling reference rather than skipping it.
    if (!present.has(l.source) || !present.has(l.target)) continue
    els.push({
      group: 'edges',
      data: {
        id: l.id,
        source: l.source,
        target: l.target,
        type: l.type,
        cardinality: l.cardinality ?? '',
        fromColumn: l.fromColumn ?? '',
        toColumn: l.toColumn ?? '',
        crossDomain: !!l.crossDomain,
        resolution: l.resolution ?? '',
      },
    })
  }
  return els
}

function applyHighlight() {
  if (!cy) return
  const id = hoveredId.value ?? props.selectedId
  cy.batch(() => {
    cy!.elements().removeClass('is-selected is-neighbour is-incident is-dimmed')
    if (!id) return
    const node = cy!.getElementById(id)
    if (node.empty()) return

    const incident = node.connectedEdges()
    const neighbours = incident.connectedNodes()

    // Cytoscape cascades a parent's opacity onto its children, so dimming the
    // compounds would drag the highlighted nodes down with everything else.
    // The clusters fade on the hull layer instead.
    cy!.elements().not('.cluster').addClass('is-dimmed')
    node.removeClass('is-dimmed').addClass('is-selected')
    neighbours.removeClass('is-dimmed').addClass('is-neighbour')
    incident.removeClass('is-dimmed').addClass('is-incident')
    // The focused node itself should not carry the neighbour ring.
    node.removeClass('is-neighbour')
  })
  drawHulls()
}

function render(data: GraphData, relayout: boolean) {
  if (!cy) return
  const els = toElements(data)
  cy.batch(() => {
    cy!.elements().remove()
    cy!.add(els)
  })
  if (els.length === 0) {
    laying.value = false
    drawHulls()
    return
  }
  if (relayout) {
    runLayout(data.nodes.length)
  }
  applyHighlight()
}

/** Runs a layout, holding the overlay up until it settles and then fitting. */
function runLayout(count: number) {
  if (!cy) return
  laying.value = true
  const layout = cy.layout(layoutOptions(props.layoutMode, count) as any)
  layout.one('layoutstop', () => {
    laying.value = false
    // Before the fit: separating clusters changes the bounds being framed.
    if (grouping.value) separateClusters()
    cy?.fit(undefined, 46)
    applyHighlight()
  })
  layout.run()
}

// --- cluster hulls -------------------------------------------------------

interface HullShape {
  id: string
  label: string
  kind: 'domain' | 'source'
  slot: number
  /** Convex ring in model coordinates. */
  ring: Point[]
  /**
   * The same cluster as a sakura petal, for the theme that outlines it with
   * one. Null under every other theme: fitting one is far too much work to do
   * on every frame of a drag for a shape that will not be drawn.
   */
  petal: Petal | null
}

// Rings are held in model space and only transformed at paint time, so panning
// and zooming never re-run the hull solve.
let hullCache: HullShape[] | null = null

function invalidateHulls() {
  hullCache = null
}

function hulls(): HullShape[] {
  if (!hullCache) hullCache = computeHulls()
  return hullCache
}

/**
 * Whether the outlines are petals rather than hulls.
 *
 * Haru Urara wears sakura petals for eyes, so her theme wears one on every
 * cluster: the outline is not the hull at all but the smallest petal that
 * encloses the cluster (see fitPetal), painted from the palette's pink band
 * rather than from the colour wheel (see paletteFamily).
 */
function sakura(): boolean {
  return art.value === SAKURA_ART
}

function computeHulls(): HullShape[] {
  if (!cy || !grouping.value) return []
  const out: HullShape[] = []
  cy.nodes('.cluster').forEach((parent) => {
    const kids = parent.children()
    if (kids.empty()) return
    const pts: Point[] = []
    kids.forEach((n) => {
      // Labels are included so a name never hangs outside its own cluster.
      const b = n.boundingBox({ includeLabels: true, includeOverlays: false })
      pts.push({ x: b.x1, y: b.y1 }, { x: b.x2, y: b.y1 }, { x: b.x2, y: b.y2 }, { x: b.x1, y: b.y2 })
    })
    const kind = parent.data('clusterKind') === 'source' ? 'source' : 'domain'
    const ring = convexHull(pts)
    out.push({
      id: parent.id(),
      label: parent.data('label') ?? '',
      kind,
      slot: domainSlot.value.get(parent.data('label')) ?? 0,
      ring,
      petal: sakura() ? fitPetal(ring, PETAL_PAD) : null,
    })
  })
  return out
}

/**
 * Pushes clusters apart until no two petals overlap.
 *
 * Grouping nodes under compound parents is what keeps the plain hulls from
 * intersecting: a hull is inflated by less than the compound's padding, and
 * fcose never lets two compounds overlap. A petal does not inherit that. It
 * circumscribes the same nodes far more loosely, so it reaches straight past
 * the box that was holding its neighbours off.
 *
 * A petal cannot be shrunk to fix it -- it has to keep containing its nodes --
 * so the nodes move instead. Each cluster is translated as a unit, which leaves
 * the arrangement inside a domain exactly as the layout solved it and only
 * opens up the space between domains. Clusters that were already clear do not
 * move at all.
 *
 * This runs once after a layout settles rather than on every frame: it walks
 * every pair of clusters, which is far too much work for a drag.
 */
function separateClusters() {
  if (!cy || !grouping.value || !sakura()) return

  const shapes = hulls()
  if (shapes.length < 2) return
  const petals: Petal[] = []
  for (const s of shapes) {
    // Only the sakura theme fits petals, and the guard above means they are all
    // present; bail rather than guess if that ever stops being true.
    if (!s.petal) return
    petals.push(s.petal)
  }

  const offsets = resolvePetalOverlaps(petals, PETAL_GAP)
  let shifted = false
  cy.batch(() => {
    shapes.forEach((shape, i) => {
      const { dx, dy } = offsets[i]
      if (dx === 0 && dy === 0) return
      shifted = true
      cy!
        .getElementById(shape.id)
        .children()
        .positions((n) => ({ x: n.position('x') + dx, y: n.position('y') + dy }))
    })
  })
  // The rings were solved against the old positions, so they have to go.
  if (shifted) invalidateHulls()
}

/** The cluster holding the hovered or selected node, if any. */
function activeClusterId(): string | null {
  if (!cy) return null
  const id = hoveredId.value ?? props.selectedId
  if (!id) return null
  const n = cy.getElementById(id)
  if (n.empty()) return null
  const p = n.parent()
  return p.empty() ? null : p.first().id()
}

function roundRect(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  w: number,
  h: number,
  r: number,
) {
  ctx.beginPath()
  ctx.moveTo(x + r, y)
  ctx.arcTo(x + w, y, x + w, y + h, r)
  ctx.arcTo(x + w, y + h, x, y + h, r)
  ctx.arcTo(x, y + h, x, y, r)
  ctx.arcTo(x, y, x + w, y, r)
  ctx.closePath()
}

/**
 * Cluster name, seated on the outline the way a fieldset legend sits. `cx` and
 * `top` are where the outline runs: the pill's middle lands on that point.
 */
function drawCaption(
  ctx: CanvasRenderingContext2D,
  cx: number,
  top: number,
  label: string,
  colour: ClusterColor,
  panel: string,
  zoom: number,
) {
  if (!label) return
  const size = Math.max(10, Math.min(14, 12 * zoom))
  const cy0 = top

  ctx.font = `600 ${size}px ${token('--font-sans', 'sans-serif')}`
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  const w = ctx.measureText(label).width + 14
  const h = size + 7

  ctx.setLineDash([])
  roundRect(ctx, cx - w / 2, cy0 - h / 2, w, h, h / 2)
  ctx.fillStyle = panel
  ctx.fill()
  ctx.strokeStyle = colour.stroke
  ctx.lineWidth = 1
  ctx.stroke()

  ctx.fillStyle = colour.label
  ctx.fillText(label, cx, cy0 + 0.5)
}

function drawHulls() {
  const cv = hullCanvas.value
  const el = container.value
  if (!cv || !el) return
  const ctx = cv.getContext('2d')
  if (!ctx) return

  const dpr = window.devicePixelRatio || 1
  const w = el.clientWidth
  const h = el.clientHeight
  if (cv.width !== Math.round(w * dpr) || cv.height !== Math.round(h * dpr)) {
    cv.width = Math.round(w * dpr)
    cv.height = Math.round(h * dpr)
  }
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.clearRect(0, 0, w, h)

  if (!cy || !grouping.value) return
  const shapes = hulls()
  if (!shapes.length) return

  // Keyed off the canvas colour itself, not the app mode -- see canvasTheme.
  const theme: Theme = canvasTheme(token('--graph-bg', '#ffffff'))
  // A petal has to be a petal colour, so the outlines take their family from
  // the theme rather than always from the wheel.
  const family = paletteFamily(art.value)
  const zoom = cy.zoom()
  const pan = cy.pan()
  const pad = HULL_PAD * zoom
  const active = activeClusterId()
  const panel = token('--graph-bg', '#f6f4f0')

  ctx.lineJoin = 'round'
  ctx.lineCap = 'round'

  for (const s of shapes) {
    const colour =
      s.kind === 'source' ? sourceColor(theme, family) : domainColor(s.slot, theme, family)

    // A selection dims the rest of the graph; the hulls follow it down so the
    // highlighted cluster stays the loudest thing on screen.
    ctx.globalAlpha = active && active !== s.id ? 0.35 : 1

    // Where the caption seats: on the outline, above the middle of the shape.
    let capX: number
    let capY: number
    if (s.petal) {
      const bl = placePetal(s.petal, zoom, pan.x, pan.y)
      tracePetal(ctx, bl)
      capX = bl.cx
      capY = petalTop(bl)
    } else {
      const ring = s.ring.map((p) => ({ x: p.x * zoom + pan.x, y: p.y * zoom + pan.y }))
      traceInflatedHull(ctx, ring, pad)
      const b = ringBounds(ring)
      capX = (b.x1 + b.x2) / 2
      capY = ringTopAt(ring, capX) - pad
    }
    ctx.fillStyle = colour.fill
    ctx.fill()
    ctx.setLineDash(s.kind === 'source' ? [6, 5] : [])
    ctx.strokeStyle = colour.stroke
    ctx.lineWidth = 1.25
    ctx.stroke()

    drawCaption(ctx, capX, capY, s.label, colour, panel, zoom)
  }

  ctx.globalAlpha = 1
  ctx.setLineDash([])
}

function init() {
  if (!container.value) return
  cy = cytoscape({
    container: container.value,
    style: buildStyle(),
    minZoom: 0.08,
    maxZoom: 3.5,
    wheelSensitivity: 0.25,
    boxSelectionEnabled: false,
    autoungrabify: false,
  })

  cy.on('tap', 'node', (evt) => emit('select', evt.target.id()))
  cy.on('dbltap', 'node', (evt) => emit('focus', evt.target.id()))
  cy.on('tap', (evt) => {
    if (evt.target === cy) emit('select', null)
  })
  cy.on('mouseover', 'node', (evt) => {
    hoveredId.value = evt.target.id()
    applyHighlight()
  })
  cy.on('mouseout', 'node', () => {
    hoveredId.value = null
    applyHighlight()
  })
  cy.on('zoom', () => {
    zoomLevel.value = cy?.zoom() ?? 1
  })
  cy.on('position', 'node', invalidateHulls)
  cy.on('add remove', invalidateHulls)
  // Cytoscape repaints on pan, zoom and drag; the hull layer rides along.
  cy.on('render', drawHulls)

  render(props.data, true)

  // Cytoscape needs an explicit resize when its container changes size.
  resizeObserver = new ResizeObserver(() => {
    cy?.resize()
    drawHulls()
  })
  resizeObserver.observe(container.value)
}

onMounted(() => {
  nextTick(init)
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  cy?.destroy()
  cy = null
})

watch(
  () => props.data,
  (d) => {
    // A filter change can bring a role on screen that had none of its own
    // rules, so the stylesheet is rebuilt alongside the elements.
    cy?.style(buildStyle())
    render(d, true)
  },
  { deep: false },
)

// Layered and radial drop the compound parents, so the element graph itself
// changes and a plain relayout would not be enough.
watch(
  () => props.layoutMode,
  () => {
    invalidateHulls()
    render(props.data, true)
  },
)

watch(
  () => props.selectedId,
  () => applyHighlight(),
)

watch(domainSlot, () => drawHulls())

// Changing theme repaints the canvas: the palette, the corner radii and the
// label typeface all come from it.
watch(art, () => {
  // Wait a frame so the new custom-property values are committed before they
  // are read back off the document element.
  requestAnimationFrame(() => {
    cy?.style(buildStyle())
    paletteTick.value++
    applyHighlight()
    // The outline shape is part of the theme, so the cached hulls go with it.
    invalidateHulls()
    // Switching into the petal theme is what creates the overlaps, so this is
    // the other moment worth resolving them. Switching out leaves the extra
    // room behind rather than yanking the graph back together; the plain hulls
    // just sit in a slightly airier layout until the next relayout.
    separateClusters()
    drawHulls()
  })
})

function fit() {
  cy?.animate({ fit: { eles: cy.elements(), padding: 46 }, duration: 250 })
}
function zoomBy(factor: number) {
  if (!cy) return
  const next = Math.min(Math.max(cy.zoom() * factor, cy.minZoom()), cy.maxZoom())
  cy.animate({ zoom: { level: next, renderedPosition: centre() }, duration: 140 })
}
function centre() {
  const el = container.value
  return { x: (el?.clientWidth ?? 0) / 2, y: (el?.clientHeight ?? 0) / 2 }
}
function relayout() {
  if (!cy || cy.elements().length === 0) return
  runLayout(props.data.nodes.length)
}

const canGroup = computed(() => supportsGrouping(props.layoutMode))
const hasSources = computed(() => props.data.nodes.some((n) => n.type === 'source'))

const groupTitle = computed(() => {
  if (!canGroup.value) return 'Grouping is only available in the Force layout'
  return grouping.value ? 'Ungroup domains' : 'Group by domain'
})

/**
 * The legend's swatch colour. Read through the same tokens the canvas uses so
 * the two never disagree, and recomputed on a theme change by way of paletteTick.
 */
function legendColor(r: RoleSpec): string {
  void paletteTick.value
  return roleColor(r, token('--fact', '#0f766e'), token('--dim', '#b45309'))
}

/** Grouping changes the element graph itself, so it needs a fresh layout. */
function toggleClusters() {
  clustered.value = !clustered.value
  invalidateHulls()
  render(props.data, true)
}

/** Centres the view on a node without changing the selection. */
function panTo(id: string) {
  if (!cy) return
  const n = cy.getElementById(id)
  if (n.empty()) return
  cy.animate({ center: { eles: n }, zoom: Math.max(cy.zoom(), 1.1) }, { duration: 260 })
}

defineExpose({ fit, relayout, panTo })
</script>

<template>
  <div class="canvas-wrap">
    <canvas ref="hullCanvas" class="hulls" :class="{ 'canvas--hidden': busy }" aria-hidden="true" />

    <div
      ref="container"
      class="canvas"
      :class="{ 'canvas--hidden': busy }"
      role="application"
      aria-label="Table relationship graph"
    />

    <div v-if="busy" class="overlay">
      <div class="spinner" aria-hidden="true" />
      <span>{{ loading ? 'Building graph…' : 'Laying out…' }}</span>
    </div>

    <div v-else-if="isEmpty" class="overlay overlay--quiet">
      <p><strong>Nothing to draw</strong></p>
      <p class="muted">No tables match the current filters.</p>
    </div>

    <div class="controls" role="group" aria-label="Graph controls">
      <button class="ctl" title="Zoom in" aria-label="Zoom in" @click="zoomBy(1.35)">+</button>
      <button class="ctl" title="Zoom out" aria-label="Zoom out" @click="zoomBy(1 / 1.35)">−</button>
      <button class="ctl" title="Fit to view" aria-label="Fit to view" @click="fit">⤢</button>
      <button class="ctl" title="Re-run layout" aria-label="Re-run layout" @click="relayout">↻</button>
      <button
        class="ctl"
        :class="{ 'ctl--on': grouping }"
        :disabled="!canGroup"
        :title="groupTitle"
        :aria-label="groupTitle"
        :aria-pressed="grouping"
        @click="toggleClusters"
      >
        ⬡
      </button>
    </div>

    <div class="legend" aria-label="Legend">
      <span v-for="r in presentRoles" :key="r.id" class="lg">
        <i class="sw" :class="`sw--${r.swatch}`" :style="{ background: legendColor(r) }" />{{ r.label }}
      </span>
      <span v-if="hasSources" class="lg"><i class="sw sw--src" />Source model</span>
      <span class="lg"><i class="ln ln--cross" />Cross-domain join</span>
      <span v-if="grouping" class="lg"><i class="sw sw--cluster" />Domain cluster</span>
    </div>
  </div>
</template>

<style scoped>
.canvas-wrap {
  position: relative;
  width: 100%;
  height: 100%;
  background: var(--graph-bg);
  overflow: hidden;
}

/* The hull layer sits under the graph. Raising the Cytoscape container into
   its own stacking context keeps its internal canvases above this one. */
.hulls {
  position: absolute;
  inset: 0;
  z-index: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  transition: opacity 160ms var(--ease);
}

.canvas {
  position: relative;
  z-index: 1;
  width: 100%;
  height: 100%;
  transition: opacity 160ms var(--ease);
}
.canvas--hidden { opacity: 0; }

.overlay {
  position: absolute;
  inset: 0;
  z-index: 5;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  background: color-mix(in srgb, var(--graph-bg) 82%, transparent);
  color: var(--text-muted);
  font-size: 13px;
  pointer-events: none;
  text-align: center;
}
.overlay--quiet { background: transparent; }

.spinner {
  width: 22px;
  height: 22px;
  border: 2px solid var(--border-strong);
  border-top-color: var(--accent);
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }

.controls {
  position: absolute;
  top: 12px;
  right: 12px;
  z-index: 6;
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 4px;
  background: var(--panel);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: var(--shadow-sm);
}

.ctl {
  width: 28px;
  height: 28px;
  display: grid;
  place-items: center;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  font-size: 15px;
  line-height: 1;
  transition: background var(--dur) var(--ease), color var(--dur) var(--ease);
}
.ctl:hover { background: var(--bg-sunken); color: var(--text); }
.ctl--on {
  background: color-mix(in srgb, var(--accent) 14%, transparent);
  color: var(--accent);
}

.legend {
  position: absolute;
  left: 12px;
  bottom: 12px;
  z-index: 6;
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  padding: 7px 11px;
  background: color-mix(in srgb, var(--panel) 92%, transparent);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  font-size: 11px;
  color: var(--text-muted);
  box-shadow: var(--shadow-sm);
  max-width: calc(100% - 24px);
}

.lg { display: inline-flex; align-items: center; gap: 5px; white-space: nowrap; }

/* A 10px swatch cannot carry a heptagon, so the node shapes collapse to three
   silhouettes here. The colour is the part doing the identifying at this size;
   the silhouette only has to stop two adjacent entries reading as one. */
.sw { width: 10px; height: 10px; display: inline-block; }
.sw--square { border-radius: 2px; }
.sw--round { border-radius: 50%; }
.sw--angular { clip-path: polygon(50% 0, 100% 50%, 50% 100%, 0 50%); }
.sw--src {
  background: var(--source);
  opacity: 0.6;
  clip-path: polygon(25% 0, 75% 0, 100% 50%, 75% 100%, 25% 100%, 0 50%);
}
.sw--cluster {
  width: 13px;
  border: 1px solid var(--text-faint);
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--text-faint) 16%, transparent);
}
.ln { width: 16px; height: 2px; display: inline-block; }
.ln--cross { background: var(--edge-cross); }

@media (max-width: 720px) {
  .legend { display: none; }
}
</style>
