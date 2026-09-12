"""The whole agent, against a real model and a real snapshot."""

import re
from collections.abc import AsyncIterator, Callable, Coroutine
from typing import Any

import pytest

from urara_chat.agent.context_card import ContextCardCache
from urara_chat.agent.pipeline import AgentAnswer, Pipeline
from urara_chat.backend.client import BackendClient
from urara_chat.config import Settings
from urara_chat.llm.factory import build_chat_model
from urara_chat.tools.langchain import to_langchain_tools
from urara_chat.tools.registry import build_tools

# `llm` only, deliberately not `integration`: `make test-chat-integration` runs `-m integration`,
# and a billed test that a backend-only target picks up is one nobody meant to pay for.
pytestmark = pytest.mark.llm

FACT_ORDERS = "ordering/fact_orders"
DIM_CUSTOMERS = "customer_identity/dim_customers"

# A table that exists only in the *other* demo set. Asking the Jaffle Shop snapshot about it must
# come back empty.
FOREIGN_TABLE = "fact_basket_events"

# The codes the demo set is built around, pinned in backend/tests/unit/demo/demo_test.go, each
# with the words an answer is likely to use for it.
DIAGNOSTIC_MARKERS = (
    "unresolved_reference",
    "unresolved reference",
    "cross_domain_reference",
    "cross-domain",
    "cross domain",
    "conformed_drift",
    "conformed",
    "unmatched_join_key",
    "unmatched join",
    "join key",
    "empty_domain",
    "empty domain",
    "isolated_fact",
    "isolated fact",
    "narrative_reference",
    "narrative reference",
    "undocumented_lineage",
    "undocumented lineage",
)

# A refusal, matched by shape rather than by phrase: literal phrases failed on refusals that were
# entirely correct.
REFUSAL_PATTERN = re.compile(
    r"\b(?:can(?:'|\u2019)?t|cannot|could\s?n(?:'|\u2019)?t|could not|unable to|"
    r"is\s?n(?:'|\u2019)?t|is not|does\s?n(?:'|\u2019)?t|does not|do\s?n(?:'|\u2019)?t|"
    r"no|not|none)\b[^.!?]{0,40}?\b"
    r"(?:find|found|locate|exist|document|documented|documentation|present|"
    r"include|included|appears?|available)",
    re.IGNORECASE,
)

# A refusal has nothing to compare, so any markdown table in one is a fabricated column list --
# the single worst thing this agent can produce.
MARKDOWN_TABLE_ROW = re.compile(r"^\s*\|.*\|", re.MULTILINE)

# The refusal should be cheap. Hunting for a table that is not there is exactly the behaviour the
# inventory in the system prompt exists to prevent.
REFUSAL_ITERATION_BUDGET = 3

Ask = Callable[..., Coroutine[Any, Any, AgentAnswer]]


@pytest.fixture(scope="session")
def agent_settings(llm_settings: Settings, backend_url: str, api_token: str) -> Settings:
    return Settings(
        backend_base_url=backend_url,
        backend_api_token=api_token,
        backend_timeout_seconds=60.0,
        llm_provider="vertex",
        vertex_project=llm_settings.vertex_project,
        vertex_location=llm_settings.vertex_location,
        llm_max_output_tokens=2048,
        llm_timeout_seconds=90.0,
    )


@pytest.fixture
async def ask(agent_settings: Settings, snapshot_id: str) -> AsyncIterator[Ask]:
    """Ask the agent one question and get the whole answer back."""
    client = BackendClient(agent_settings)
    model = build_chat_model(agent_settings)
    cache = ContextCardCache(ttl_seconds=agent_settings.context_cache_ttl_seconds)
    pipeline = Pipeline(
        model,
        lambda sid: to_langchain_tools(build_tools(client, sid)),
        cache,
        client,
        model_name=agent_settings.llm_model,
        max_tool_iterations=agent_settings.max_tool_iterations,
    )

    async def run(question: str, sid: str | None = None, language: str = "EN") -> AgentAnswer:
        # No history: every test here is one turn, and a shared transcript would let one test's
        # answer change another's.
        return await pipeline.answer(question, sid or snapshot_id, history=[], language=language)

    yield run
    await client.aclose()


def called(answer: AgentAnswer) -> set[str]:
    return {call["name"] for call in answer.tool_calls}


def mentions(text: str, markers: tuple[str, ...]) -> bool:
    lowered = text.lower()
    return any(marker in lowered for marker in markers)


def refuses(text: str) -> bool:
    return REFUSAL_PATTERN.search(text) is not None


async def test_grain_lookup(ask: Ask) -> None:
    """A claim about a table's contents is retrieved and cited."""
    answer = await ask("List the columns of fact_orders and state its grain.")

    assert FACT_ORDERS in answer.citations
    assert "get_tables" in called(answer)
    assert "order" in answer.text.lower()


async def test_join_traversal(ask: Ask) -> None:
    """Both ends of the join must be cited."""
    answer = await ask("How do I join orders to customers?")

    assert {FACT_ORDERS, DIM_CUSTOMERS} <= set(answer.citations)
    assert called(answer) & {"find_join_paths", "get_neighbourhood"}


async def test_diagnostics(ask: Ask) -> None:
    """Telling people what is wrong with their documentation is what this tool"""
    answer = await ask("What is wrong with this documentation?")

    assert "list_diagnostics" in called(answer)
    assert mentions(answer.text, DIAGNOSTIC_MARKERS), answer.text


async def test_conformed(ask: Ask) -> None:
    """The demo set carries a deliberate conformed drift between the two"""
    answer = await ask("Why are there two dim_date tables?")

    dim_dates = [c for c in answer.citations if c.endswith("dim_date")]
    assert len(dim_dates) >= 2, answer.citations
    assert mentions(answer.text, ("conform", "duplicat", "drift")), answer.text


async def test_refuses_unknown_table(ask: Ask) -> None:
    """A confident description of `fact_unicorns` is a failed phase, whatever"""
    answer = await ask("Describe the fact_unicorns table and list its columns.")

    assert answer.citations == [], "cited something for a table that does not exist"
    assert not MARKDOWN_TABLE_ROW.search(answer.text), f"fabricated a column table:\n{answer.text}"
    assert refuses(answer.text), answer.text
    assert answer.iterations <= REFUSAL_ITERATION_BUDGET, (
        f"spent {answer.iterations} iterations hunting for a table that is not there"
    )


def has_japanese(text: str) -> bool:
    return any(
        "\u3040" <= c <= "\u30ff"  # hiragana and katakana
        or "\u4e00" <= c <= "\u9fff"  # CJK ideographs
        or "\uff66" <= c <= "\uff9f"  # halfwidth katakana
        for c in text
    )


async def test_bilingual(ask: Ask) -> None:
    """Citations are IDs, so they are language-independent."""
    answer = await ask("fact_orders の粒度は？", language="JA")

    assert has_japanese(answer.text), answer.text
    assert FACT_ORDERS in answer.citations


async def test_stays_in_snapshot(ask: Ask, other_snapshot_id: str) -> None:
    """The binding from Phase 02, proved end to end."""
    assert other_snapshot_id, "the second snapshot must exist for this test to mean anything"

    answer = await ask(f"Describe the {FOREIGN_TABLE} table and list its columns.")

    assert answer.citations == [], "answered about a table from another snapshot"
    assert refuses(answer.text), answer.text
