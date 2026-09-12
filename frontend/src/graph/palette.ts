/** Per-domain colours for the graph's cluster hulls. */

export type Theme = 'light' | 'dark'

/** Which treatment a hull needs, judged from the canvas it will be drawn on. */
export function canvasTheme(background: string): Theme {
  return relativeLuminance(background) < 0.42 ? 'dark' : 'light'
}

/** WCAG relative luminance of a #rrggbb colour; 0 for anything unparseable. */
function relativeLuminance(hex: string): number {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex.trim())
  if (!m) return 1
  const n = parseInt(m[1], 16)
  const channels = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((v) => {
    const c = v / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]
}

/** Which set of colours the domains are drawn from. */
export type Family = 'wheel' | 'sakura'

/** The theme that wears petals, and so the one that gets the pink band. */
export const SAKURA_ART = 'haru-urara'

/** The family a theme asks for, by its [data-art] slug. */
export function paletteFamily(art: string): Family {
  return art === SAKURA_ART ? 'sakura' : 'wheel'
}

/** One sakura colour, given at its light-canvas strength. See petalShade. */
interface Tone {
  h: number
  s: number
  l: number
}

/** Spaced around the wheel, skipping the near-yellow band that reads as a warning. */
const HUES = [188, 265, 24, 142, 328, 205, 45, 286, 352, 168, 236, 96]

/** The sakura band: petal pink through rose and cherry to plum and wisteria. */
const SAKURA: Tone[] = [
  { h: 340, s: 68, l: 47 }, // vivid rose -- the theme's own pink
  { h: 318, s: 46, l: 52 }, // orchid
  { h: 356, s: 58, l: 50 }, // cherry
  { h: 300, s: 32, l: 46 }, // plum
  { h: 332, s: 58, l: 36 }, // deep rose
  { h: 350, s: 30, l: 40 }, // wine mauve
  { h: 310, s: 44, l: 38 }, // deep plum
  { h: 344, s: 40, l: 60 }, // petal pink
  { h: 322, s: 34, l: 62 }, // dusty lilac
  { h: 0, s: 46, l: 58 }, // soft coral
  { h: 288, s: 30, l: 52 }, // wisteria
  { h: 336, s: 28, l: 46 }, // ash mauve
]

/** The sakura theme's own neutral: barely-tinted mauve, not a cold grey. */
const SAKURA_SOURCE: Tone = { h: 330, s: 10, l: 50 }

/** Clamps a derived saturation or lightness back into range. */
function pct(v: number): number {
  return Math.max(0, Math.min(100, Math.round(v)))
}

/** Fill, outline, caption and swatch for one sakura tone. */
function petalShade(t: Tone, theme: Theme, quiet = false): ClusterColor {
  // On a dark canvas a tone has to come up to meet it. Everything shifts
  // lighter together, so the four parts keep their relationship to each other.
  const lift = theme === 'dark' ? 22 : 0
  const s = pct(t.s - (theme === 'dark' ? 4 : 6))
  const base = pct(t.l + lift)
  const fillA = quiet ? (theme === 'dark' ? 0.09 : 0.07) : theme === 'dark' ? 0.14 : 0.13
  const strokeA = quiet ? (theme === 'dark' ? 0.44 : 0.4) : theme === 'dark' ? 0.6 : 0.55
  return {
    fill: `hsla(${t.h}, ${pct(t.s)}%, ${pct(base + 8)}%, ${fillA})`,
    stroke: `hsla(${t.h}, ${s}%, ${base}%, ${strokeA})`,
    // Captions are text, so the palest tones cannot simply be stepped down
    // from: the cap is what keeps a pale petal's name readable in its pill.
    label: `hsla(${t.h}, ${s}%, ${pct(theme === 'dark' ? base - 2 : Math.min(base - 14, 42))}%, ${
      quiet ? 0.9 : 0.95
    })`,
    swatch: `hsl(${t.h}, ${s}%, ${base}%)`,
  }
}

export interface ClusterColor {
  /** Hull interior. Translucent: the graph background must stay visible. */
  fill: string
  /** Hull outline. */
  stroke: string
  /** Cluster caption. */
  label: string
  /** Opaque swatch for chips and dots drawn outside the canvas. */
  swatch: string
}

export function domainColor(index: number, theme: Theme, family: Family = 'wheel'): ClusterColor {
  if (family === 'sakura') {
    return petalShade(SAKURA[((index % SAKURA.length) + SAKURA.length) % SAKURA.length], theme)
  }
  const h = HUES[((index % HUES.length) + HUES.length) % HUES.length]
  return theme === 'dark'
    ? {
        fill: `hsla(${h}, 66%, 56%, 0.13)`,
        stroke: `hsla(${h}, 62%, 62%, 0.55)`,
        label: `hsla(${h}, 58%, 74%, 0.95)`,
        swatch: `hsl(${h}, 62%, 62%)`,
      }
    : {
        fill: `hsla(${h}, 58%, 48%, 0.10)`,
        stroke: `hsla(${h}, 50%, 40%, 0.50)`,
        label: `hsla(${h}, 55%, 32%, 0.95)`,
        swatch: `hsl(${h}, 50%, 40%)`,
      }
}

/** Upstream source models are not a domain; they get a neutral. */
export function sourceColor(theme: Theme, family: Family = 'wheel'): ClusterColor {
  if (family === 'sakura') return petalShade(SAKURA_SOURCE, theme, true)
  return theme === 'dark'
    ? {
        fill: 'hsla(215, 20%, 62%, 0.09)',
        stroke: 'hsla(215, 16%, 62%, 0.42)',
        label: 'hsla(215, 18%, 70%, 0.9)',
        swatch: 'hsl(215, 16%, 62%)',
      }
    : {
        fill: 'hsla(215, 18%, 45%, 0.07)',
        stroke: 'hsla(215, 16%, 42%, 0.38)',
        label: 'hsla(215, 16%, 38%, 0.9)',
        swatch: 'hsl(215, 16%, 42%)',
      }
}

/** Maps domain id to its palette slot, in the snapshot's own domain order. */
export function domainIndex(ids: string[]): Map<string, number> {
  const m = new Map<string, number>()
  for (const id of ids) {
    if (!m.has(id)) m.set(id, m.size)
  }
  return m
}
