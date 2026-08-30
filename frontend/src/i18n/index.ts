/**
 * Interface language.
 *
 * A catalogue lookup and a locale ref, deliberately hand-rolled. The app ships
 * three runtime dependencies and a content-security policy that forbids `eval`
 * -- a check the frontend workflow enforces against the built bundle -- and the
 * usual library compiles its messages with generated functions at runtime,
 * which that policy blocks. What is actually needed here is a typed lookup and
 * `{name}` substitution, and that is what this is.
 *
 * The choice persists per browser, alongside the theme, and follows the same
 * shape as `useTheme`: module-level state, one `watch` that applies and stores
 * it, and a composable that hands the pieces to a component.
 *
 * `t` reads `locale.value`, so any template or computed that calls it
 * re-renders when the language changes. Nothing needs to remount.
 */

import { computed, ref, watch } from 'vue'

import * as en from './messages/en'
import * as ja from './messages/ja'
import type { MessageKey } from './messages/en'

export type { MessageKey, Messages } from './messages/en'

/** The languages the interface is available in, in the order a picker lists them. */
export const LOCALES = ['en', 'ja'] as const

export type Locale = (typeof LOCALES)[number]

const CATALOGUES: Record<Locale, { messages: Record<MessageKey, string>; plural: (n: number) => 'one' | 'other' }> = {
  en,
  ja,
}

const DEFAULT_LOCALE: Locale = 'en'
const LOCALE_KEY = 'relviz.locale'

function isLocale(v: unknown): v is Locale {
  return typeof v === 'string' && (LOCALES as readonly string[]).includes(v)
}

/**
 * The language to open in.
 *
 * A stored choice wins outright: someone who picked English on a Japanese
 * machine meant it. Otherwise the browser's preference list is read in order
 * and matched on the primary subtag, so `ja-JP` and a bare `ja` both land on
 * Japanese, and a list that leads with an unsupported language still finds a
 * supported one further down instead of falling straight to English.
 */
function detectLocale(): Locale {
  try {
    const stored = localStorage.getItem(LOCALE_KEY)
    if (isLocale(stored)) return stored
  } catch {
    // Private windows and blocked site data both throw; fall through to the
    // browser's own preference.
  }

  const preferred = typeof navigator === 'undefined' ? [] : (navigator.languages ?? [navigator.language])
  for (const tag of preferred) {
    const primary = String(tag ?? '').toLowerCase().split('-')[0]
    if (isLocale(primary)) return primary
  }
  return DEFAULT_LOCALE
}

const locale = ref<Locale>(detectLocale())

/**
 * Keeps the document's own language in step.
 *
 * This is not decoration. Screen readers pick a voice from it, and browsers
 * pick line-breaking and font fallback from it -- Japanese broken with English
 * rules breaks in the wrong places.
 */
function apply(v: Locale) {
  if (typeof document === 'undefined') return
  document.documentElement.setAttribute('lang', v)
}

apply(locale.value)

watch(locale, (v) => {
  apply(v)
  try {
    localStorage.setItem(LOCALE_KEY, v)
  } catch {
    // Persisting is a convenience; the choice still holds for this session.
  }
})

// --- lookup ---------------------------------------------------------------

const PLACEHOLDER = /\{(\w+)\}/g

export type MessageParams = Record<string, string | number>

/**
 * Fills `{name}` placeholders.
 *
 * A placeholder with no matching param is left standing rather than blanked.
 * A visible `{count}` is a bug report; a silent gap in a sentence is not.
 */
export function interpolate(template: string, params?: MessageParams): string {
  if (!params) return template
  return template.replace(PLACEHOLDER, (whole, name: string) =>
    name in params ? String(params[name]) : whole,
  )
}

/**
 * One string in the active language.
 *
 * The key type makes a missing entry a compile error, so the English fallback
 * is not for typos -- it is for the case where a translation is added to the
 * catalogue with an empty value while the wording is still being settled.
 */
export function translate(key: MessageKey, params?: MessageParams): string {
  const active = CATALOGUES[locale.value] ?? CATALOGUES[DEFAULT_LOCALE]
  return interpolate(active.messages[key] || en.messages[key] || key, params)
}

/** The counted keys: every `x.other` whose base can be handed to `tn`. */
type BaseOf<K> = K extends `${infer B}.other` ? B : never
export type PluralKey = BaseOf<MessageKey>

/**
 * A counted string, in the variant the active language gives that number.
 *
 * `n` is also passed through as the `{n}` placeholder, because a counted
 * string that does not show its count is vanishingly rare and repeating it at
 * every call site is not worth the symmetry.
 */
export function translateCount(key: PluralKey, n: number, params?: MessageParams): string {
  const active = CATALOGUES[locale.value] ?? CATALOGUES[DEFAULT_LOCALE]
  return translate(`${key}.${active.plural(n)}` as MessageKey, { n, ...params })
}

// --- public API -----------------------------------------------------------

export function setLocale(v: Locale) {
  locale.value = v
}

export function useI18n() {
  return {
    locale: computed(() => locale.value),
    locales: LOCALES,
    t: translate,
    tn: translateCount,
    setLocale,
  }
}
