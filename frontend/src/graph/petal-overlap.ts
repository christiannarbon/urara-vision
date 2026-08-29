/**
 * Keeping the sakura cluster petals off each other.
 *
 * The compound layout is what keeps the plain hulls from intersecting: a hull
 * stays inside its compound's padding, and fcose never overlaps two compounds.
 * A petal does not inherit that. It circumscribes the same nodes far more
 * loosely -- a fitted petal covers roughly twice the area of the hull it holds
 * -- so it reaches well past the box that was keeping its neighbours away, and
 * two adjacent domains end up sharing ink.
 *
 * A petal cannot simply be shrunk, because it has to keep containing its own
 * nodes. So the only way to separate two of them is to move the nodes, and the
 * work here is to say by how much: `resolvePetalOverlaps` returns a translation
 * per cluster, and the caller moves each cluster as a unit. Translating a
 * cluster rigidly carries its petal along with it, so containment is preserved
 * for free and the layout *inside* a domain stays exactly as fcose solved it.
 */

import { petalContains, petalProfile, petalRing, translatePetal, type Petal, type Point } from './hull'

/**
 * Outline samples per petal used by the overlap test.
 *
 * The test asks whether either outline has a sampled point inside the other,
 * which can in principle miss a sliver of overlap that falls entirely between
 * two samples. The three corners are always exact vertices -- and a prong
 * poking into a neighbour is by far the likeliest way two petals meet -- so at
 * this density what escapes is thinner than the stroke drawn over it.
 */
const RING_SAMPLES = 160

/**
 * The profile's maximum, measured rather than restated from the fit constants.
 * A petal never reaches further from its centre than this times its radius.
 */
const PROFILE_MAX = (() => {
  let max = 0
  for (let i = 0; i < 720; i++) {
    max = Math.max(max, petalProfile((i / 720) * Math.PI * 2))
  }
  return max
})()

/** How far the petal reaches from its centre, at its furthest. */
function petalReach(pt: Petal): number {
  // The stretch can only magnify, so bounding by the larger factor is safe.
  return pt.r * PROFILE_MAX * Math.max(pt.sx, pt.sy)
}

/**
 * Grows a petal so the tests leave a visible gap between two outlines.
 *
 * The radius scales the whole profile and the profile is anisotropic, so what
 * this buys varies around the outline rather than being exactly `pad`
 * everywhere. It is a breathing-room setting, not a measured margin.
 */
function padPetal(pt: Petal, pad: number): Petal {
  return pad <= 0 ? pt : { ...pt, r: pt.r + pad }
}

/**
 * A petal with the parts of it the overlap test reads repeatedly worked out
 * once: its padded form, its outline samples, and its bounding radius.
 *
 * The separation search asks about the same two petals twenty times over while
 * sliding one of them along a line, so re-sampling both outlines each time is
 * most of the cost of the whole pass.
 */
interface Probe {
  petal: Petal
  ring: Point[]
  reach: number
}

function probe(pt: Petal, gap: number): Probe {
  const padded = padPetal(pt, gap / 2)
  return { petal: padded, ring: petalRing(padded, RING_SAMPLES), reach: petalReach(padded) }
}

/**
 * Whether two petals share any area, with `b` optionally shifted by (bx, by).
 *
 * Both outlines are star-shaped about their own centres, which is what makes
 * this cheap: "is this point inside that petal?" is one radius comparison, so
 * the test is a sweep of one outline's samples against the other. It is run
 * both ways round, which is also what catches one petal sitting wholly inside
 * another -- a case where neither boundary crosses the other.
 *
 * The shift is applied here rather than by rebuilding `b`, so sliding a petal
 * along a line costs one addition per sample instead of a fresh outline.
 */
function probesOverlap(a: Probe, b: Probe, bx = 0, by = 0): boolean {
  // Bounding circles first. In a real graph most pairs of domains are nowhere
  // near each other, and this rejects them in two multiplications.
  const span = Math.hypot(b.petal.cx + bx - a.petal.cx, b.petal.cy + by - a.petal.cy)
  if (span > a.reach + b.reach) return false

  const shifted = bx === 0 && by === 0 ? b.petal : translatePetal(b.petal, bx, by)
  for (const p of a.ring) {
    if (petalContains(shifted, p)) return true
  }
  for (const p of b.ring) {
    if (petalContains(a.petal, { x: p.x + bx, y: p.y + by })) return true
  }
  return false
}

