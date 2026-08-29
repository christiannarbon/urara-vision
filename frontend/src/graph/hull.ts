/**
 * Convex hull geometry for the cluster outlines.
 *
 * Everything here works in screen coordinates, where y grows downwards, so
 * "clockwise" below means clockwise as drawn rather than as a mathematician
 * would have it.
 */

export interface Point {
  x: number
  y: number
}

function cross(o: Point, a: Point, b: Point): number {
  return (a.x - o.x) * (b.y - o.y) - (a.y - o.y) * (b.x - o.x)
}

/** Twice the signed area; positive when the ring is clockwise on screen. */
function signedArea(ring: Point[]): number {
  let sum = 0
  for (let i = 0; i < ring.length; i++) {
    const p = ring[i]
    const q = ring[(i + 1) % ring.length]
    sum += p.x * q.y - q.x * p.y
  }
  return sum
}

/**
 * Andrew's monotone chain. Collinear points are dropped, so a row of nodes
 * collapses to its two endpoints and still inflates into a clean stadium.
 * The result is ordered clockwise on screen.
 */
export function convexHull(points: Point[]): Point[] {
  if (points.length <= 2) return points.slice()

  const pts = [...points].sort((a, b) => a.x - b.x || a.y - b.y)

  const lower: Point[] = []
  for (const p of pts) {
    while (lower.length >= 2 && cross(lower[lower.length - 2], lower[lower.length - 1], p) <= 0) {
      lower.pop()
    }
    lower.push(p)
  }

  const upper: Point[] = []
  for (let i = pts.length - 1; i >= 0; i--) {
    const p = pts[i]
    while (upper.length >= 2 && cross(upper[upper.length - 2], upper[upper.length - 1], p) <= 0) {
      upper.pop()
    }
    upper.push(p)
  }

  lower.pop()
  upper.pop()
  const ring = lower.concat(upper)
  if (ring.length > 2 && signedArea(ring) < 0) ring.reverse()
  return ring
}

/**
 * Traces the hull expanded outwards by `pad`, with the corners rounded to that
 * same radius. Offsetting a convex ring needs no clipping: each edge shifts
 * along its outward normal and each vertex becomes an arc joining the two
 * normals, which is one uninterrupted canvas path.
 *
 * The path is left for the caller to fill and stroke.
 */
export function traceInflatedHull(ctx: CanvasRenderingContext2D, ring: Point[], pad: number) {
  ctx.beginPath()
  if (ring.length === 0) return
  if (ring.length === 1) {
    ctx.arc(ring[0].x, ring[0].y, pad, 0, Math.PI * 2)
    return
  }

  // Outward normal angle of the edge leaving each vertex. For a clockwise ring
  // the outward normal of (dx, dy) is (dy, -dx).
  const n = ring.length
  const angles: number[] = []
  for (let i = 0; i < n; i++) {
    const p = ring[i]
    const q = ring[(i + 1) % n]
    angles.push(Math.atan2(-(q.x - p.x), q.y - p.y))
  }

  for (let i = 0; i < n; i++) {
    // arc() draws the straight offset edge as an implicit lineTo from the
    // previous arc's end point, so the ring closes up on its own.
    ctx.arc(ring[i].x, ring[i].y, pad, angles[(i - 1 + n) % n], angles[i])
  }
  ctx.closePath()
}

/** Axis-aligned bounds of a ring, before any padding is added. */
export function ringBounds(ring: Point[]) {
  let x1 = Infinity
  let y1 = Infinity
  let x2 = -Infinity
  let y2 = -Infinity
  for (const p of ring) {
    if (p.x < x1) x1 = p.x
    if (p.y < y1) y1 = p.y
    if (p.x > x2) x2 = p.x
    if (p.y > y2) y2 = p.y
  }
  return { x1, y1, x2, y2 }
}

/**
 * Y of the ring's upper boundary directly above `x`. A caption anchored to the
 * ring's highest corner floats free of a sloped top edge; anchoring it here
 * seats it on the outline instead.
 */
