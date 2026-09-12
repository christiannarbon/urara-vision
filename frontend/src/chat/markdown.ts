/**
 * Render the small markdown subset an answer uses, as escaped HTML.
 *
 * The text is untrusted. Everything is escaped first and only the constructs
 * below are turned back into markup; escaping afterwards would escape our own
 * tags. No markdown library and no sanitiser, deliberately.
 */

const ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
}

/** One pass rather than five, so `&` cannot double-escape what came before. */
function escapeHtml(text: string): string {
  return text.replace(/[&<>"']/g, (c) => ESCAPES[c])
}

function unescapeHtml(text: string): string {
  return text
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&amp;/g, '&')
}

/** Where finished markup waits out the remaining passes, so emphasis cannot
 *  find the `_` in a URL. The token is NUL-delimited; the input cannot
 *  contain one because `renderAnswer` strips NUL up front. */
interface Slots {
  html: string[]
}

const TOKEN = /\u0000(\d+)\u0000/g

function stash(slots: Slots, html: string): string {
  slots.html.push(html)
  return `\u0000${slots.html.length - 1}\u0000`
}

/** Loops because a slot can hold a token: a link label may contain a code span. */
function restore(html: string, slots: Slots): string {
  let out = html
  for (let pass = 0; pass < 8; pass++) {
    TOKEN.lastIndex = 0
    if (!TOKEN.test(out)) break
    TOKEN.lastIndex = 0
    out = out.replace(TOKEN, (_m, i: string) => slots.html[Number(i)] ?? '')
  }
  TOKEN.lastIndex = 0
  return out
}

// --- links ------------------------------------------------------------------

const SAFE_SCHEMES = new Set(['http:', 'https:'])
const BLANK_OR_CONTROL = /[\s\u0000-\u0020\u007f-\u009f]/

/** The href to use, or null when the link must render as plain text. */
function safeHref(raw: string): string | null {
  const url = raw.trim()
  if (!url) return null

  // A URL that is only valid once characters are removed from it is not one this renderer will
  // link to.
  if (BLANK_OR_CONTROL.test(url)) return null

  let parsed: URL
  try {
    parsed = new URL(url)
  } catch {
    return null
  }
  if (!SAFE_SCHEMES.has(parsed.protocol)) return null

  // As written rather than the parser's normalisation, so the URL is not rewritten in front of the
  // reader.
  return url
}

// --- inline constructs ------------------------------------------------------

/** Bold before italic, or `**x**` is read as an empty italic. */
function emphasis(text: string): string {
  return text
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/(?<![\w*])\*(?!\s)([^*\n]+?)(?<!\s)\*(?![\w*])/g, '<em>$1</em>')
    .replace(/(?<![\w_])_(?!\s)([^_\n]+?)(?<!\s)_(?![\w_])/g, '<em>$1</em>')
}

/**
 * One level of nested parens, because `alert(1)` is the shape a hostile URL takes and a pattern
 * stopping at the first `(` never reaches the validator.
 */
const LINK = /\[([^\]\n]*)\]\(((?:[^()\n]|\([^()\n]*\))*)\)/g

const INLINE_CODE = /`([^`\n]+)`/g

function inline(text: string, slots: Slots): string {
  // Code first: nothing inside a span is markdown.
  let out = text.replace(INLINE_CODE, (_m, code: string) => stash(slots, `<code>${code}</code>`))

  out = out.replace(LINK, (source: string, label: string, href: string) => {
    const url = safeHref(unescapeHtml(href))
    // Refused: renders as the text the model wrote, still escaped.
    if (url === null) return source
    const attrs = `href="${escapeHtml(url)}" target="_blank" rel="noopener noreferrer nofollow"`
    return stash(slots, `<a ${attrs}>${emphasis(label)}</a>`)
  })

  return emphasis(out)
}

// --- blocks -----------------------------------------------------------------

const UNORDERED = /^\s*[-*]\s+(.*)$/
const ORDERED = /^\s*\d+\.\s+(.*)$/
const SLOT_LINE = /^\u0000\d+\u0000$/
const LANGUAGE = /^[A-Za-z0-9_+-]+$/

function listHtml(ordered: boolean, items: string[], slots: Slots): string {
  const tag = ordered ? 'ol' : 'ul'
  const li = items.map((item) => `<li>${inline(item, slots)}</li>`).join('')
  return `<${tag}>${li}</${tag}>`
}

/** One run of lines with no blank line in it. */
function renderBlock(block: string, slots: Slots): string {
  const lines = block.split('\n').filter((line) => line.trim() !== '')
  const out: string[] = []

  let paragraph: string[] = []
  let list: { ordered: boolean; items: string[] } | null = null

  const flushParagraph = () => {
    if (!paragraph.length) return
    // A break, not a space: two facts on two lines meant two lines.
    out.push(`<p>${paragraph.map((line) => inline(line, slots)).join('<br>')}</p>`)
    paragraph = []
  }

  const flushList = () => {
    if (!list) return
    out.push(listHtml(list.ordered, list.items, slots))
    list = null
  }

  for (const line of lines) {
    if (SLOT_LINE.test(line)) {
      flushParagraph()
      flushList()
      out.push(line)
      continue
    }

    const unordered = line.match(UNORDERED)
    const ordered = unordered ? null : line.match(ORDERED)
    if (unordered || ordered) {
      flushParagraph()
      const isOrdered = ordered !== null
      if (list && list.ordered !== isOrdered) flushList()
      if (!list) list = { ordered: isOrdered, items: [] }
      list.items.push((unordered ?? ordered)![1])
      continue
    }

    flushList()
    paragraph.push(line)
  }

  flushParagraph()
  flushList()
  return out.join('')
}

// --- entry point ------------------------------------------------------------

/** Render the answer's markdown subset as escaped HTML. */
export function renderAnswer(markdown: string): string {
  if (!markdown) return ''

  const slots: Slots = { html: [] }

  // NUL stripped, not escaped: it is what the slot tokens are built from, so removing it here is
  // what makes a token in the input impossible to forge.
  const source = String(markdown).replace(/\r\n?/g, '\n').replace(/\u0000/g, '')

  // Below this line nothing in the text can still become a tag.
  const escaped = escapeHtml(source)

  // Fences first, so their contents are never read as markdown. Wrapped in
  // blank lines so the block splitter always meets one alone.
  const withCode = escaped.replace(
    /```([^\n`]*)\n([\s\S]*?)```/g,
    (_m, lang: string, body: string) => {
      const label = lang.trim()
      const attr = LANGUAGE.test(label) ? ` class="language-${label}"` : ''
      const code = `<pre><code${attr}>${body.replace(/\n$/, '')}</code></pre>`
      return `\n\n${stash(slots, code)}\n\n`
    },
  )

  const html = withCode
    .split(/\n{2,}/)
    .map((block) => renderBlock(block, slots))
    .join('')

  return restore(html, slots)
}
