/** Keeping the sakura cluster petals off each other. */

import { petalContains, petalProfile, petalRing, translatePetal, type Petal, type Point } from './hull'

/** Outline samples per petal used by the overlap test. */
const RING_SAMPLES = 160

/** The profile's maximum, measured rather than restated from the fit constants. */
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

/** Grows a petal so the tests leave a visible gap between two outlines. */
function padPetal(pt: Petal, pad: number): Petal {
  return pad <= 0 ? pt : { ...pt, r: pt.r + pad }
}

/**
 * A petal with the parts of it the overlap test reads repeatedly worked out once: its padded form,…
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

/** Whether two petals share any area, with `b` optionally shifted by (bx, by). */
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

/** The direction to push two overlapping petals apart along. */
function separationDirection(a: Petal, b: Petal, i: number, j: number): Point {
  const dx = b.cx - a.cx
  const dy = b.cy - a.cy
  const span = Math.hypot(dx, dy)
  if (span > 1e-9) return { x: dx / span, y: dy / span }

  // Concentric clusters have no centre line to push along.
  const angle = (((i * 97 + j * 43) % 360) * Math.PI) / 180
  return { x: Math.cos(angle), y: Math.sin(angle) }
}

/** How far `b` has to travel along `dir` to clear `a`. */
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

/** Passes over the pairs. */
const MAX_PASSES = 40

/** Translations that leave no two petals overlapping. */
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
