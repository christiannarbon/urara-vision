"""The whole agent, against a real model and a real snapshot.

These are the tests that cannot be faked. Every unit test in the phase asserts
that the machinery does what it was told; only these say whether the thing
answers, and the one that matters most says whether it declines to.

**Assertions are on citations and tool calls, never on prose.** A model's
wording drifts between versions, and a test pinning it fails for no reason, gets
deleted, and takes its real assertion with it. What must not drift is which
tables an answer rests on and which tools it reached for -- those are the
contract between the model and the retrieval layer, and a regression in either
is a regression in the product.

Marked `llm`: they need a key and a running backend, they are billed, and they
never run in CI. `make test-chat` does not reach this file.
"""

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

# `llm` only, deliberately not `integration`: `make test-chat-integration` runs
# `-m integration`, and a billed test that a backend-only target picks up is one
# nobody meant to pay for. The backend fixtures skip this file on their own when
# CHAT_TEST_BACKEND_URL is unset.
pytestmark = pytest.mark.llm

FACT_ORDERS = "ordering/fact_orders"
DIM_CUSTOMERS = "customer_identity/dim_customers"

# A table that exists only in the *other* demo set. Asking the Jaffle Shop
# snapshot about it must come back empty.
FOREIGN_TABLE = "fact_basket_events"

# The codes the demo set is built around, pinned in
# backend/tests/unit/demo/demo_test.go, each with the words an answer is likely
# to use for it. A model may name the code or describe it, and either counts as
# having found a real finding -- what is being tested is that it reported
# something the checker actually reported, not how it phrased it.
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

# What the prompt's "WHEN IT IS NOT DOCUMENTED" section reliably produces.
#
# Matched by *shape* rather than by phrase: a negation followed, in the same
# sentence, by a word about finding or documenting. A list of literal phrases
# was here first and it failed on refusals that were entirely correct -- "I
# can't find a table named X" misses "cannot find", and "it might not be
# documented in this model" misses "not documented" because of the "be" in the
# middle. Every one of those was the agent behaving perfectly and the test
# disagreeing about wording, which is how a test earns deletion and takes its
# real assertion with it.
#
# The assertion that carries the weight is `citations == []`. This one only
# guards against an empty citation list that arrived with a confident answer
# anyway, so it should be loose about words and strict about meaning.
REFUSAL_PATTERN = re.compile(
    r"\b(?:can(?:'|\u2019)?t|cannot|could\s?n(?:'|\u2019)?t|could not|unable to|"
    r"is\s?n(?:'|\u2019)?t|is not|does\s?n(?:'|\u2019)?t|does not|do\s?n(?:'|\u2019)?t|"
    r"no|not|none)\b[^.!?]{0,40}?\b"
    r"(?:find|found|locate|exist|document|documented|documentation|present|"
    r"include|included|appears?|available)",
    re.IGNORECASE,
)

# A refusal has nothing to compare, so any markdown table in one is a fabricated
# column list -- the single worst thing this agent can produce.
MARKDOWN_TABLE_ROW = re.compile(r"^\s*\|.*\|", re.MULTILINE)

# The refusal should be cheap. Hunting for a table that is not there is exactly
# the behaviour the inventory in the system prompt exists to prevent.
REFUSAL_ITERATION_BUDGET = 3

Ask = Callable[..., Coroutine[Any, Any, AgentAnswer]]


@pytest.fixture(scope="session")
def agent_settings(llm_settings: Settings, backend_url: str, api_token: str) -> Settings:
    """One Settings that can both reach the backend and pay a provider.

    `llm_settings` is depended on for its skip and for the project it resolved,
    so this suite is gated like every other billed test. Its 512-token output
    cap is not reused: that number was picked for a one-word smoke test, and
    Gemini 2.5 spends output tokens on reasoning before it emits anything, so a
    real two-paragraph answer comes back truncated at MAX_TOKENS for a reason
    that looks nothing like a token limit.
    """
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
    """Ask the agent one question and get the whole answer back.

    One pipeline, built the way the lifespan builds it: the tools are a
    *factory* keyed on the snapshot, not a fixed list. Each tool closes over the
    snapshot it reads, so a graph compiled for the Jaffle Shop set cannot answer
    about another -- which is the binding `test_stays_in_snapshot` proves holds,
    and which the pipeline maintains itself by compiling one graph per snapshot.

    Passing a list here instead of a factory is how this suite came to raise
    "'list' object is not callable" on every test without anyone noticing: it
    was gated on an API key nobody had set, so it skipped rather than failed.
    """
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
        # No history: every test here is one turn, and a shared transcript would
        # let one test's answer change another's.
        return await pipeline.answer(question, sid or snapshot_id, history=[], language=language)

    yield run
    await client.aclose()


