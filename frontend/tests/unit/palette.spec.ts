/**
 * Cluster colour assignment.
 *
 * The colours carry meaning -- one hue per domain, stable as filters change --
 * so the properties worth asserting are about identity and legibility, not
 * about any particular hue.
 */

import { describe, expect, it } from 'vitest'

import {
  canvasTheme,
  domainColor,
  domainIndex,
  paletteFamily,
  sourceColor,
  SAKURA_ART,
} from '../../src/graph/palette'

describe('canvasTheme', () => {
  it('reads a light canvas as light and a dark one as dark', () => {
    expect(canvasTheme('#ffffff')).toBe('light')
    expect(canvasTheme('#000000')).toBe('dark')
  })

  it('judges by luminance, not by the theme being light-mode', () => {
    // The point of the function: a deep salmon canvas is a light-mode theme but
    // needs the dark treatment, or hulls tuned for white paper vanish into it.
    expect(canvasTheme('#7a2e28')).toBe('dark')
    expect(canvasTheme('#f7e6e2')).toBe('light')
  })

  it('accepts a colour with or without the hash', () => {
    expect(canvasTheme('ffffff')).toBe('light')
    expect(canvasTheme('  #FFFFFF  ')).toBe('light')
  })

  it('treats an unparseable colour as light rather than throwing', () => {
    // An unknown value means the theme changed shape; a wrong-but-legible
    // default beats a blank canvas.
    for (const bad of ['', 'rebeccapurple', 'rgb(0,0,0)', '#fff', '#12345g']) {
      expect(canvasTheme(bad)).toBe('light')
    }
  })
})

describe('domainIndex', () => {
  it('assigns slots in the order given', () => {
    expect(domainIndex(['domain_one', 'domain_two', 'users'])).toEqual(
      new Map([
        ['domain_one', 0],
        ['domain_two', 1],
        ['users', 2],
      ]),
    )
  })

  it('keeps the first slot a domain was given when it repeats', () => {
    const m = domainIndex(['domain_one', 'domain_two', 'domain_one'])
    expect(m.get('domain_one')).toBe(0)
    expect(m.size).toBe(2)
  })

  it('is empty for no domains', () => {
    expect(domainIndex([]).size).toBe(0)
  })
})

