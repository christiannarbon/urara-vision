/**
 * Test environment setup.
 *
 * jsdom does not implement layout or canvas, and the graph canvas reaches for
 * both. Rather than mock them per spec, the missing pieces are filled in once
 * here with the smallest stand-ins the components actually need.
 */

import { beforeEach, vi } from 'vitest'

// The graph canvas measures its container; jsdom reports every element as 0x0,
// which makes layout code divide by zero.
Object.defineProperty(HTMLElement.prototype, 'getBoundingClientRect', {
  configurable: true,
  value() {
    return { x: 0, y: 0, width: 800, height: 600, top: 0, left: 0, right: 800, bottom: 600, toJSON: () => ({}) }
  },
})

// jsdom has no canvas implementation. Every drawing call is a no-op recorder,
// which is enough for code that only traces paths.
if (!HTMLCanvasElement.prototype.getContext) {
  HTMLCanvasElement.prototype.getContext = (() =>
    new Proxy(
      {},
      {
        get: (_t, prop) => {
          if (prop === 'canvas') return undefined
          if (prop === 'measureText') return () => ({ width: 0 })
          return () => undefined
        },
        set: () => true,
      },
    )) as never
}

// ResizeObserver drives the canvas's resize handling.
if (!globalThis.ResizeObserver) {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as never
}

if (!globalThis.matchMedia) {
  globalThis.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as never
}

beforeEach(() => {
  // No spec should reach the network by accident; each one that needs fetch
  // installs its own stub.
  vi.restoreAllMocks()
  // Storage is not guaranteed to exist -- jsdom only provides it for a real
  // origin -- and a spec that does not touch it should not care either way.
  try {
    globalThis.localStorage?.clear()
  } catch {
    // No accessible storage in this environment.
  }
})
