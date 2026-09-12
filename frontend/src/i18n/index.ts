/** Interface language. Hand-rolled because the CSP forbids eval and the usual
 *  library compiles its messages with generated functions at runtime. */

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

/** The language to open in. */
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

/** The language on screen, for modules that read it outside a component -- `content.ts` picks a… */
export const activeLocale = computed(() => locale.value)

/** Keeps the document's own language in step. */
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

/** Fills `{name}` placeholders. */
export function interpolate(template: string, params?: MessageParams): string {
  if (!params) return template
  return template.replace(PLACEHOLDER, (whole, name: string) =>
    name in params ? String(params[name]) : whole,
  )
}

/** One string in the active language. */
export function translate(key: MessageKey, params?: MessageParams): string {
  const active = CATALOGUES[locale.value] ?? CATALOGUES[DEFAULT_LOCALE]
  return interpolate(active.messages[key] || en.messages[key] || key, params)
}

/** The counted keys: every `x.other` whose base can be handed to `tn`. */
type BaseOf<K> = K extends `${infer B}.other` ? B : never
export type PluralKey = BaseOf<MessageKey>

/** A counted string, in the variant the active language gives that number. */
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
    locale: activeLocale,
    locales: LOCALES,
    t: translate,
    tn: translateCount,
    setLocale,
  }
}
