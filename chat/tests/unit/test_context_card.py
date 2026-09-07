"""The context card and its cache.

The card is paid for on every turn, so what matters is that it stays small and
says true things: a domain with no tables is a real finding rather than an
absence to hide, and a truncated table list must say so or the model will answer
as though the model has no tables.

The cache exists because a snapshot is immutable. What is worth testing is that
it never caches under `latest`, and that two turns starting together cost one
fetch.
"""

import asyncio
import json
from pathlib import Path
from typing import Any

import pytest

from urara_chat.agent.context_card import (
    MAX_CACHED_CARDS,
    ContextCardCache,
    render_context_card,
)
from urara_chat.backend.models import SnapshotContext

FIXTURES = Path(__file__).parent / "fixtures"


def jaffle() -> SnapshotContext:
    """The real /context response captured in 02.3."""
    return SnapshotContext.model_validate(json.loads((FIXTURES / "context.json").read_text()))


def context(**over: Any) -> SnapshotContext:
    base: dict[str, Any] = {
        "snapshot": {
            "id": "snap-1",
            "createdAt": "2026-01-01T00:00:00Z",
            "stats": {"domains": 1, "tables": 1, "columns": 4, "relationships": 2},
            "project": {
                "project": {"name": "Demo", "version": "1.0", "description": "a demo"},
                "internationalization": {"primary": "EN", "supported": ["EN"], "type": "inline"},
            },
        },
        "domains": [
            {"id": "ordering", "title": "Ordering", "description": "Orders.", "tableCount": 1}
        ],
        "tables": [
            {
                "id": "ordering/fact_orders",
                "name": "fact_orders",
                "domainId": "ordering",
                "kind": "fact",
                "grain": "One row per order.",
                "columnCount": 4,
            }
        ],
        "diagnostics": {"error": 0, "warning": 0, "info": 0},
        "truncated": False,
    }
    return SnapshotContext.model_validate(base | over)


class TestTheRenderedShape:
    def test_it_is_text_not_json(self) -> None:
        """JSON spends tokens on punctuation, and this is paid every turn."""
        card = render_context_card(jaffle())
        with pytest.raises(json.JSONDecodeError):
            json.loads(card)

    def test_the_header_carries_project_and_counts(self) -> None:
        card = render_context_card(jaffle())
        assert card.startswith("PROJECT: jaffle-shop-ddd (v0.1.0) — ")
        assert "COUNTS: 6 domains, 10 tables, 80 columns" in card

    def test_diagnostics_are_broken_down_by_severity(self) -> None:
        assert "16 diagnostics (1 error, 11 warning, 4 info)" in render_context_card(jaffle())

    def test_the_blocks_are_present(self) -> None:
        card = render_context_card(jaffle())
        assert "DOMAINS" in card
        assert "TABLES  (id | kind | grain | columns)" in card

    def test_a_real_table_appears_with_its_grain(self) -> None:
        card = render_context_card(jaffle())
        assert "ordering/fact_orders | fact | One row per order. | 15" in card

    def test_a_conformed_table_is_marked(self) -> None:
        assert (
            "shared_kernel/dim_date | dimension | One row per calendar date. | 10 | conformed"
            in (render_context_card(jaffle()))
        )

    def test_an_unconformed_table_carries_no_marker(self) -> None:
        for line in render_context_card(jaffle()).splitlines():
            if "dim_customers" in line:
                assert "conformed" not in line

    def test_no_column_lists(self) -> None:
        """That is what get_tables is for; naming every column here would cost
        more than the whole card."""
        assert "order_id" not in render_context_card(jaffle())


