"""The snapshot inventory that goes into the system prompt.

This is what lets the agent answer most questions in one tool call instead of
searching blindly: it can see the whole inventory before deciding where to look.

Rendered as text rather than JSON. The punctuation is pure cost and it is paid
on every turn, and a model reads a plain listing at least as well.
"""

from __future__ import annotations

import asyncio
import time
from collections import OrderedDict
from collections.abc import Callable

from urara_chat.backend.client import BackendClient
from urara_chat.backend.models import SnapshotContext

# A domain description is a paragraph of orientation. One sentence of it is
# enough to choose by, and the whole thing is one list_domains call away.
_MAX_DOMAIN_SUMMARY = 160

# An empty grain renders as this rather than as nothing, which would leave the
# column blank and the row misaligned against its neighbours.
_NO_GRAIN = "—"

# Snapshots a single pod is likely to be asked about at once. Bounded so a
# long-running pod that has seen many of them does not grow without limit.
MAX_CACHED_CARDS = 32


def render_context_card(ctx: SnapshotContext) -> str:
    """The whole snapshot as a compact inventory for the system prompt.

    Text rather than JSON: the punctuation is pure cost, and this is paid for
    on every turn. It is an inventory, not a description -- the prompt says
    so, because a model given names and grains will otherwise narrate what it
    imagines the tables contain.
    """
    lines: list[str] = [*_header(ctx), "", *_domains(ctx), "", *_tables(ctx)]
    return "\n".join(lines).strip() + "\n"


def _header(ctx: SnapshotContext) -> list[str]:
    project = ctx.snapshot.project.project
    name = project.name or ctx.snapshot.name or ctx.snapshot.id
    version = f" (v{project.version})" if project.version else ""
    description = f" — {project.description}" if project.description else ""

    lines = [f"PROJECT: {name}{version}{description}"]

    i18n = ctx.snapshot.project.internationalization
    others = [lang for lang in i18n.supported if lang != i18n.primary]
    if others:
        # Omitted entirely for a single-language project: a line saying there is
        # one language tells the model nothing it can act on.
        lines.append(f"LANGUAGES: primary {i18n.primary}, also {', '.join(others)}")

    lines.append(f"COUNTS: {_counts(ctx)}")
    return lines


def _counts(ctx: SnapshotContext) -> str:
    stats = ctx.snapshot.stats
    parts = [
        f"{stats.domains} domains",
        f"{stats.tables} tables",
        f"{stats.columns} columns",
        f"{stats.relationships} relationships",
    ]

    total = sum(ctx.diagnostics.values())
    if total:
        # Named in severity order rather than the map's, so the number that
        # matters is first.
        breakdown = ", ".join(
            f"{ctx.diagnostics.get(level, 0)} {level}"
            for level in ("error", "warning", "info")
            if ctx.diagnostics.get(level, 0)
        )
        parts.append(f"{total} diagnostics ({breakdown})")
    return ", ".join(parts)


def _domains(ctx: SnapshotContext) -> list[str]:
    lines = ["DOMAINS"]
    for domain in ctx.domains:
        # A domain with no tables is marked rather than dropped. The demo sets
        # contain one, and it is a real finding about the documentation rather
        # than an absence worth hiding.
        detail = (
            "(no tables documented)" if not domain.table_count else _summarise(domain.description)
        )
        lines.append(f"  {domain.id} — {detail}" if detail else f"  {domain.id}")
    return lines


def _summarise(text: str) -> str:
    """One sentence, or 160 characters, whichever is shorter."""
    collapsed = " ".join(text.split())
    sentence, stop, _ = collapsed.partition(". ")
    first = sentence + "." if stop else collapsed
    if len(first) <= _MAX_DOMAIN_SUMMARY:
        return first
    return first[:_MAX_DOMAIN_SUMMARY].rstrip() + "…"


def _tables(ctx: SnapshotContext) -> list[str]:
    if ctx.truncated:
        # Saying nothing would leave the model believing the model has no
        # tables, and it would answer confidently on that basis.
        return [
            f"TABLES: {ctx.snapshot.stats.tables} tables, too many to list here. "
            "Use search_model to find them by name or description, then get_tables "
            "to read them."
        ]

    lines = ["TABLES  (id | kind | grain | columns)"]
    for table in ctx.tables:
        row = f"  {table.id} | {table.kind} | {table.grain or _NO_GRAIN} | {table.column_count}"
        if table.conformed:
            row += " | conformed"
        lines.append(row)
    return lines


class ContextCardCache:
    """Rendered cards, by snapshot ID.

    A snapshot is immutable once ingested, so refetching it every turn is
    pure latency. Keyed by concrete snapshot ID -- never by "latest", which
    is not a snapshot but a question about which one is newest.
    """

    def __init__(
        self,
        ttl_seconds: float,
        *,
        max_entries: int = MAX_CACHED_CARDS,
        clock: Callable[[], float] = time.monotonic,
    ) -> None:
        self._ttl = ttl_seconds
        self._max_entries = max_entries
        # Injected so a test can expire an entry without sleeping.
        self._clock = clock
        self._cards: OrderedDict[str, tuple[float, str]] = OrderedDict()
        # One lock per snapshot rather than one for the cache: two turns on
        # different snapshots have no reason to wait for each other's fetch,
        # and the fetch is the slow part. The registry itself is only ever
        # mutated from the event loop, so it needs no lock of its own.
        self._locks: dict[str, asyncio.Lock] = {}

    async def get(self, client: BackendClient, snapshot_id: str) -> str:
        """The card for a snapshot, fetching and rendering on a miss."""
        if snapshot_id == "latest":
            # By the time this is called the ID is resolved. Caching under the
            # alias would serve a stale model after a re-ingest, and the reader
            # would never know the answer was about the wrong snapshot.
            raise ValueError(
                "ContextCardCache requires a concrete snapshot ID, not 'latest'; resolve it first"
            )

        cached = self._live(snapshot_id)
        if cached is not None:
            return cached

        async with self._locks.setdefault(snapshot_id, asyncio.Lock()):
            # Checked again inside the lock: two turns starting together on a
            # cold cache should cost one fetch, not two.
            cached = self._live(snapshot_id)
            if cached is not None:
                return cached

            card = render_context_card(await client.get_context(snapshot_id))
            self._store(snapshot_id, card)
            return card

    def _live(self, snapshot_id: str) -> str | None:
        entry = self._cards.get(snapshot_id)
        if entry is None:
            return None
        expires_at, card = entry
        if self._clock() >= expires_at:
            del self._cards[snapshot_id]
            return None
        self._cards.move_to_end(snapshot_id)
        return card

    def _store(self, snapshot_id: str, card: str) -> None:
        self._cards[snapshot_id] = (self._clock() + self._ttl, card)
        self._cards.move_to_end(snapshot_id)
        while len(self._cards) > self._max_entries:
            evicted, _ = self._cards.popitem(last=False)
            self._locks.pop(evicted, None)