export function ringTopAt(ring: Point[], x: number): number {
  if (ring.length === 0) return 0
  if (ring.length === 1) return ring[0].y

  let top = Infinity
  for (let i = 0; i < ring.length; i++) {
    const p = ring[i]
    const q = ring[(i + 1) % ring.length]
    if (p.x === q.x) {
      if (x === p.x) top = Math.min(top, p.y, q.y)
      continue
    }
    if (x < Math.min(p.x, q.x) || x > Math.max(p.x, q.x)) continue
    const y = p.y + ((x - p.x) / (q.x - p.x)) * (q.y - p.y)
    if (y < top) top = y
  }
  return Number.isFinite(top) ? top : ringBounds(ring).y1
}

// --- sakura petal ----------------------------------------------------------

/**
 * For the Haru Urara theme a cluster is not outlined by its hull at all: it is
 * enclosed by a single sakura petal -- the petal being the shape of her eyes.
 *
 * The petal is defined radially, as a tip radius times an angular profile,
 * which is what makes it usable as a hull. A radial outline is star-shaped
 * about its centre, so "is this node inside it?" is one comparison per point,
 * and the smallest petal that holds a cluster can be solved for directly
 * rather than searched for.
 *
 * Every length below is a share of the radius out to a tip, and the profile's
 * phase is measured from the notch: phase 0 points into the cleft between the
 * two prongs, and the outline sweeps from there round to the base at pi.
 */

/**
 * The outer edge, away from the notch: a superellipse about the polar centre,
 * with its own reach and exponent above and below the waist. The exponents are
 * what taper the shoulders -- an ellipse is far too round to read as a petal --
 * and the two halves meet at the waist with the same radius and a flat tangent,
 * so the seam between them does not show.
 *
 * The numbers are a least-squares fit of this form to a drawn sakura petal.
 */
const PETAL_TIP_REACH = 1.2023
const PETAL_TIP_TAPER = 1.403
const PETAL_BASE_REACH = 0.8192
const PETAL_BASE_TAPER = 1.641
/** Half-width at the waist, which is where the petal is widest. */
const PETAL_WAIST = 0.6317
/**
 * The notch, cut by a straight chord on each side running from the valley out
 * to a tip. Straight is the point: both prongs then come to a point and the
 * valley to a sharp V, which is the whole silhouette of a sakura petal, where
 * a rounded dip reads as a cloud. `PETAL_TIP` is where a tip sits, in radians
 * off the notch axis, and `PETAL_VALLEY` how far the valley is from the centre
 * -- the profile's minimum, and so what decides how much room a petal costs.
 */
const PETAL_TIP = 0.2442
const PETAL_VALLEY = 0.5188

/** The outer edge at profile phase `t`, with the notch ignored. */
function petalOutline(t: number): number {
  const c = Math.cos(t)
  const s = Math.sin(t)
  const reach = c >= 0 ? PETAL_TIP_REACH : PETAL_BASE_REACH
  const taper = c >= 0 ? PETAL_TIP_TAPER : PETAL_BASE_TAPER
  return (Math.abs(c / reach) ** taper + Math.abs(s / PETAL_WAIST) ** taper) ** (-1 / taper)
}

/**
 * Petal radius, as a share of the tip radius, at profile phase `t` radians.
 *
 * Exported because it is the whole definition of the shape: anything that has
 * to reason about a petal rather than just draw one -- whether a point is
 * inside it, whether two of them overlap -- reads it from here rather than
 * restating the constants above.
 */
export function petalProfile(t: number): number {
  let p = t % (Math.PI * 2)
  if (p > Math.PI) p -= Math.PI * 2
  else if (p < -Math.PI) p += Math.PI * 2
  const off = Math.abs(p)
  if (off >= PETAL_TIP) return petalOutline(p)
  // The chord through the valley at (PETAL_VALLEY, 0) and the tip at
  // (1, PETAL_TIP), written as a radius per angle.
  return Math.sin(PETAL_TIP) / (Math.sin(PETAL_TIP - off) / PETAL_VALLEY + Math.sin(off))
}

