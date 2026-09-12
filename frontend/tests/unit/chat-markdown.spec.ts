/**
 * The answer renderer.
 *
 * Every test goes through `render`, which records its output for the sweep at
 * the bottom of the file: a construct added later has to get past that as well
 * as its own test.
 */

import { describe, expect, it } from 'vitest'

import { renderAnswer } from '../../src/chat/markdown'

const rendered: string[] = []

function render(markdown: string): string {
  const html = renderAnswer(markdown)
  rendered.push(html)
  return html
}

describe('inline formatting', () => {
  it('renders bold', () => {
    expect(render('a **bold** word')).toBe('<p>a <strong>bold</strong> word</p>')
  })

  it('renders italic in both spellings', () => {
    expect(render('an *italic* word')).toBe('<p>an <em>italic</em> word</p>')
    expect(render('an _italic_ word')).toBe('<p>an <em>italic</em> word</p>')
  })

  it('leaves the underscores in an identifier alone', () => {
    expect(render('see fct_orders_daily')).toBe('<p>see fct_orders_daily</p>')
  })

  it('renders inline code', () => {
    expect(render('the `order_id` column')).toBe('<p>the <code>order_id</code> column</p>')
  })

  it('does not format inside an inline code span', () => {
    expect(render('`**not bold**`')).toBe('<p><code>**not bold**</code></p>')
  })

  it('leaves an unclosed marker as literal asterisks', () => {
    // Guards against a stray `**` swallowing the rest of the answer.
    expect(render('**unclosed and then some more text')).toBe(
      '<p>**unclosed and then some more text</p>',
    )
  })

  it('does not read spaced asterisks as emphasis', () => {
    expect(render('a * b * c')).toBe('<p>a * b * c</p>')
  })
})

describe('blocks', () => {
  it('splits paragraphs on a blank line', () => {
    expect(render('first\n\nsecond')).toBe('<p>first</p><p>second</p>')
  })

  it('keeps a single newline as a line break', () => {
    expect(render('first\nsecond')).toBe('<p>first<br>second</p>')
  })

  it('renders an unordered list', () => {
    expect(render('- one\n- two')).toBe('<ul><li>one</li><li>two</li></ul>')
  })

  it('renders an ordered list', () => {
    expect(render('1. one\n2. two')).toBe('<ol><li>one</li><li>two</li></ol>')
  })

  it('renders a list introduced on the line above it', () => {
    expect(render('The facts are:\n- orders\n- payments')).toBe(
      '<p>The facts are:</p><ul><li>orders</li><li>payments</li></ul>',
    )
  })

  it('formats inside a list item', () => {
    expect(render('- a **fact** table')).toBe('<ul><li>a <strong>fact</strong> table</li></ul>')
  })

  it('renders a fenced block without a language', () => {
    expect(render('```\nselect 1\n```')).toBe('<pre><code>select 1</code></pre>')
  })

  it('renders a fenced block with a language', () => {
    expect(render('```sql\nselect 1\n```')).toBe(
      '<pre><code class="language-sql">select 1</code></pre>',
    )
  })

  it('keeps markdown inside a fence literal', () => {
    const html = render('```\n**not bold** and _not italic_\n```')
    expect(html).toBe('<pre><code>**not bold** and _not italic_</code></pre>')
    expect(html).not.toContain('<strong>')
    expect(html).not.toContain('<em>')
  })

  it('keeps a fence separate from the prose around it', () => {
    expect(render('before\n```\ncode\n```\nafter')).toBe(
      '<p>before</p><pre><code>code</code></pre><p>after</p>',
    )
  })

  it('renders nothing for empty input', () => {
    expect(render('')).toBe('')
  })
})

describe('what is deliberately not supported', () => {
  it('leaves a heading as visible text', () => {
    // A visible `#` is the signal that the prompt drifted.
    expect(render('# Heading')).toBe('<p># Heading</p>')
  })

  it('does not render an image', () => {
    const html = render('![alt](https://example.com/x.png)')
    expect(html).not.toContain('<img')
  })

  it('does not autolink a bare URL', () => {
    const html = render('see https://example.com for more')
    expect(html).not.toContain('<a ')
  })
})

describe('escaping', () => {
  it('escapes a script tag rather than emitting one', () => {
    const html = render('<script>alert(1)</script>')
    expect(html).not.toContain('<script')
    expect(html).toContain('&lt;script&gt;')
  })

  it('escapes an image with an event handler', () => {
    const html = render('<img src=x onerror=alert(1)>')
    expect(html).not.toContain('<img')
    expect(html).toContain('&lt;img')
  })

  it('escapes an event handler that sits inside otherwise valid markdown', () => {
    // The surrounding markdown really does render, so the escaping has to
    // survive a pass that is actively writing tags.
    const html = render('a **bold** <b onclick="alert(1)">click</b> word')
    expect(html).toContain('<strong>bold</strong>')
    expect(html).not.toContain('<b ')
    expect(html).not.toContain('onclick="alert(1)"')
    expect(html).toContain('&lt;b onclick=&quot;alert(1)&quot;&gt;')
  })

  it('escapes an ampersand exactly once', () => {
    const html = render('orders & payments')
    expect(html).toBe('<p>orders &amp; payments</p>')
    expect(html).not.toContain('&amp;amp;')
  })

  it('does not double-escape an entity the author wrote', () => {
    expect(render('&amp;')).toBe('<p>&amp;amp;</p>')
  })

  it('escapes quotes, which is what keeps an attribute from being reopened', () => {
    const html = render(`a "quoted" and an 'apostrophed' word`)
    expect(html).toContain('&quot;quoted&quot;')
    expect(html).toContain('&#39;apostrophed&#39;')
  })
})

