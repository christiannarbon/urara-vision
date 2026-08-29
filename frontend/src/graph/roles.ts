/**
 * The vocabulary of table roles, and how each one is drawn.
 *
 * This mirrors backend/internal/model/roles.go: the backend decides what a
 * document means by "Type: Hub", this file decides what a hub looks like. The
 * two lists are allowed to drift. A role the backend learns first falls through
 * to roleSpec's default and renders with a generated shape, a generated colour
 * and its own name -- which is exactly the treatment any role outside the three
 * known vocabularies gets, so a model built on a style nobody anticipated still
 * reads correctly rather than collapsing into a canvas of grey circles.
 *
 * Two channels carry the role, and they are deliberately unequal:
 *
 *   Shape is primary. It is theme-independent, survives being printed in grey,
 *   and is the only channel a reader with a colour vision deficiency can rely
 *   on. Every role gets its own.
 *
 *   Colour is secondary and derived. Themes define exactly two role hues,
 *   --fact and --dim, and regenerating twenty painting-derived palettes for
 *   sixteen roles is not a trade worth making -- see styles/art-themes.css.
 *   Each role instead sits at a small deterministic shift off whichever of the
 *   two hues it is nearest in meaning: tables that carry events and keys move
 *   off --fact, tables that carry context move off --dim. The shifts are kept
 *   small enough to stay inside a theme's character, which matters most for
 *   Haru Urara, whose graph is meant to read as sakura rather than as a wheel.
 *
 * Fact and dimension sit at zero shift and use their tokens untouched, so a
 * plain star schema looks exactly as it did before any of this existed.
 */

/** The modelling style a role belongs to. Mirrors model.RoleFamily. */
export type RoleFamily = 'kimball' | 'vault' | 'relational' | 'other'

/** Which of the theme's two role hues a role derives its colour from. */
export type Anchor = 'fact' | 'dim'

/**
 * The legend draws a swatch rather than a node, and a 10px swatch cannot carry
 * a heptagon. Shapes collapse to three silhouettes there.
 */
export type Swatch = 'square' | 'round' | 'angular'

export interface RoleSpec {
  id: string
  label: string
  family: RoleFamily
  /** A Cytoscape node shape. */
  shape: string
  anchor: Anchor
  /** Degrees of hue rotation off the anchor token. */
  hueShift: number
  /** Percentage points of lightness shift off the anchor token. */
  lightShift: number
  swatch: Swatch
}

const FAMILY_ORDER: RoleFamily[] = ['kimball', 'vault', 'relational', 'other']

/**
 * The built-in roles, in the order the UI lists them.
 *
 * Within an anchor the hue shifts fan out either side of zero rather than
 * marching in one direction, so no role drifts far from the theme's own hue.
 */
const KNOWN: RoleSpec[] = [
  // Kimball: star and snowflake schemas.
  { id: 'fact', label: 'Fact', family: 'kimball', shape: 'round-rectangle', anchor: 'fact', hueShift: 0, lightShift: 0, swatch: 'square' },
  { id: 'factless', label: 'Factless fact', family: 'kimball', shape: 'cut-rectangle', anchor: 'fact', hueShift: 18, lightShift: 7, swatch: 'square' },
  { id: 'dimension', label: 'Dimension', family: 'kimball', shape: 'ellipse', anchor: 'dim', hueShift: 0, lightShift: 0, swatch: 'round' },
  { id: 'outrigger', label: 'Outrigger', family: 'kimball', shape: 'round-diamond', anchor: 'dim', hueShift: 18, lightShift: 6, swatch: 'angular' },
  { id: 'bridge', label: 'Bridge', family: 'kimball', shape: 'hexagon', anchor: 'dim', hueShift: 30, lightShift: -4, swatch: 'angular' },
  { id: 'junk', label: 'Junk dimension', family: 'kimball', shape: 'barrel', anchor: 'dim', hueShift: -12, lightShift: 12, swatch: 'round' },
  { id: 'degenerate', label: 'Degenerate dimension', family: 'kimball', shape: 'tag', anchor: 'dim', hueShift: 24, lightShift: -8, swatch: 'square' },

  // Data Vault.
  { id: 'hub', label: 'Hub', family: 'vault', shape: 'round-hexagon', anchor: 'fact', hueShift: -18, lightShift: -5, swatch: 'angular' },
  { id: 'link', label: 'Link', family: 'vault', shape: 'diamond', anchor: 'fact', hueShift: 30, lightShift: 3, swatch: 'angular' },
  { id: 'satellite', label: 'Satellite', family: 'vault', shape: 'pentagon', anchor: 'dim', hueShift: -18, lightShift: 4, swatch: 'round' },
  { id: 'pit', label: 'Point-in-time', family: 'vault', shape: 'vee', anchor: 'dim', hueShift: -24, lightShift: -7, swatch: 'angular' },

  // Third normal form and plain relational.
  { id: 'entity', label: 'Entity', family: 'relational', shape: 'rectangle', anchor: 'fact', hueShift: -30, lightShift: 8, swatch: 'square' },
  { id: 'associative', label: 'Associative', family: 'relational', shape: 'rhomboid', anchor: 'fact', hueShift: 12, lightShift: -6, swatch: 'angular' },
  { id: 'lookup', label: 'Lookup', family: 'relational', shape: 'round-triangle', anchor: 'dim', hueShift: -30, lightShift: 9, swatch: 'angular' },
  { id: 'reference', label: 'Reference', family: 'relational', shape: 'octagon', anchor: 'dim', hueShift: 12, lightShift: 10, swatch: 'round' },

  // The role a document names when it names none.
  { id: 'unknown', label: 'Unknown', family: 'other', shape: 'ellipse', anchor: 'dim', hueShift: 0, lightShift: 22, swatch: 'round' },
]

