# Design notes

How each stage actually works — the rules, and why they are those rules. The
[architecture notes](../architecture/) say what the pieces are; these say what
is inside them.

| Document | Covers |
|---|---|
| [Parsing](parsing.md) | Sections, classification, reading a table document, determinism |
| [Resolution](resolution.md) | Conformed authority, join-key orientation, edge normalisation, source folding |
| [The theme pipeline](theming.md) | The generator, the contrast audit, and where its limit shows |
| [The frontend](frontend.md) | Layout engines, hulls and petals, role colour, state |
| [Testing](testing.md) | What each layer covers and how the integration suites isolate |
| [CI](ci.md) | The six workflows and running them locally with act |
