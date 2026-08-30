/**
 * Document prose, in the language the reader chose.
 *
 * This is the other half of the interface's language switch. `t()` translates
 * the app's own words, which ship with the app; this reads a language out of
 * the documentation, which the app has never seen and cannot translate. A
 * project that declares `type = "inline"` writes both languages into the field
 * itself, after a bracketed tag:
 *
 *     This is a column [JP] これはコラムです。
 *
 * The rules are the backend's -- `internal/i18ntext` -- and are documented once
 * in docs/usage/documentation-format.md. They are implemented twice because the
 * choice is made twice: the backend stores every language as written, and the
 * reader changes their mind about which one they are reading without fetching
 * the model again.
 *
 * Selection happens as a field is rendered rather than as it arrives, so
 * switching language redraws the pane instead of refetching the snapshot.
 */

import { computed, ref } from 'vue'

import type { ProjectMeta } from '../api/types'
import { activeLocale, type Locale } from './index'

/**
 * The document tags a UI language answers to.
 *
 * A manifest names its languages in the author's own words, and `JP` -- a
 * country code rather than a language one -- is what people write for
 * Japanese. Rather than insist on ISO 639, each interface language lists the
 * tags it will answer to and the first one the project declares wins.
 */
const LOCALE_TAGS: Record<Locale, string[]> = {
  en: ['EN', 'ENG'],
  ja: ['JA', 'JP', 'JPN'],
}

export interface DocumentLanguages {
  /** The language a field's untagged text is written in. */
  primary: string
  /** Every language the project declares. Only these are tags. */
  supported: string[]
}

const NO_LANGUAGES: DocumentLanguages = { primary: '', supported: [] }

/** Upper-cases a tag, so a manifest and a document written in different cases
 *  name the same language. */
export function normaliseTag(v: string): string {
  return v.trim().toUpperCase()
}

export interface Splitter {
  primary: string
  /** The field's text by upper-cased tag, in the order the field declares. */
  split(text: string): { order: string[]; byLang: Record<string, string> }
  /** The field's text in the first of `want` it carries, else the primary
   *  language, else whatever it does carry. */
  select(text: string, want: string[]): string
}

/** Builds a splitter for one project's languages. Built once per project and
 *  locale rather than per field: the tag pattern is the same for every field
 *  in a model, and a table has as many fields as it has columns. */
export function splitter(langs: DocumentLanguages): Splitter {
  const primary = normaliseTag(langs.primary)
  const tags = langs.supported.map(normaliseTag).filter(Boolean)
  // No declared languages -- a snapshot older than the manifest -- means no
  // tags, so every field reads whole.
  const markers = tags.length
    ? new RegExp(`\\[[ \\t]*(${tags.map(escape).join('|')})[ \\t]*\\]`, 'gi')
    : null

  function split(text: string) {
    const order: string[] = []
    const byLang: Record<string, string> = {}

    const add = (lang: string, body: string) => {
      const trimmed = body.trim()
      if (!trimmed) return
      if (lang in byLang) {
        // Joined rather than dropped: documentation is not worth losing to a
        // formatting slip. The backend reports it as duplicate_language_tag.
        byLang[lang] = `${byLang[lang]} ${trimmed}`
        return
      }
      byLang[lang] = trimmed
      order.push(lang)
    }

    if (!markers) {
      add(primary, text)
      return { order, byLang }
    }

    markers.lastIndex = 0
    const found: { tag: string; from: number; to: number }[] = []
    for (let m = markers.exec(text); m; m = markers.exec(text)) {
      found.push({ tag: normaliseTag(m[1]), from: m.index, to: m.index + m[0].length })
    }
    if (!found.length) {
      add(primary, text)
      return { order, byLang }
    }

    add(primary, text.slice(0, found[0].from))
    found.forEach((f, i) => add(f.tag, text.slice(f.to, found[i + 1]?.from ?? text.length)))
    return { order, byLang }
  }

  return {
    primary,
    split,
    select(text, want) {
      if (!text.trim()) return ''
      const { order, byLang } = split(text)
      for (const tag of want) {
        const t = byLang[normaliseTag(tag)]
        if (t) return t
      }
      if (byLang[primary]) return byLang[primary]
      for (const lang of order) {
        if (byLang[lang]) return byLang[lang]
      }
      return ''
    },
  }
}

function escape(v: string): string {
  return v.replace(/[.*+?^${}()|[\]\\-]/g, '\\$&')
}

// --- the loaded snapshot's languages --------------------------------------

const languages = ref<DocumentLanguages>(NO_LANGUAGES)

/**
 * Tells the module which languages the loaded snapshot's documents are in.
 *
 * Called when a snapshot loads. A snapshot ingested before the manifest was
 * required carries none, and its documents then read exactly as they did
 * before: whole, tags and all, since nothing is a tag.
 */
export function setDocumentLanguages(meta?: ProjectMeta | null): void {
  const i18n = meta?.internationalization
  languages.value = i18n ? { primary: i18n.primary, supported: i18n.supported ?? [] } : NO_LANGUAGES
}

const reader = computed(() => splitter(languages.value))

/** The tags the interface's current language answers to. */
const wanted = computed(() => LOCALE_TAGS[activeLocale.value] ?? [])

/**
 * One prose field, in the language on screen.
 *
 * Reads `activeLocale`, so any template that calls it re-renders when the
 * reader switches language -- the same contract `t()` has.
 */
export function documentText(text: string | null | undefined): string {
  if (!text) return ''
  return reader.value.select(text, wanted.value)
}

/** Companion to `useI18n`, for components that show documentation as well as
 *  the app's own words. `dt` is to a document what `t` is to the interface. */
export function useDocumentText() {
  return { dt: documentText }
}