def called(answer: AgentAnswer) -> set[str]:
    return {call["name"] for call in answer.tool_calls}


def mentions(text: str, markers: tuple[str, ...]) -> bool:
    lowered = text.lower()
    return any(marker in lowered for marker in markers)


def refuses(text: str) -> bool:
    """Whether an answer declines rather than describes."""
    return REFUSAL_PATTERN.search(text) is not None


# --- the four grounded cases ------------------------------------------------


async def test_grain_lookup(ask: Ask) -> None:
    """A claim about a table's contents is retrieved and cited.

    The question asks for columns as well as grain, and the columns are the
    part that matters. The context card is an inventory: it carries every
    table's id, kind, grain and column *count*, so "what is the grain of
    fact_orders?" is answered correctly from the card alone, with no tool call
    and nothing to cite -- which proves only that the card was read. Column
    names are not in it, so answering this needs a real lookup.

    That shortcut is the agent working as designed, not a defect. What would be
    a defect is a claim the card cannot support arriving uncited, which is what
    this test is here to catch.
    """
    answer = await ask("List the columns of fact_orders and state its grain.")

    assert FACT_ORDERS in answer.citations
    assert "get_tables" in called(answer)
    assert "order" in answer.text.lower()


async def test_join_traversal(ask: Ask) -> None:
    """Both ends of the join must be cited. An answer that names only the fact
    has described half a join and linked the reader to half an answer."""
    answer = await ask("How do I join orders to customers?")

    assert {FACT_ORDERS, DIM_CUSTOMERS} <= set(answer.citations)
    assert called(answer) & {"find_join_paths", "get_neighbourhood"}


async def test_diagnostics(ask: Ask) -> None:
    """Telling people what is wrong with their documentation is what this tool
    is for, and the findings must be the checker's, not the model's."""
    answer = await ask("What is wrong with this documentation?")

    assert "list_diagnostics" in called(answer)
    assert mentions(answer.text, DIAGNOSTIC_MARKERS), answer.text


async def test_conformed(ask: Ask) -> None:
    """The demo set carries a deliberate conformed drift between the two
    dim_date tables. Both have to be cited for the answer to be checkable."""
    answer = await ask("Why are there two dim_date tables?")

    dim_dates = [c for c in answer.citations if c.endswith("dim_date")]
    assert len(dim_dates) >= 2, answer.citations
    assert mentions(answer.text, ("conform", "duplicat", "drift")), answer.text


# --- the one that matters ---------------------------------------------------


async def test_refuses_unknown_table(ask: Ask) -> None:
    """A confident description of `fact_unicorns` is a failed phase, whatever
    else passes.

    If this fails, the fix is the prompt. Weakening it would leave the product
    free to invent columns, which is the failure that makes the whole tool worse
    than no tool -- fluent prose about a table nobody has is more damaging than
    silence.
    """
    answer = await ask("Describe the fact_unicorns table and list its columns.")

    assert answer.citations == [], "cited something for a table that does not exist"
    assert not MARKDOWN_TABLE_ROW.search(answer.text), f"fabricated a column table:\n{answer.text}"
    assert refuses(answer.text), answer.text
    assert answer.iterations <= REFUSAL_ITERATION_BUDGET, (
        f"spent {answer.iterations} iterations hunting for a table that is not there"
    )


# --- language and snapshot binding ------------------------------------------


def has_japanese(text: str) -> bool:
    """Hiragana, katakana or CJK ideographs. A codepoint check rather than a
    word list: what is being tested is the language, not the vocabulary."""
    return any(
        "\u3040" <= c <= "\u30ff"  # hiragana and katakana
        or "\u4e00" <= c <= "\u9fff"  # CJK ideographs
        or "\uff66" <= c <= "\uff9f"  # halfwidth katakana
        for c in text
    )


async def test_bilingual(ask: Ask) -> None:
    """Citations are IDs, so they are language-independent. An answer that
    changes its citations with its language has built them from the prose."""
    answer = await ask("fact_orders の粒度は？", language="JA")

    assert has_japanese(answer.text), answer.text
    assert FACT_ORDERS in answer.citations


async def test_stays_in_snapshot(ask: Ask, other_snapshot_id: str) -> None:
    """The binding from Phase 02, proved end to end.

    A second set is ingested alongside the first, and the first is asked about a
    table only the second has. No unit test can show this: the tools close over
    a snapshot, and whether that closure actually holds through the model, the
    tool loop and the citation pass is only visible against a real backend
    holding both.
    """
    assert other_snapshot_id, "the second snapshot must exist for this test to mean anything"

    answer = await ask(f"Describe the {FOREIGN_TABLE} table and list its columns.")

    assert answer.citations == [], "answered about a table from another snapshot"
    assert refuses(answer.text), answer.text