/**
 * The profile, tabulated, for the fit to read. The fit evaluates it tens of
 * thousands of times per cluster and re-runs whenever a node moves, and each
 * evaluation costs three pow calls, so it is worth sampling once up front.
 *
 * Each slot holds the *smallest* radius across its slice rather than a sample
 * from somewhere inside it. That makes every radius the fit reads an
 * under-estimate of the profile, and an under-estimate of the profile is an
 * over-estimate of the petal a cluster needs, so the table can only ever fit a
 * cluster loosely -- never crop a node out of one.
 */
const PETAL_SLOTS = 2048
/**
 * Pieces each slot is cut into before its minimum is taken. The profile's
 * kinks are minima, so a slot holding one has to be looked inside.
 */
const PETAL_SUBSAMPLES = 4
const PETAL_TABLE = buildPetalTable()

function buildPetalTable(): Float64Array {
  const table = new Float64Array(PETAL_SLOTS)
  const slot = (Math.PI * 2) / PETAL_SLOTS
  for (let i = 0; i < PETAL_SLOTS; i++) {
    let low = Infinity
    for (let k = 0; k <= PETAL_SUBSAMPLES; k++) {
      const v = petalProfile((i + k / PETAL_SUBSAMPLES) * slot)
      if (v < low) low = v
    }
    table[i] = low
  }
  return table
}

/** The tabulated profile at phase `t`. See PETAL_TABLE. */
function petalProfileAt(t: number): number {
  let i = Math.floor((t * PETAL_SLOTS) / (Math.PI * 2)) % PETAL_SLOTS
  if (i < 0) i += PETAL_SLOTS
  return PETAL_TABLE[i]
}

/**
 * How far a petal is stretched to follow a long cluster before it gives up and
 * keeps its shape. A petal is already half again as long as it is wide and the
 * fit is free to turn it, so a cluster up to about that shape is followed for
 * nothing, and the stretch is only for the ones past it.
 *
 * It is capped low, and the cap is what decides the look: the fit costs a
 * petal by area, and stretching one always wins on area, so left to itself the
 * fit pulls every petal out to whatever it is allowed. Past about here the
 * silhouette stops reading as a petal, and the room bought is small -- lifting
 * the cap to 2.2 takes the median petal from 2.3 times the area of the hull it
 * holds down to 2.2, which is not worth the shape.
 */
const PETAL_ASPECT = 1.5
/** Stretches tried between those bounds, log-spaced. */
const PETAL_STRETCHES = 7
/**
 * Rotations tried when fitting. A petal has no symmetry to exploit, so this
 * has to cover the whole turn, and it is worth more here than it was to a
 * five-fold blossom: turning the petal is most of how it follows a cluster.
 */
const PETAL_TURNS = 32
/** Directions the fit is checked in, on top of the exact ones. */
const PETAL_GRID = 288
/**
 * Slack on the fitted radius. The direction that matters most is tested
 * exactly, so this only has to cover the smooth stretches between grid
 * directions.
 */
const PETAL_SLACK = 1.005

export interface Petal {
  /** Centre, in whatever space the ring was given in. */
  cx: number
  cy: number
  /** Axis stretch. Both are at least one, so padding never shrinks under it. */
  sx: number
  sy: number
  /** Radius out to a tip, before the stretch. */
  r: number
  /** Where the notch points, in radians. */
  rot: number
}

/**
 * How far the ring's boundary lies from `c` along the ray at `dir`, or -1 if
 * the ray misses it -- which only happens when `c` is outside the ring, since
 * the ring is convex. Every crossing is considered and the furthest kept, so
 * grazing a vertex cannot report the near side of the ring.
 */
function ringRadius(ring: Point[], cx: number, cy: number, dx: number, dy: number): number {
  let far = -1
  for (let i = 0; i < ring.length; i++) {
    const p = ring[i]
    const q = ring[(i + 1) % ring.length]
    const ex = q.x - p.x
    const ey = q.y - p.y
    const wx = p.x - cx
    const wy = p.y - cy
    const denom = ex * dy - ey * dx
    if (denom === 0) continue
    const s = -(wx * dy - wy * dx) / denom
    if (s < 0 || s > 1) continue
    const t = (wx + s * ex) * dx + (wy + s * ey) * dy
    if (t > far) far = t
  }
  return far
}