describe('domainColor', () => {
  it('gives every slot a full set of colours', () => {
    for (const theme of ['light', 'dark'] as const) {
      const c = domainColor(0, theme)
      expect(c.fill).toMatch(/^hsla\(/)
      expect(c.stroke).toMatch(/^hsla\(/)
      expect(c.label).toMatch(/^hsla\(/)
      // The swatch is drawn outside the canvas, so it must be opaque.
      expect(c.swatch).toMatch(/^hsl\(/)
      expect(c.swatch).not.toMatch(/^hsla\(/)
    }
  })

  it('gives neighbouring domains distinct hues', () => {
    const hues = new Set<string>()
    for (let i = 0; i < 12; i++) {
      hues.add(domainColor(i, 'light').swatch)
    }
    expect(hues.size).toBe(12)
  })

  it('is stable for a given slot, so filtering does not recolour a domain', () => {
    expect(domainColor(3, 'light')).toEqual(domainColor(3, 'light'))
  })

  it('wraps past the end of the palette instead of running out', () => {
    // There are 12 hues; slot 12 reuses the first rather than returning
    // undefined and painting a cluster black.
    expect(domainColor(12, 'light')).toEqual(domainColor(0, 'light'))
    expect(domainColor(25, 'dark')).toEqual(domainColor(1, 'dark'))
  })

  it('handles a negative slot, which a missing domain would produce', () => {
    const c = domainColor(-1, 'light')
    expect(c.swatch).toMatch(/^hsl\(\d+/)
    expect(c).toEqual(domainColor(11, 'light'))
  })

  it('differs between the two canvas treatments', () => {
    expect(domainColor(0, 'light')).not.toEqual(domainColor(0, 'dark'))
  })
})

describe('sourceColor', () => {
  it('is a neutral grey, not a domain hue', () => {
    // Upstream models are not a subject area, and giving them a hue would read
    // as one more domain.
    const grey = sourceColor('light')
    for (let i = 0; i < 12; i++) {
      expect(domainColor(i, 'light').swatch).not.toBe(grey.swatch)
    }
  })

  it('provides both treatments', () => {
    expect(sourceColor('light')).not.toEqual(sourceColor('dark'))
    expect(sourceColor('dark').swatch).toMatch(/^hsl\(/)
  })
})

describe('paletteFamily', () => {
  it('gives the petal theme the pink band and everything else the wheel', () => {
    expect(paletteFamily(SAKURA_ART)).toBe('sakura')
    expect(paletteFamily('studio-paper')).toBe('wheel')
    expect(paletteFamily('')).toBe('wheel')
  })
})

describe('the sakura family', () => {
  /** Hue of an hsl()/hsla() string. */
  function hue(c: string): number {
    const m = /^hsla?\((\d+(?:\.\d+)?),/.exec(c)
    expect(m, c).not.toBeNull()
    return Number(m![1])
  }

  /** Distance to the pink band, which runs 280 through 0 to 25. */
  function outsidePink(h: number): boolean {
    return h > 25 && h < 280
  }

  it('keeps every domain inside the pink band', () => {
    // The whole point: a cluster in this theme is drawn as a sakura petal, and
    // a teal or olive petal is not a petal.
    for (const theme of ['light', 'dark'] as const) {
      for (let i = 0; i < 12; i++) {
        const c = domainColor(i, theme, 'sakura')
        for (const part of [c.fill, c.stroke, c.label, c.swatch]) {
          expect(outsidePink(hue(part)), `slot ${i} ${theme}: ${part}`).toBe(false)
        }
      }
    }
  })

  it('leaves the wheel themes alone', () => {
    // Only Haru Urara is repainted; the default argument is the old palette.
    expect(domainColor(0, 'light')).toEqual(domainColor(0, 'light', 'wheel'))
    expect(domainColor(0, 'light', 'wheel')).not.toEqual(domainColor(0, 'light', 'sakura'))
  })

  it('still tells twelve domains apart', () => {
    // Hue alone cannot do it inside one band, so the tones vary in saturation
    // and lightness too; this is the assertion that holds them to it.
    const swatches = new Set<string>()
    for (let i = 0; i < 12; i++) swatches.add(domainColor(i, 'light', 'sakura').swatch)
    expect(swatches.size).toBe(12)
  })

  it('gives every slot a full set of colours', () => {
    for (const theme of ['light', 'dark'] as const) {
      const c = domainColor(0, theme, 'sakura')
      expect(c.fill).toMatch(/^hsla\(/)
      expect(c.stroke).toMatch(/^hsla\(/)
      expect(c.label).toMatch(/^hsla\(/)
      expect(c.swatch).toMatch(/^hsl\(/)
      expect(c.swatch).not.toMatch(/^hsla\(/)
    }
  })

  it('wraps and handles a negative slot, as the wheel does', () => {
    expect(domainColor(12, 'light', 'sakura')).toEqual(domainColor(0, 'light', 'sakura'))
    expect(domainColor(-1, 'light', 'sakura')).toEqual(domainColor(11, 'light', 'sakura'))
  })

  it('differs between the two canvas treatments', () => {
    expect(domainColor(0, 'light', 'sakura')).not.toEqual(domainColor(0, 'dark', 'sakura'))
  })

  it('keeps captions dark enough to read on a pale pink canvas', () => {
    for (let i = 0; i < 12; i++) {
      const l = Number(/,\s*([\d.]+)%,\s*[\d.]+\)$/.exec(domainColor(i, 'light', 'sakura').label)![1])
      expect(l, `slot ${i}`).toBeLessThanOrEqual(42)
    }
  })

  it('gives sources a pink-side neutral that is still not a domain', () => {
    const neutral = sourceColor('light', 'sakura')
    expect(outsidePink(hue(neutral.swatch))).toBe(false)
    // Nearly grey, so it reads as the quiet cluster it is rather than a
    // thirteenth domain.
    const sat = Number(/,\s*([\d.]+)%,/.exec(neutral.swatch)![1])
    expect(sat).toBeLessThan(20)
    for (let i = 0; i < 12; i++) {
      expect(domainColor(i, 'light', 'sakura').swatch).not.toBe(neutral.swatch)
    }
  })

  it('leaves the wheel source grey untouched', () => {
    expect(sourceColor('light')).toEqual(sourceColor('light', 'wheel'))
    expect(sourceColor('light', 'wheel')).not.toEqual(sourceColor('light', 'sakura'))
  })
})
