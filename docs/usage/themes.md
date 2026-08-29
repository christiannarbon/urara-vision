# Themes

Ten themes: two of the app's own, and eight derived from paintings, taken from
the Material 3 token sets in [art_inspired_design_system_for_AI][art]. The app
is **light-mode only** — a theme is a single palette, and there is no dark
variant, no `prefers-color-scheme` block and no mode toggle.

| Theme | Canvas | Display / body face |
|---|---|---|
| **Haru Urara** — sakura pink & spring sky | blossom paper | M PLUS Rounded 1c / Nunito Sans |
| **Studio Paper** — teal & burnt orange | warm paper | Inter |
| Cézanne — *Mont Sainte-Victoire* | blue-grey | Bitter / Nunito Sans |
| Hokusai — *The Great Wave* | sand | Noto Serif JP / Inter |
| Hopper — *Nighthawks* | brass | DM Sans / IBM Plex Sans |
| Matisse — *The Red Studio* | rose | Space Grotesk / DM Sans |
| Monet — *Water Lilies* | misty blue | Lora / Source Serif 4 |
| Van Gogh — *Green Wheat Fields* | pale green | Lora / Source Sans 3 |
| Van Gogh — *Irises* | ochre | Archivo Black / Work Sans |
| Wang Ximeng — *A Thousand Li of Rivers and Mountains* | parchment | Noto Serif Display / Noto Sans |

**Haru Urara** is the default and the one the app is named for. Sakura pink
carries the fact role, a clear spring sky the dimensions, and the sunlit gold of
her accessories the conformed markers, all on blossom-tinted paper — with the
roundest corners and the only rounded display face in the set.

The pink is deepened well past her actual hair colour, which is the one place
the theme argues with its source. `--fact` is a ten-pixel node square and also
the accent behind every button and focus ring, so it owes 4:1 on white; a true
sakura pink manages about 1.6:1 and the contrast pass would have dragged it down
there regardless. Choosing the deeper pink up front keeps the hue relationships
deliberate instead of derived.

**Studio Paper** is the palette the app shipped with: warm paper, a teal fact
colour, a burnt-orange dimension colour. Neither house palette is hand-written
into the stylesheet — both are expressed in the same M3 roles the paintings use
and go through the identical pipeline, so they carry the same guarantees.

The choice persists in `localStorage`. Webfonts load per theme rather than
upfront — fifteen families across ten themes is far too much to ship on first
paint — and every family chains onto a fallback stack of the same species, so a
theme reads correctly before its font lands or if the network never delivers it.

None of these palettes is a straight copy of its painting: every derived colour
is checked against the surfaces it will actually sit on and nudged until it
clears a contrast target. What that pipeline does, and the four problems that
shaped it, is in the [theming design notes](../tech/design/theming.md).

[art]: https://github.com/peiqingzhang/art_inspired_design_system_for_AI