/** Whether two petals share any area, or come within `gap` of doing so. */
export function petalsOverlap(a: Petal, b: Petal, gap = 0): boolean {
  return probesOverlap(probe(a, gap), probe(b, gap))
}

/**
 * The direction to push two overlapping petals apart along.
 *
 * The line between their centres, which for shapes this round is also close to
 * the shortest way out.
 */
function separationDirection(a: Petal, b: Petal, i: number, j: number): Point {
  const dx = b.cx - a.cx
  const dy = b.cy - a.cy
  const span = Math.hypot(dx, dy)
  if (span > 1e-9) return { x: dx / span, y: dy / span }

  // Concentric clusters have no centre line to push along. Fan them out by
  // index rather than picking at random, so the same graph always resolves the
  // same way.
  const angle = (((i * 97 + j * 43) % 360) * Math.PI) / 180
  return { x: Math.cos(angle), y: Math.sin(angle) }
}

/**
 * How far `b` has to travel along `dir` to clear `a`.
 *
 * Found by bisection rather than by solving the geometry: the petal's notch
 * makes the exact answer awkward, while "does it still overlap?" is cheap
 * enough to ask twenty times. `dir` points from a to b, so translating b along
 * it increases the distance between the centres by exactly the step, which is
 * what makes the upper bound below a real bracket.
 */
function clearingDistance(a: Probe, b: Probe, dir: Point): number {
  const span = Math.hypot(b.petal.cx - a.petal.cx, b.petal.cy - a.petal.cy)

  let hi = Math.max(a.reach + b.reach - span, 0) + 1
  // The bound comes from the bounding circles, so it always clears. Guard it
  // anyway: a bisection on an unbracketed interval fails silently.
  for (let tries = 0; tries < 8; tries++) {
    if (!probesOverlap(a, b, dir.x * hi, dir.y * hi)) break
    hi *= 2
  }

  let lo = 0
  for (let step = 0; step < 20; step++) {
    const mid = (lo + hi) / 2
    if (probesOverlap(a, b, dir.x * mid, dir.y * mid)) lo = mid
    else hi = mid
  }
  return hi
}

/** A cluster's translation, in the space its petal was fitted in. */
export interface Offset {
  dx: number
  dy: number
}

/**
 * Passes over the pairs. Each pass moves every overlapping pair halfway out of
 * each other's way, so a crowded run needs several before it settles; the loop
 * stops as soon as a pass finds nothing to do.
 */
const MAX_PASSES = 40

/**
 * Translations that leave no two petals overlapping.
 *
 * Petals that were already clear of their neighbours get a zero offset, so a
 * layout with room to spare is left exactly where the layout engine put it and
 * only crowded neighbourhoods are opened up. Each push is split evenly between
 * the two clusters involved, so no one domain gets singled out and shoved
 * across the canvas.
 *
 * Nothing here is random and nothing reads the clock, so the same input always
 * gives the same answer -- re-running it on an unchanged layout is a no-op.
 */
export function resolvePetalOverlaps(petals: Petal[], gap = 0): Offset[] {
  const offsets: Offset[] = petals.map(() => ({ dx: 0, dy: 0 }))
  if (petals.length < 2) return offsets

  // Padding and sampling depend only on the shape, never on where it sits, so
  // the probes are built once and re-centred as the clusters move.
  const probes = petals.map((p) => probe(p, gap))
  const placed = (i: number): Probe => {
    const { dx, dy } = offsets[i]
    if (dx === 0 && dy === 0) return probes[i]
    const moved = translatePetal(probes[i].petal, dx, dy)
    return {
      petal: moved,
      ring: probes[i].ring.map((p) => ({ x: p.x + dx, y: p.y + dy })),
      reach: probes[i].reach,
    }
  }

  for (let pass = 0; pass < MAX_PASSES; pass++) {
    let moved = false

    for (let i = 0; i < petals.length; i++) {
      for (let j = i + 1; j < petals.length; j++) {
        const a = placed(i)
        const b = placed(j)
        if (!probesOverlap(a, b)) continue

        const dir = separationDirection(a.petal, b.petal, i, j)
        const push = clearingDistance(a, b, dir)
        if (push <= 0) continue

        offsets[i].dx -= (dir.x * push) / 2
        offsets[i].dy -= (dir.y * push) / 2
        offsets[j].dx += (dir.x * push) / 2
        offsets[j].dy += (dir.y * push) / 2
        moved = true
      }
    }

    if (!moved) break
  }
  return offsets
}
