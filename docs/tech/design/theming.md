# The theme pipeline

Ten palettes, eight of them derived from paintings, and none of them
hand-written into a stylesheet. The [user-facing list](../../usage/themes.md)
says which is which; this is how they are produced and why they are not copies.

## How it is wired

Components speak a small semantic vocabulary — `--panel`, `--text`, `--fact`,
`--edge` — and never M3 role names. `scripts/gen-art-themes.mjs` maps each
theme's tokens onto that vocabulary and writes `src/styles/art-themes.css`,
selected by a `[data-art]` attribute on the root element. Adding a theme is a
data change, not a refactor.

```bash
git clone https://github.com/peiqingzhang/art_inspired_design_system_for_AI /tmp/art
cd frontend && node scripts/gen-art-themes.mjs /tmp/art/themes
```

The generated CSS is committed, so the build never depends on the upstream
repository. `src/styles/theme.css` holds what does not vary with the theme: the
spacing scale, motion, the monospace face, and a fallback palette. The default
is the `DEFAULT_ART` constant in `composables/useTheme.ts`.

## The mapping is not a copy

An upstream palette is designed to look like its painting, not to survive a
dense read-only tool, so every derived colour is checked against the surfaces
it will actually sit on and nudged in lightness — hue and saturation held —
until it clears its target. `src/styles/art-themes.audit.md` is regenerated
alongside the CSS and records the worst-case ratio for every pairing.

Four problems the audit caught, and what the generator now does about them:

- **Matisse's ramp runs from a deep red surface to a pale salmon container.**
  No single text colour spans it: black reaches 3.81:1 on one end, white
  2.15:1 on the other. The ramp is chosen, not assumed — first the full one,
  then containers only, and finally the containers washed toward white until
  they can carry AA text *and* a red and an amber that stay apart. Matisse
  lands on a 40% wash and keeps every hue relationship intact.
- **Several paintings hand M3 two near-identical roles.** Hokusai's primary and
  secondary are both blues; Van Gogh's wheat fields a green and a green-teal.
  Facts keep the primary; the dimension takes whichever remaining role sits
  furthest from it, and is then walked in lightness and chroma until the two
  dots are plainly different. Shape still carries the distinction on the canvas
  — facts square, dimensions round — but colour now reinforces it.

  Every other role derives its colour from one of those two at runtime rather
  than adding a token here: a small hue and lightness shift off `--fact` for
  the roles that carry events and keys, off `--dim` for the roles that carry
  context. Twenty palettes times sixteen roles is not a contrast pipeline worth
  running, and shape is the channel doing the real work anyway — it survives
  greyscale, which colour does not. Fact and dimension sit at zero shift, so a
  star schema looks exactly as it did before any other role existed.
- **Upstream harmonises the M3 `error` role into the artwork**, which for Hopper
  and Wang Ximeng lands on an orange indistinguishable from the amber warning.
  In a tool that prints "1 error, 20 warnings" side by side, danger's hue is
  held inside a red band.
- **A chip fill and a label on that fill need different lightnesses of one
  hue.** An amber light enough to read as amber as a fill is too light to be
  text on a pale amber wash. Each soft wash ships its own ink — `--fact-soft`
  with `--on-fact-soft` — the way M3 pairs `error-container` with
  `on-error-container`.

## Where the limit shows

The pairs the generator guards are the ones that share a surface, and Haru
Urara is where that limit shows. Its conformed gold lands 15 from the amber
warning, closer than any other theme manages and well under the ~45 where two
colours stop reading as two. It is kept anyway, because the two are never drawn
together: conformed is a node border on the canvas, where its neighbours are the
fact (83), the dimension (101) and danger (60), while warning is a chip in the
diagnostics pane. The obvious alternative — a burnt orange — buys separation
from warning only by closing on danger, to 37, which *is* a collision on the
canvas. The audit reports the numbers; which ones matter is still a judgement.

Graph hull and legend colours key off the *canvas colour*, not the app's
nominal mode. Light-mode does not mean a light canvas: Matisse's is a deep
rose, and hulls tuned for white paper vanished into it.
