/**
 * Convex hull and petal geometry.
 *
 * These are the two pieces of pure maths in the app, and both have properties
 * worth asserting directly: a hull must actually enclose its points, and a
 * fitted petal must actually contain the hull. A cluster outline that crops a
 * node out is a visible bug that is hard to spot by eye.
 */

import { describe, expect, it } from 'vitest'

import {
  convexHull,
  fitPetal,
  petalContains,
  petalTop,
  placePetal,
  ringBounds,
  ringTopAt,
} from '../../src/graph/hull'
import type { Point } from '../../src/graph/hull'

/** Signed area doubled; positive means clockwise in screen coordinates. */
function signedArea(ring: Point[]): number {
  let sum = 0
  for (let i = 0; i < ring.length; i++) {
    const p = ring[i]
    const q = ring[(i + 1) % ring.length]
    sum += p.x * q.y - q.x * p.y
  }
  return sum
}

/** Whether p is inside or on the convex ring, allowing for float slop. */
function inside(ring: Point[], p: Point, eps = 1e-9): boolean {
  for (let i = 0; i < ring.length; i++) {
    const a = ring[i]
    const b = ring[(i + 1) % ring.length]
    // Clockwise ring: an interior point is to the right of every edge.
    const cross = (b.x - a.x) * (p.y - a.y) - (b.y - a.y) * (p.x - a.x)
    if (cross < -eps) return false
  }
  return true
}

describe('convexHull', () => {
  it('returns degenerate inputs unchanged', () => {
    expect(convexHull([])).toEqual([])
    const one = [{ x: 1, y: 2 }]
    expect(convexHull(one)).toEqual(one)
    const two = [
      { x: 0, y: 0 },
      { x: 5, y: 5 },
    ]
    expect(convexHull(two)).toHaveLength(2)
  })

  it('does not mutate its input', () => {
    const pts = [
      { x: 3, y: 1 },
      { x: 0, y: 0 },
      { x: 1, y: 2 },
    ]
    const copy = pts.map((p) => ({ ...p }))
    convexHull(pts)
    expect(pts).toEqual(copy)
  })

  it('keeps only the corners of a square, dropping the interior point', () => {
    const ring = convexHull([
      { x: 0, y: 0 },
      { x: 10, y: 0 },
      { x: 10, y: 10 },
      { x: 0, y: 10 },
      { x: 5, y: 5 },
    ])
    expect(ring).toHaveLength(4)
    expect(ring).not.toContainEqual({ x: 5, y: 5 })
  })

  it('collapses a collinear row to its two endpoints', () => {
    // A row of nodes has no area; keeping the midpoints would leave the
    // inflated outline with a kink in the middle of a straight edge.
    const ring = convexHull([
      { x: 0, y: 0 },
      { x: 5, y: 0 },
      { x: 10, y: 0 },
      { x: 15, y: 0 },
    ])
    expect(ring).toHaveLength(2)
    expect(ring.map((p) => p.x).sort((a, b) => a - b)).toEqual([0, 15])
  })

  it('orders the ring clockwise on screen', () => {
    const ring = convexHull([
      { x: 0, y: 0 },
      { x: 10, y: 0 },
      { x: 10, y: 10 },
      { x: 0, y: 10 },
    ])
    expect(signedArea(ring)).toBeGreaterThan(0)
  })

  it('encloses every input point', () => {
    // A fixed pseudo-random cloud: deterministic, but not a shape chosen to
    // flatter the algorithm.
    const pts: Point[] = []
    let seed = 12345
    for (let i = 0; i < 200; i++) {
      seed = (seed * 1103515245 + 12345) % 2147483648
      const x = (seed / 2147483648) * 500
      seed = (seed * 1103515245 + 12345) % 2147483648
      const y = (seed / 2147483648) * 300
      pts.push({ x, y })
    }
    const ring = convexHull(pts)
    expect(ring.length).toBeGreaterThan(2)
    for (const p of pts) {
      expect(inside(ring, p, 1e-6)).toBe(true)
    }
  })
})

describe('ringBounds', () => {
  it('reports the axis-aligned extent', () => {
    expect(
      ringBounds([
        { x: 2, y: 7 },
        { x: -3, y: 1 },
        { x: 5, y: 4 },
      ]),
    ).toEqual({ x1: -3, y1: 1, x2: 5, y2: 7 })
  })
})

