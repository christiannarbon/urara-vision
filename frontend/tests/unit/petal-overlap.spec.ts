/**
 * Keeping the cluster petals off each other.
 *
 * The property that matters is the one in the name: after the offsets are
 * applied, no two petals overlap. That is asserted directly rather than
 * inferred from the numbers, so the tests hold whatever the search inside
 * changes to.
 *
 * The second property matters almost as much: a layout that was already clear
 * must come back untouched, or every re-render would drift the graph apart.
 */

import { describe, expect, it } from 'vitest'

import { convexHull, fitPetal, petalContains, translatePetal, type Petal, type Point } from '../../src/graph/hull'
import { petalsOverlap, resolvePetalOverlaps } from '../../src/graph/petal-overlap'

/** A petal fitted around a square cluster of the given size, at (cx, cy). */
function cluster(cx: number, cy: number, size = 60, pad = 8): Petal {
  const half = size / 2
  const ring = convexHull([
    { x: cx - half, y: cy - half },
    { x: cx + half, y: cy - half },
    { x: cx + half, y: cy + half },
    { x: cx - half, y: cy + half },
  ])
  return fitPetal(ring, pad)
}

/** Applies the offsets, the way the canvas moves each cluster's nodes. */
function applied(petals: Petal[], gap = 0): Petal[] {
  const offsets = resolvePetalOverlaps(petals, gap)
  return petals.map((p, i) => translatePetal(p, offsets[i].dx, offsets[i].dy))
}

/** Every unordered pair, for asserting over a whole arrangement. */
function pairs<T>(items: T[]): Array<[T, T, number, number]> {
  const out: Array<[T, T, number, number]> = []
  for (let i = 0; i < items.length; i++) {
    for (let j = i + 1; j < items.length; j++) out.push([items[i], items[j], i, j])
  }
  return out
}

function expectAllClear(petals: Petal[], gap = 0) {
  for (const [a, b, i, j] of pairs(petals)) {
    expect(petalsOverlap(a, b, gap), `petals ${i} and ${j} still overlap`).toBe(false)
  }
}

describe('petalContains', () => {
  it('holds its own centre', () => {
    const p = cluster(0, 0)
    expect(petalContains(p, { x: 0, y: 0 })).toBe(true)
  })

  it('excludes a point far outside', () => {
    const p = cluster(0, 0)
    expect(petalContains(p, { x: 10_000, y: 10_000 })).toBe(false)
  })

  it('holds every node of the cluster it was fitted to', () => {
    const ring = convexHull([
      { x: 0, y: 0 },
      { x: 120, y: 10 },
      { x: 140, y: 90 },
      { x: 30, y: 110 },
    ])
    const p = fitPetal(ring, 12)
    for (const node of ring) {
      expect(petalContains(p, node)).toBe(true)
    }
  })

  it('excludes the cleft between the two prongs', () => {
    // The notch is the whole reason the shape reads as a petal, so a
    // containment test that fills it in would be wrong in the way that matters.
    const p = cluster(0, 0)
    // Straight out along the notch axis, past the valley but well within the
    // radius a tip reaches.
    const justPastValley = p.r * 0.9
    const x = p.cx + Math.cos(p.rot) * justPastValley * p.sx
    const y = p.cy + Math.sin(p.rot) * justPastValley * p.sy
    expect(petalContains(p, { x, y })).toBe(false)
  })
})

describe('petalsOverlap', () => {
  it('is true for a petal against itself', () => {
    const p = cluster(0, 0)
    expect(petalsOverlap(p, p)).toBe(true)
  })

  it('is false for two petals far apart', () => {
    expect(petalsOverlap(cluster(0, 0), cluster(5000, 5000))).toBe(false)
  })

  it('is true when one petal sits wholly inside another', () => {
    // Neither boundary crosses the other here, which is exactly the case a
    // one-directional test would miss.
    const big = cluster(0, 0, 400)
    const small = cluster(0, 0, 10)
    expect(petalsOverlap(big, small)).toBe(true)
    expect(petalsOverlap(small, big)).toBe(true)
  })

  it('is symmetric', () => {
    const a = cluster(0, 0, 80)
    const b = cluster(90, 20, 80)
    expect(petalsOverlap(a, b)).toBe(petalsOverlap(b, a))
  })

  it('reports a gap violation between petals that are clear but close', () => {
    const a = cluster(0, 0, 60)
    const b = cluster(0, 0, 60)
    // Placed just clear of each other, then asked for room they do not have.
    const [ca, cb] = applied([a, b])
    expect(petalsOverlap(ca, cb)).toBe(false)
    expect(petalsOverlap(ca, cb, 200)).toBe(true)
  })
})