/**
 * Smallest petal that holds `ring` with at least `pad` clearance, over the
 * rotations tried.
 *
 * The fit is per direction rather than per point. A radial outline is a radius
 * for every angle, and the ring is convex with its centre inside, so the ring
 * is inside the petal exactly when, in every direction, the petal's radius
 * clears the ring's -- which makes the smallest petal a maximum over angles of
 * (how far the ring reaches, plus the padding) over (what share of the tip
 * radius the profile gives that angle).
 *
 * Testing directions rather than the ring's own points is what keeps a node
 * out of the notch: the profile has a sharp minimum in the valley, so a point
 * sampled a hair off it reads a radius that is far too generous, and that is a
 * node hanging out of the cleft. The valley direction, and the directions of
 * the ring's corners, are therefore all tested exactly; the grid between them
 * covers the smooth stretches, where a missed maximum costs a fraction of a
 * percent, and PETAL_SLACK pays for it.
 *
 * The rotation matters -- a corner pointing into the valley costs much more
 * than one pointing along a prong -- so the fit is run at each rotation and
 * the tightest kept, which also gives every cluster its own turn of the petal
 * for free. The stretch is searched the same way, on area rather than radius,
 * so a petal is only drawn out of shape when doing so buys back the room.
 *
 * Work in model space and fit once per layout: the result only needs scaling
 * to paint, so panning and zooming never re-solve it.
 */
export function fitPetal(ring: Point[], pad: number): Petal {
  const b = ringBounds(ring)
  const cx = (b.x1 + b.x2) / 2
  const cy = (b.y1 + b.y2) / 2

  let best: Petal | null = null
  let bestCost = Infinity
  for (let i = 0; i < PETAL_STRETCHES; i++) {
    // Log-spaced, so wide and tall are tried alike, and 1:1 is one of them.
    const t = i / (PETAL_STRETCHES - 1)
    const aspect = Math.exp(Math.log(1 / PETAL_ASPECT) * (1 - t) + Math.log(PETAL_ASPECT) * t)
    // Both factors are kept at or above one, so a model unit of padding never
    // shrinks under the mapping and the fit can treat `pad` as it is given.
    const sx = Math.max(1, aspect)
    const sy = Math.max(1, 1 / aspect)
    const fit = fitAt(ring, cx, cy, sx, sy, pad)
    // Area, so a stretch is only taken when it buys back more than it spends.
    const cost = fit.r * fit.r * sx * sy
    if (cost < bestCost) {
      bestCost = cost
      best = { cx, cy, sx, sy, r: fit.r, rot: fit.rot }
    }
  }
  return best as Petal
}

/** The petal fit at one stretch: the tightest radius, and where its notch sits. */
function fitAt(
  ring: Point[],
  cx: number,
  cy: number,
  sx: number,
  sy: number,
  pad: number,
): { r: number; rot: number } {
  // The stretch is undone on the ring instead of applied to the petal, so the
  // fit is a plain unstretched one from here on.
  const flat = ring.map((p) => ({ x: (p.x - cx) / sx, y: (p.y - cy) / sy }))
  let far = 0
  for (const p of flat) far = Math.max(far, Math.hypot(p.x, p.y))
  if (far === 0) return { r: pad / PETAL_VALLEY, rot: 0 }

  const fixed: number[] = []
  for (const p of flat) fixed.push(Math.atan2(p.y, p.x))
  for (let k = 0; k < PETAL_GRID; k++) fixed.push((k / PETAL_GRID) * Math.PI * 2)
  // Radii in the directions every rotation shares, measured once.
  const reach = fixed.map((a) => {
    const t = ringRadius(flat, 0, 0, Math.cos(a), Math.sin(a))
    return t < 0 ? far : t
  })

  let bestRot = 0
  let bestR = Infinity
  for (let k = 0; k < PETAL_TURNS; k++) {
    const rot = (k / PETAL_TURNS) * Math.PI * 2
    let r = 0
    for (let i = 0; i < fixed.length; i++) {
      const need = (reach[i] + pad) / petalProfileAt(fixed[i] - rot)
      if (need > r) r = need
    }
    // The valley, where the profile is at its lowest and kinked, is the one
    // direction a fit is most easily wrong about. It moves with the rotation,
    // so it is measured per rotation.
    const t = ringRadius(flat, 0, 0, Math.cos(rot), Math.sin(rot))
    const need = ((t < 0 ? far : t) + pad) / PETAL_VALLEY
    if (need > r) r = need
    if (r < bestR) {
      bestR = r
      bestRot = rot
    }
  }

  return { r: bestR * PETAL_SLACK, rot: bestRot }
}