describe('link schemes', () => {
  it('renders an https link with rel and target', () => {
    const html = render('[docs](https://example.com)')
    expect(html).toBe(
      '<p><a href="https://example.com" target="_blank" rel="noopener noreferrer nofollow">docs</a></p>',
    )
  })

  it('renders an http link', () => {
    expect(render('[docs](http://example.com/a)')).toContain('href="http://example.com/a"')
  })

  it('keeps a query string intact and escaped', () => {
    const html = render('[docs](https://example.com/?a=1&b=2)')
    expect(html).toContain('href="https://example.com/?a=1&amp;b=2"')
  })

  const refused: Array<[string, string]> = [
    ['javascript', '[click](javascript:alert(1))'],
    ['javascript in mixed case', '[click](JaVaScRiPt:alert(1))'],
    ['javascript split by a tab', '[click](java\tscript:alert(1))'],
    ['javascript split by a newline inside the parens', '[click](java\nscript:alert(1))'],
    ['data', '[click](data:text/html,<script>)'],
    ['vbscript', '[click](vbscript:msgbox(1))'],
    ['scheme-relative', '[click](//evil.com)'],
    ['a bare path', '[click](/relative)'],
  ]

  for (const [name, markdown] of refused) {
    it(`refuses a ${name} URL and renders it as text`, () => {
      const html = render(markdown)
      expect(html).not.toContain('<a ')
      expect(html).not.toContain('href')
      // Still shown, still inert.
      expect(html).toContain('click')
    })
  }

  it('refuses a scheme the parser only accepts after removing characters', () => {
    // `htt\tps://` parses as https once the tab is stripped.
    const html = render('[click](htt\tps://example.com)')
    expect(html).not.toContain('<a ')
  })

  it('does not let a link label carry an event handler out', () => {
    const html = render('[<img src=x onerror=alert(1)>](https://example.com)')
    expect(html).toContain('<a ')
    expect(html).not.toContain('<img')
    expect(html).toContain('&lt;img')
  })

  it('does not let a URL break out of the href attribute', () => {
    const html = render('[click](https://example.com/"onmouseover="alert(1))')
    // Refused outright or quoted as entities; either way, no second attribute.
    expect(html).not.toContain('onmouseover="alert(1)"')
  })

  it('formats inside a link label', () => {
    expect(render('[a **bold** label](https://example.com)')).toContain(
      '>a <strong>bold</strong> label</a>',
    )
  })

  it('does not italicise the underscores in a URL', () => {
    const html = render('[docs](https://example.com/a_b_c_d)')
    expect(html).toContain('href="https://example.com/a_b_c_d"')
    expect(html).not.toContain('<em>')
  })
})

/** Every output above, parsed as real HTML. Not written per-construct, so a
 *  feature added later is covered the moment its own test calls `render`. */
describe('the parsed output of every case above', () => {
  /** Checked here too, so this describe stands on its own under a name filter. */
  const CORPUS = [
    '<script>alert(1)</script>',
    '<SCRIPT SRC=//evil.com></SCRIPT>',
    '<img src=x onerror=alert(1)>',
    '<svg onload=alert(1)>',
    '<iframe src="javascript:alert(1)"></iframe>',
    '<body onload=alert(1)>',
    '[click](javascript:alert(1))',
    '[click](JaVaScRiPt:alert(1))',
    '[click](java\tscript:alert(1))',
    '[click](data:text/html,<script>alert(1)</script>)',
    '[click](//evil.com)',
    '**bold** <b onclick="alert(1)">x</b>',
    '```\n<script>alert(1)</script>\n```',
    '`<script>alert(1)</script>`',
    '- <script>alert(1)</script>',
    '[<script>alert(1)</script>](https://example.com)',
    '<a href="javascript:alert(1)">click</a>',
    '&lt;script&gt;alert(1)&lt;/script&gt;',
  ]

  /** Scoped to the wrapper, so the document's own html/head/body are not
   *  mistaken for the renderer's output. */
  function parse(html: string): Element {
    const doc = new DOMParser().parseFromString(`<div id="out">${html}</div>`, 'text/html')
    const out = doc.getElementById('out')
    if (!out) throw new Error('the wrapper did not survive parsing')
    return out
  }

  it('contains no script element anywhere', () => {
    for (const html of [...rendered, ...CORPUS.map(render)]) {
      expect(parse(html).querySelectorAll('script')).toHaveLength(0)
    }
  })

  it('contains no element carrying an on* attribute', () => {
    for (const html of [...rendered, ...CORPUS.map(render)]) {
      for (const el of parse(html).querySelectorAll('*')) {
        const handlers = [...el.attributes].map((a) => a.name).filter((n) => n.startsWith('on'))
        expect(handlers, `${el.tagName} in ${html}`).toEqual([])
      }
    }
  })

  it('gives every anchor it did produce a safe scheme, a rel and a target', () => {
    for (const html of [...rendered, ...CORPUS.map(render)]) {
      for (const a of parse(html).querySelectorAll('a')) {
        expect(a.getAttribute('href')).toMatch(/^https?:/)
        expect(a.getAttribute('rel')).toBe('noopener noreferrer nofollow')
        expect(a.getAttribute('target')).toBe('_blank')
      }
    }
  })

  it('produces only the elements this renderer is allowed to produce', () => {
    // The list is the contract: adding to it should be a deliberate edit.
    const allowed = new Set(['P', 'BR', 'STRONG', 'EM', 'CODE', 'PRE', 'UL', 'OL', 'LI', 'A'])
    for (const html of [...rendered, ...CORPUS.map(render)]) {
      for (const el of parse(html).querySelectorAll('*')) {
        expect(allowed, `unexpected <${el.tagName.toLowerCase()}> from ${html}`).toContain(
          el.tagName,
        )
      }
    }
  })
})
