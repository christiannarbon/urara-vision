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

// jsdom only provides Web Storage for a real origin, and the default test URL
// is not one. The theme, the API token and the locale all persist there, so
// the specs that exercise persistence need somewhere for it to land.
if (!globalThis.localStorage) {
  const store = new Map<string, string>()
  const storage: Storage = {
    get length() {
      return store.size
    },
    key: (i: number) => [...store.keys()][i] ?? null,
    getItem: (k: string) => store.get(k) ?? null,
    setItem: (k: string, v: string) => void store.set(k, String(v)),
    removeItem: (k: string) => void store.delete(k),
    clear: () => store.clear(),
  }
  Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: storage })
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
  // Storage is stubbed above where jsdom does not supply it, but a spec is
  // still free to replace it with one that throws; clearing must not be the
  // thing that fails the test.
  try {
    globalThis.localStorage?.clear()
  } catch {
    // No accessible storage in this environment.
  }
})