/** The petal scaled and panned into screen space, ready to trace. */
export function placePetal(pt: Petal, zoom: number, panX: number, panY: number): Petal {
  return {
    cx: pt.cx * zoom + panX,
    cy: pt.cy * zoom + panY,
    sx: pt.sx,
    sy: pt.sy,
    r: pt.r * zoom,
    rot: pt.rot,
  }
}

/**
 * The petal's outline as a polygon, in whatever space the petal is given in.
 *
 * The two tips and the valley are the whole silhouette, so they are placed as
 * exact vertices and only the three stretches between them are sampled: a
 * sample that straddles one of the three corners rounds it off. The two notch
 * stretches are straight chords, which two points describe exactly.
 *
 * `steps` is the sample count for the full turn; each stretch gets its share.
 */
export function petalRing(pt: Petal, steps: number): Point[] {
  const corners = [0, PETAL_TIP, Math.PI * 2 - PETAL_TIP, Math.PI * 2]
  const ring: Point[] = []

  for (let seg = 0; seg < 3; seg++) {
    const a0 = corners[seg]
    const a1 = corners[seg + 1]
    const n = Math.max(2, Math.round((steps * (a1 - a0)) / (Math.PI * 2)))
    for (let i = 0; i < n; i++) {
      const t = a0 + ((a1 - a0) * i) / n
      const f = petalProfile(t) * pt.r
      ring.push({
        x: pt.cx + Math.cos(t + pt.rot) * f * pt.sx,
        y: pt.cy + Math.sin(t + pt.rot) * f * pt.sy,
      })
    }
  }
  return ring
}

/** Traces the petal. The path is left for the caller to fill and stroke. */
export function tracePetal(ctx: CanvasRenderingContext2D, pt: Petal) {
  const reach = pt.r * Math.max(pt.sx, pt.sy)
  const total = Math.max(120, Math.min(2000, Math.ceil((Math.PI * 2 * reach) / 3)))

  ctx.beginPath()
  const ring = petalRing(pt, total)
  for (let i = 0; i < ring.length; i++) {
    if (i === 0) ctx.moveTo(ring[i].x, ring[i].y)
    else ctx.lineTo(ring[i].x, ring[i].y)
  }
  ctx.closePath()
}

/**
 * Whether a point lies inside the petal.
 *
 * Undoing the stretch and the rotation puts the petal back in the frame where
 * it is a radius per angle, and a radial outline is star-shaped about its
 * centre, so containment is one comparison rather than a polygon crossing
 * count. This is the same fact the fit is built on -- see fitPetal.
 */
export function petalContains(pt: Petal, p: Point): boolean {
  const dx = (p.x - pt.cx) / pt.sx
  const dy = (p.y - pt.cy) / pt.sy
  const r = Math.hypot(dx, dy)
  if (r === 0) return true
  return r <= petalProfile(Math.atan2(dy, dx) - pt.rot) * pt.r
}

/** The same petal centred somewhere else. Shape and rotation are untouched. */
export function translatePetal(pt: Petal, dx: number, dy: number): Petal {
  return { ...pt, cx: pt.cx + dx, cy: pt.cy + dy }
}

/**
 * Y of the outline directly above the centre. Straight up from the centre is
 * one ray, and the outline is star-shaped about it, so this is exact rather
 * than a search: a caption seated here sits on the petal.
 */
export function petalTop(pt: Petal): number {
  return pt.cy - petalProfile(-Math.PI / 2 - pt.rot) * pt.r * pt.sy
}
