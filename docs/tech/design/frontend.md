# The frontend

Vue 3 with Pinia and Cytoscape, no Tailwind, no component library. The canvas
is the app; everything else is a panel around it.

```
src/
  api/client.ts        thin fetch wrapper, one method per route
  api/types.ts         the response shapes, mirroring the Go structs
  stores/workspace.ts  the single Pinia store: snapshot, filters, selection
  graph/layout.ts      the three layout engines and what each is for
  graph/hull.ts        convex hulls for the domain outlines
  graph/petal-overlap.ts  keeping the sakura petals off each other
  graph/palette.ts     role colour derivation
  graph/roles.ts       role → shape and family
  components/          canvas, detail, sidebar, search, diagnostics, theme
  composables/         directory picker, theme, role palette
  styles/              tokens, base CSS, generated art themes
```

## Reading a directory

`useDirectoryPicker` reads the chosen folder in the browser — the File System
Access API where it exists, a `webkitdirectory` input where it does not — and
posts the markdown as text. The server never touches the user's filesystem, and
nothing is written back to disk.

The paths in that upload are the one piece of genuinely untrusted input the
backend takes, which is why they are normalised before anything else looks at
them.

## The three layouts

No single layout suits every way a warehouse gets modelled, which is the whole
reason there is a choice:

- **Force** (fcose) has no reading direction, which is right for a star schema —
  nothing about a fact and its dimensions is upstream or downstream of anything
  else. It is also the only layout that groups, because fcose lays compound
  parents out as units and never lets two of them intersect; that is what keeps
  the domain hulls apart.
- **Layered** (dagre) ranks nodes along the direction of their joins. A
  snowflake's normalisation depth and a Data Vault's hub-link-satellite tiers
  are both genuinely layered structures, and seeing them as a flat cloud loses
  the one thing worth reading.
- **Radial** (concentric) puts the busiest tables in the middle and works
  outwards by degree — a star schema drawn the way it is usually drawn on a
  whiteboard, and it puts Data Vault hubs where a reader expects them.

Neither dagre nor concentric understands compound nodes, so grouping is a
force-mode feature and the UI disables the toggle rather than silently dropping
the clusters.

## Hulls and petals

Domain outlines are convex hulls in screen coordinates, where y grows
downwards — so "clockwise" in that code means clockwise as drawn rather than as
a mathematician would have it.

The Haru Urara theme draws those hulls as sakura petals, and a petal is the one
shape that does not inherit the compound layout's guarantee. It circumscribes
the same nodes far more loosely — roughly twice the area of the hull it holds —
so it reaches past the box that was keeping its neighbours away, and two
adjacent domains end up sharing ink.

A petal cannot simply be shrunk, because it still has to contain its own nodes.
So the only way to separate two of them is to move the nodes:
`resolvePetalOverlaps` returns a translation per cluster and the caller moves
each cluster as a unit. Translating rigidly carries the petal along with it, so
containment is preserved for free and the layout *inside* a domain stays exactly
as fcose solved it.

## Colour by role

Only two role colours are real tokens: `--fact` and `--dim`. Every other role
derives from one of them at runtime with a small hue and lightness shift — off
`--fact` for the roles that carry events and keys, off `--dim` for the roles
that carry context — which is what lets an unanticipated role like `anchor`
arrive with a colour of its own without a token per theme.

Shape carries the distinction that has to survive greyscale: facts square,
dimensions round, and a shape per family beyond that.

## State

One Pinia store holds the workspace: the current snapshot, the filters, the
selection and the layout mode. Filters are derived state over the graph the API
returned rather than refetches, so toggling a domain is instant; the queries
that genuinely need the server — neighbourhood, paths, lineage, search — go
through the client.

The API base is `/api/v1` by default and the frontend needs no runtime config,
because nginx proxies `/api` to the backend and the browser stays on one
origin. When `API_TOKEN` is set, `ApiTokenGate` prompts for it and the client keeps it
in `localStorage`.
