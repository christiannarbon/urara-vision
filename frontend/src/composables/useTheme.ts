/**
 * Theme state: which painting (or the house palette) supplies the colours,
 * corner style and typefaces.
 *
 * There is one axis. The app is light-mode only by design, so a theme is a
 * single palette rather than a light/dark pair -- see styles/art-themes.css.
 * The choice persists per browser.
 */

import { ref, computed, watch } from 'vue'

import themeMeta from '../styles/art-themes.meta.json'

export interface ArtTheme {
  /** Slug, matching the [data-art] value in art-themes.css. */
  id: string
  /** Primary line in the picker: an artist, or the house palette's name. */
  name: string
  /** Secondary line: the work, or a description of the palette. */
  subtitle: string
  /** Distinguishes the app's own palette from the painting-derived ones. */
  kind: 'house' | 'painting'
  /** Display and body faces the theme asks for, deduplicated. */
  fonts: string[]
  /** The theme's fact colour, for the picker's swatch. */
  swatch: string
}

export const themes = themeMeta as ArtTheme[]

const DEFAULT_ART = 'haru-urara'
const ART_KEY = 'relviz.art'

function readStoredArt(): string {
  try {
    const v = localStorage.getItem(ART_KEY)
    if (v && themes.some((t) => t.id === v)) return v
  } catch {
    // Private windows and blocked site data both throw; fall through.
  }
  return DEFAULT_ART
}

const art = ref<string>(readStoredArt())

// Older builds persisted a light/dark choice here. Nothing reads it now, so
// drop it rather than leave a key that implies the app still has a dark mode.
try {
  localStorage.removeItem('relviz.theme')
} catch {
  // Blocked site data; the stale key is harmless either way.
}

// --- webfonts -------------------------------------------------------------

/**
 * Loads a theme's faces on demand. Fifteen families across ten themes is far
 * too much to ship upfront, so each theme fetches only its own, and links are
 * left in place once added: switching back to a theme you have already seen
 * costs nothing.
 */
function ensureFonts(theme: ArtTheme) {
  if (typeof document === 'undefined') return
  // The head is the source of truth rather than a module-level set: under hot
  // reload, and anywhere this module ends up instantiated twice, a set would
  // start empty and stack a second copy of every link.
  if (document.querySelector(`link[data-art-fonts="${theme.id}"]`)) return

  if (!document.querySelector('link[data-font-preconnect]')) {
    for (const href of ['https://fonts.googleapis.com', 'https://fonts.gstatic.com']) {
      const l = document.createElement('link')
      l.rel = 'preconnect'
      l.href = href
      if (href.endsWith('gstatic.com')) l.crossOrigin = ''
      l.dataset.fontPreconnect = ''
      document.head.appendChild(l)
    }
  }

  // Google Fonts clamps a weight list to what a family actually ships, so the
  // same request works for a single-weight face like Archivo Black.
  const families = theme.fonts
    .map((f) => `family=${encodeURIComponent(f).replace(/%20/g, '+')}:wght@400;500;600;700`)
    .join('&')

  const link = document.createElement('link')
  link.rel = 'stylesheet'
  link.href = `https://fonts.googleapis.com/css2?${families}&display=swap`
  link.dataset.artFonts = theme.id
  document.head.appendChild(link)
}

// --- applying -------------------------------------------------------------

function apply(id: string) {
  const theme = themes.find((t) => t.id === id) ?? themes.find((t) => t.id === DEFAULT_ART)
  if (!theme) return
  document.documentElement.setAttribute('data-art', theme.id)
  ensureFonts(theme)
}

apply(art.value)

watch(art, (v) => {
  apply(v)
  try {
    localStorage.setItem(ART_KEY, v)
  } catch {
    // Persisting is a convenience; the choice still applies for this session.
  }
})

// --- public API -----------------------------------------------------------

export function useTheme() {
  const current = computed(() => themes.find((t) => t.id === art.value) ?? themes[0])

  function setArt(id: string) {
    art.value = id
  }

  return { art, themes, current, setArt }
}