describe('resolvePetalOverlaps', () => {
  it('returns nothing to do for fewer than two petals', () => {
    expect(resolvePetalOverlaps([])).toEqual([])
    expect(resolvePetalOverlaps([cluster(0, 0)])).toEqual([{ dx: 0, dy: 0 }])
  })

  it('leaves an already-clear arrangement exactly where it was', () => {
    // The important half of the contract: a graph with room to spare must not
    // drift every time this runs.
    const petals = [cluster(0, 0, 40), cluster(3000, 0, 40), cluster(0, 3000, 40)]
    expect(resolvePetalOverlaps(petals, 20)).toEqual([
      { dx: 0, dy: 0 },
      { dx: 0, dy: 0 },
      { dx: 0, dy: 0 },
    ])
  })

  it('separates two overlapping petals', () => {
    const petals = [cluster(0, 0, 80), cluster(40, 15, 80)]
    expect(petalsOverlap(petals[0], petals[1])).toBe(true)
    expectAllClear(applied(petals))
  })

  it('separates two petals sharing a centre', () => {
    // No centre line to push along, so this exercises the fallback direction.
    const petals = [cluster(100, 100, 70), cluster(100, 100, 70)]
    expectAllClear(applied(petals))
  })

  it('splits the push evenly, so neither cluster is singled out', () => {
    const petals = [cluster(0, 0, 80), cluster(50, 0, 80)]
    const offsets = resolvePetalOverlaps(petals)
    expect(offsets[0].dx).toBeCloseTo(-offsets[1].dx, 6)
    expect(offsets[0].dy).toBeCloseTo(-offsets[1].dy, 6)
  })

  it('pushes along the line between the centres', () => {
    // Two clusters side by side must part sideways, not drift vertically.
    const petals = [cluster(0, 0, 80), cluster(60, 0, 80)]
    const offsets = resolvePetalOverlaps(petals)
    expect(offsets[0].dx).toBeLessThan(0)
    expect(offsets[1].dx).toBeGreaterThan(0)
    expect(offsets[0].dy).toBeCloseTo(0, 6)
    expect(offsets[1].dy).toBeCloseTo(0, 6)
  })

  it('honours the requested gap', () => {
    const petals = [cluster(0, 0, 80), cluster(40, 10, 80)]
    const gap = 30
    expectAllClear(applied(petals, gap), gap)
  })

  it('opens up a tightly packed row', () => {
    const petals = [0, 30, 60, 90, 120].map((x) => cluster(x, 0, 70))
    expectAllClear(applied(petals, 10), 10)
  })

  it('opens up a tightly packed grid', () => {
    const petals: Petal[] = []
    for (let x = 0; x < 4; x++) {
      for (let y = 0; y < 4; y++) petals.push(cluster(x * 45, y * 45, 60))
    }
    expectAllClear(applied(petals, 8), 8)
  })

  it('handles clusters of very different sizes', () => {
    const petals = [cluster(0, 0, 400), cluster(20, 20, 25), cluster(-30, 10, 90)]
    expectAllClear(applied(petals, 12), 12)
  })

  it('keeps each petal around its own nodes', () => {
    // The offsets are applied to the cluster's nodes too, so the petal has to
    // still hold them once everything has moved.
    const groups: Point[][] = [
      [
        { x: 0, y: 0 },
        { x: 70, y: 0 },
        { x: 70, y: 50 },
        { x: 0, y: 50 },
      ],
      [
        { x: 40, y: 20 },
        { x: 110, y: 20 },
        { x: 110, y: 80 },
        { x: 40, y: 80 },
      ],
    ]
    const petals = groups.map((g) => fitPetal(convexHull(g), 8))
    const offsets = resolvePetalOverlaps(petals, 10)

    const moved = petals.map((p, i) => translatePetal(p, offsets[i].dx, offsets[i].dy))
    expectAllClear(moved, 10)

    groups.forEach((g, i) => {
      for (const node of g) {
        const shifted = { x: node.x + offsets[i].dx, y: node.y + offsets[i].dy }
        expect(petalContains(moved[i], shifted), `cluster ${i} lost a node`).toBe(true)
      }
    })
  })

  it('is idempotent: resolving a resolved arrangement changes nothing', () => {
    const petals = [cluster(0, 0, 80), cluster(45, 20, 80), cluster(-40, 30, 80)]
    const once = applied(petals, 12)
    expect(resolvePetalOverlaps(once, 12)).toEqual([
      { dx: 0, dy: 0 },
      { dx: 0, dy: 0 },
      { dx: 0, dy: 0 },
    ])
  })

  it('is deterministic, so a re-render does not shuffle the graph', () => {
    const petals = [cluster(0, 0, 80), cluster(35, 12, 80), cluster(10, 40, 80)]
    expect(resolvePetalOverlaps(petals, 10)).toEqual(resolvePetalOverlaps(petals, 10))
  })

  it('does not mutate the petals it was given', () => {
    const petals = [cluster(0, 0, 80), cluster(30, 0, 80)]
    const before = petals.map((p) => ({ ...p }))
    resolvePetalOverlaps(petals, 10)
    expect(petals).toEqual(before)
  })
})