class TestDomains:
    def test_a_domain_with_no_tables_is_marked_not_dropped(self) -> None:
        """The demo sets contain one, and it is a real finding about the
        documentation rather than an absence worth hiding."""
        card = render_context_card(jaffle())
        assert "delivery_logistics — (no tables documented)" in card

    def test_a_description_is_cut_to_one_sentence(self) -> None:
        ctx = context(
            domains=[
                {
                    "id": "ordering",
                    "tableCount": 1,
                    "description": "First sentence. Second sentence that should not appear.",
                }
            ]
        )
        card = render_context_card(ctx)
        assert "ordering — First sentence." in card
        assert "Second sentence" not in card

    def test_a_long_single_sentence_is_capped(self) -> None:
        ctx = context(domains=[{"id": "ordering", "tableCount": 1, "description": "x" * 400}])
        line = next(
            ln for ln in render_context_card(ctx).splitlines() if ln.startswith("  ordering")
        )
        assert len(line) < 200
        assert line.endswith("…")


class TestLanguages:
    def test_a_single_language_project_omits_the_line(self) -> None:
        """A line saying there is one language tells the model nothing it can
        act on."""
        assert "LANGUAGES" not in render_context_card(jaffle())

    def test_a_bilingual_project_names_the_others(self) -> None:
        ctx = context(
            snapshot={
                "id": "snap-1",
                "createdAt": "2026-01-01T00:00:00Z",
                "stats": {"domains": 1, "tables": 1},
                "project": {
                    "project": {"name": "Demo"},
                    "internationalization": {
                        "primary": "EN",
                        "supported": ["EN", "JA"],
                        "type": "inline",
                    },
                },
            }
        )
        assert "LANGUAGES: primary EN, also JA" in render_context_card(ctx)


class TestGrain:
    def test_an_empty_grain_renders_as_a_placeholder(self) -> None:
        """An empty column would misalign the row against its neighbours."""
        ctx = context(
            tables=[
                {
                    "id": "d/t",
                    "name": "t",
                    "domainId": "d",
                    "kind": "fact",
                    "grain": "",
                    "columnCount": 3,
                }
            ]
        )
        assert "  d/t | fact | — | 3" in render_context_card(ctx)


class TestTruncation:
    def test_the_table_block_is_replaced_and_says_why(self) -> None:
        """Silence would leave the model believing the model has no tables, and
        it would answer confidently on that basis."""
        ctx = context(truncated=True, tables=[])
        card = render_context_card(ctx)

        assert "too many to list here" in card
        assert "TABLES  (id | kind" not in card

    def test_it_names_the_tools_to_use_instead(self) -> None:
        card = render_context_card(context(truncated=True, tables=[]))
        assert "search_model" in card
        assert "get_tables" in card

    def test_it_reports_how_many(self) -> None:
        ctx = context(
            truncated=True,
            tables=[],
            snapshot={
                "id": "snap-1",
                "createdAt": "2026-01-01T00:00:00Z",
                "stats": {"domains": 6, "tables": 612},
                "project": {"project": {"name": "Big"}},
            },
        )
        assert "612 tables" in render_context_card(ctx)


class CountingClient:
    """Counts fetches, so "fetched once" is an assertion rather than a hope."""

    def __init__(self, delay: float = 0.0) -> None:
        self.calls = 0
        self.delay = delay

    async def get_context(self, snapshot_id: str) -> SnapshotContext:
        self.calls += 1
        if self.delay:
            await asyncio.sleep(self.delay)
        return jaffle()


class Clock:
    """A clock a test can move, so a TTL can expire without sleeping."""

    def __init__(self) -> None:
        self.now = 0.0

    def __call__(self) -> float:
        return self.now