describe('ringTopAt', () => {
  it('follows a sloped top edge rather than the highest corner', () => {
    // A triangle with its apex on the left: a caption anchored at the apex's y
    // would float above the outline everywhere else.
    const ring = [
      { x: 0, y: 0 },
      { x: 10, y: 10 },
      { x: 0, y: 10 },
    ]
    expect(ringTopAt(ring, 0)).toBe(0)
    expect(ringTopAt(ring, 5)).toBeCloseTo(5, 6)
    expect(ringTopAt(ring, 10)).toBeCloseTo(10, 6)
  })

  it('handles degenerate rings', () => {
    expect(ringTopAt([], 0)).toBe(0)
    expect(ringTopAt([{ x: 3, y: 9 }], 3)).toBe(9)
  })

  it('falls back to the top bound off the ring', () => {
    const ring = [
      { x: 0, y: 5 },
      { x: 10, y: 5 },
      { x: 10, y: 15 },
      { x: 0, y: 15 },
    ]
    expect(ringTopAt(ring, 99)).toBe(5)
  })
})

describe('fitPetal', () => {
  it('contains every point of the ring it was fitted to', () => {
    const ring = convexHull([
      { x: 0, y: 0 },
      { x: 120, y: 10 },
      { x: 140, y: 90 },
      { x: 30, y: 110 },
    ])
    const petal = fitPetal(ring, 12)
    for (const p of ring) {
      expect(petalContains(petal, p)).toBe(true)
    }
  })

  it('clears the ring by at least the padding it was given', () => {
    const ring = convexHull([
      { x: 0, y: 0 },
      { x: 60, y: 0 },
      { x: 60, y: 40 },
      { x: 0, y: 40 },
    ])
    const tight = fitPetal(ring, 0)
    const padded = fitPetal(ring, 40)
    // More padding can only mean a larger petal.
    expect(padded.r).toBeGreaterThan(tight.r)
  })

  it('handles a single-node cluster', () => {
    const petal = fitPetal([{ x: 50, y: 50 }], 10)
    expect(petal.r).toBeGreaterThan(0)
    expect(Number.isFinite(petal.r)).toBe(true)
    expect(petal.cx).toBe(50)
    expect(petal.cy).toBe(50)
  })

  it('never shrinks either axis below one, so padding keeps its meaning', () => {
    const petal = fitPetal(
      convexHull([
        { x: 0, y: 0 },
        { x: 400, y: 0 },
        { x: 400, y: 20 },
        { x: 0, y: 20 },
      ]),
      8,
    )
    expect(petal.sx).toBeGreaterThanOrEqual(1)
    expect(petal.sy).toBeGreaterThanOrEqual(1)
  })

  it('is deterministic, so a re-render does not reshape a cluster', () => {
    const ring = convexHull([
      { x: 5, y: 5 },
      { x: 90, y: 20 },
      { x: 70, y: 95 },
    ])
    expect(fitPetal(ring, 14)).toEqual(fitPetal(ring, 14))
  })
})

describe('placePetal', () => {
  it('scales and pans without touching the shape', () => {
    const petal = fitPetal(
      convexHull([
        { x: 0, y: 0 },
        { x: 50, y: 0 },
        { x: 50, y: 50 },
      ]),
      6,
    )
    const placed = placePetal(petal, 2, 100, 200)
    expect(placed.cx).toBe(petal.cx * 2 + 100)
    expect(placed.cy).toBe(petal.cy * 2 + 200)
    expect(placed.r).toBe(petal.r * 2)
    // Stretch and rotation are shape, not placement.
    expect(placed.sx).toBe(petal.sx)
    expect(placed.sy).toBe(petal.sy)
    expect(placed.rot).toBe(petal.rot)
  })
})

describe('petalTop', () => {
  it('sits above the centre', () => {
    const petal = fitPetal(
      convexHull([
        { x: 0, y: 0 },
        { x: 40, y: 0 },
        { x: 40, y: 40 },
        { x: 0, y: 40 },
      ]),
      10,
    )
    expect(petalTop(petal)).toBeLessThan(petal.cy)
  })
})