const byId = new Map(KNOWN.map((r) => [r.id, r]))
const orderOf = new Map(KNOWN.map((r, i) => [r.id, i]))

/**
 * The shapes an unrecognised role is assigned from, chosen so none of them is
 * already the silhouette of a common built-in role. Which one a role gets is
 * hashed from its name, so it stays put across ingests and across sessions.
 */
const SPARE_SHAPES = ['round-octagon', 'heptagon', 'concave-hexagon', 'right-rhomboid', 'round-pentagon', 'bottom-round-rectangle']

/** A small stable hash. Not cryptographic; it only has to be deterministic. */
function hash(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return Math.abs(h)
}

/** Turns a role slug into a display label: "point_in_time" reads as "Point in time". */
export function roleLabel(id: string): string {
  const parts = id.split(/[_-]+/).filter(Boolean)
  if (!parts.length) return id
  return [parts[0][0].toUpperCase() + parts[0].slice(1), ...parts.slice(1)].join(' ')
}

/**
 * The spec for a role id.
 *
 * A role outside the built-in list is given one rather than refused: it keeps
 * its own name, takes a spare shape and a hue shift hashed from that name, and
 * so comes out distinct from every other unrecognised role in the same model.
 */
export function roleSpec(id: string): RoleSpec {
  const known = byId.get(id)
  if (known) return known
  if (!id) return byId.get('unknown')!
  const h = hash(id)
  return {
    id,
    label: roleLabel(id),
    family: 'other',
    shape: SPARE_SHAPES[h % SPARE_SHAPES.length],
    // Unrecognised roles lean on --dim: a role this tool cannot place is more
    // likely to be describing something than measuring it, and --fact reads as
    // the emphatic hue in every theme.
    anchor: 'dim',
    hueShift: ((h >> 3) % 61) - 30,
    lightShift: ((h >> 9) % 21) - 6,
    swatch: 'angular',
  }
}

/**
 * The roles present in a set of kinds, deduplicated and in display order:
 * built-in roles first in vocabulary order, then anything the documents brought
 * with them, alphabetically.
 */
export function rolesPresent(kinds: Iterable<string>): RoleSpec[] {
  const ids = new Set<string>()
  for (const k of kinds) if (k) ids.add(k)
  return [...ids]
    .map(roleSpec)
    .sort((a, b) => {
      const ao = orderOf.get(a.id) ?? Infinity
      const bo = orderOf.get(b.id) ?? Infinity
      if (ao !== bo) return ao - bo
      const af = FAMILY_ORDER.indexOf(a.family)
      const bf = FAMILY_ORDER.indexOf(b.family)
      if (af !== bf) return af - bf
      return a.label.localeCompare(b.label)
    })
}

// --- colour ---------------------------------------------------------------

interface HSL {
  h: number
  s: number
  l: number
}

/** Parses #rgb or #rrggbb into HSL. Null for anything else. */
function hexToHsl(hex: string): HSL | null {
  const m = /^#?([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(hex.trim())
  if (!m) return null
  let v = m[1]
  if (v.length === 3) v = v[0] + v[0] + v[1] + v[1] + v[2] + v[2]
  const n = parseInt(v, 16)
  const r = ((n >> 16) & 255) / 255
  const g = ((n >> 8) & 255) / 255
  const b = (n & 255) / 255
  const max = Math.max(r, g, b)
  const min = Math.min(r, g, b)
  const l = (max + min) / 2
  const d = max - min
  if (d === 0) return { h: 0, s: 0, l: l * 100 }
  const s = d / (1 - Math.abs(2 * l - 1))
  let h: number
  if (max === r) h = ((g - b) / d) % 6
  else if (max === g) h = (b - r) / d + 2
  else h = (r - g) / d + 4
  h *= 60
  if (h < 0) h += 360
  return { h, s: s * 100, l: l * 100 }
}

/**
 * The colour a role is drawn in, given the theme's two role hues.
 *
 * Lightness is clamped well short of both ends: a role that comes out near
 * white loses its white node border, and one that comes out near black stops
 * being recognisable as the theme's colour at all.
 */
export function roleColor(spec: RoleSpec, factHex: string, dimHex: string): string {
  const base = spec.anchor === 'fact' ? factHex : dimHex
  if (!spec.hueShift && !spec.lightShift) return base
  const hsl = hexToHsl(base)
  // An unparseable token means a theme is using a colour form this cannot read;
  // the unshifted hue is wrong for the role but right for the theme, which is
  // the better failure.
  if (!hsl) return base
  const h = (((hsl.h + spec.hueShift) % 360) + 360) % 360
  const l = Math.min(72, Math.max(24, hsl.l + spec.lightShift))
  return `hsl(${Math.round(h)}, ${Math.round(hsl.s)}%, ${Math.round(l)}%)`
}