class TestTheCache:
    async def test_two_calls_with_the_same_id_fetch_once(self) -> None:
        client = CountingClient()
        cache = ContextCardCache(ttl_seconds=300.0)

        first = await cache.get(client, "snap-1")  # type: ignore[arg-type]
        second = await cache.get(client, "snap-1")  # type: ignore[arg-type]

        assert client.calls == 1
        assert first == second

    async def test_different_snapshots_are_cached_separately(self) -> None:
        client = CountingClient()
        cache = ContextCardCache(ttl_seconds=300.0)

        await cache.get(client, "snap-1")  # type: ignore[arg-type]
        await cache.get(client, "snap-2")  # type: ignore[arg-type]

        assert client.calls == 2

    async def test_it_refetches_after_the_ttl(self) -> None:
        client = CountingClient()
        clock = Clock()
        cache = ContextCardCache(ttl_seconds=300.0, clock=clock)

        await cache.get(client, "snap-1")  # type: ignore[arg-type]
        clock.now = 299.0
        await cache.get(client, "snap-1")  # type: ignore[arg-type]
        assert client.calls == 1, "still live before the TTL"

        clock.now = 301.0
        await cache.get(client, "snap-1")  # type: ignore[arg-type]
        assert client.calls == 2

    async def test_latest_is_refused(self) -> None:
        """Caching under the alias would serve a stale model after a re-ingest,
        and the reader would never know the answer was about the wrong one."""
        cache = ContextCardCache(ttl_seconds=300.0)

        with pytest.raises(ValueError, match="concrete snapshot ID"):
            await cache.get(CountingClient(), "latest")  # type: ignore[arg-type]

    async def test_it_does_not_grow_without_limit(self) -> None:
        client = CountingClient()
        cache = ContextCardCache(ttl_seconds=300.0)

        for i in range(MAX_CACHED_CARDS + 1):
            await cache.get(client, f"snap-{i}")  # type: ignore[arg-type]

        assert len(cache._cards) == MAX_CACHED_CARDS
        assert "snap-0" not in cache._cards, "the oldest should have been evicted"

    async def test_two_concurrent_cold_gets_fetch_once(self) -> None:
        """Two turns starting together must not both pay for the fetch."""
        client = CountingClient(delay=0.05)
        cache = ContextCardCache(ttl_seconds=300.0)

        first, second = await asyncio.gather(
            cache.get(client, "snap-1"),  # type: ignore[arg-type]
            cache.get(client, "snap-1"),  # type: ignore[arg-type]
        )

        assert client.calls == 1
        assert first == second

    async def test_concurrent_gets_on_different_snapshots_do_not_block_each_other(
        self,
    ) -> None:
        """One lock per snapshot rather than one for the cache: two turns on
        different snapshots have no reason to wait for each other's fetch."""
        client = CountingClient(delay=0.1)
        cache = ContextCardCache(ttl_seconds=300.0)

        started = asyncio.get_running_loop().time()
        await asyncio.gather(
            cache.get(client, "snap-1"),  # type: ignore[arg-type]
            cache.get(client, "snap-2"),  # type: ignore[arg-type]
        )
        elapsed = asyncio.get_running_loop().time() - started

        assert client.calls == 2
        assert elapsed < 0.18, "the two fetches were serialised"


class TestTheCacheIsBoundedIncludingItsLocks:
    """The locks used to live in a dict beside the cards, pruned only when a
    card was evicted -- so a failed fetch or an expired card left its lock
    behind for the life of the process. 500 failing fetches left 500 locks and
    no cards."""

    async def test_failing_fetches_do_not_accumulate(self) -> None:
        class Failing:
            async def get_context(self, sid: str) -> SnapshotContext:
                raise RuntimeError("the backend is down")

        cache = ContextCardCache(ttl_seconds=300.0, max_entries=4)

        for i in range(500):
            with pytest.raises(RuntimeError):
                await cache.get(Failing(), f"snap-{i}")  # type: ignore[arg-type]

        assert len(cache._cards) <= 4

    async def test_an_expired_entry_does_not_leak(self) -> None:
        client = CountingClient()
        clock = Clock()
        cache = ContextCardCache(ttl_seconds=10.0, max_entries=4, clock=clock)

        for i in range(50):
            clock.now = i * 100.0
            await cache.get(client, f"snap-{i}")  # type: ignore[arg-type]

        assert len(cache._cards) <= 4
