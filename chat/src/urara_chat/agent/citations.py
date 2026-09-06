"""Working out what an answer actually rested on.

**The model does not write the citation list.** A model asked to name its
sources will name plausible ones, which is the failure this whole design exists
to prevent. Instead the pipeline collects every table ID that came back from a
tool during the turn and keeps the ones the answer mentions. The model can
neither invent a citation -- it never looked the table up -- nor pad the list,
because an ID it did not mention is not something the answer rests on.

Pure: no model, no client, no I/O. That is what makes it exhaustively testable,
and it is the function most worth testing in the service.
"""

from __future__ import annotations

import re
from collections.abc import Sequence
from typing import Any

# A table ID is `domain/table`: exactly one slash, each half starting with a
# letter, digit or underscore. Source model IDs like `jaffle_shop.stg_orders`
# carry no slash and so are never collected -- they are not tables and cannot be
# opened from a citation chip.
_TABLE_ID = re.compile(r"^[a-z0-9_][a-z0-9_.-]*/[a-z0-9_][a-z0-9_.-]*$", re.IGNORECASE)

# Keys whose string value is an identifier rather than prose. Deliberately not
# `description`, `grain` or `message`: a description that happens to mention
# `domain/table` is a coincidence, not a retrieval, and citing it would claim
# the answer rested on something never looked up.
_ID_KEYS = frozenset({"id", "tableId", "from", "to", "source", "target"})

# Keys holding a list of bare IDs. `JoinPath.tables` is a list of strings rather
# than of objects, so its elements never pass under an `id` key.
_ID_LIST_KEYS = frozenset({"tables"})

# Tool results are a handful of levels deep at most. The cap is a guard against
# a malformed or adversarial result hanging a turn, not a real limit.
_MAX_DEPTH = 12

MAX_CITATIONS = 20


def extract_citations(tool_results: Sequence[Any], answer: str) -> list[str]:
    """Table IDs the answer drew on.

    Every ID that appeared in a tool result during this turn, intersected with
    the IDs the answer actually mentions. The model cannot cite a table it never
    looked at, because it is not the one writing this list -- and it cannot pad
    the list with everything retrieved, because an ID it never mentioned is not
    something the answer rests on.
    """
    if not answer:
        return []

    lowered = answer.lower()
    mentioned: list[tuple[int, int, str]] = []

    for order, candidate in enumerate(_candidates(tool_results)):
        position = _first_mention(candidate, lowered)
        if position is not None:
            # Discovery order is the tie-break, so two tables sharing a bare
            # name come back in a stable order rather than an arbitrary one.
            mentioned.append((position, order, candidate))

    # First mention in the answer, not the order tools returned them: the reader
    # scans the answer top to bottom and the chips should follow.
    mentioned.sort()
    return [candidate for _, _, candidate in mentioned][:MAX_CITATIONS]


def _candidates(tool_results: Sequence[Any]) -> list[str]:
    """Every table ID a tool returned this turn, in discovery order."""
    found: dict[str, str] = {}
    seen: set[int] = set()
    for result in tool_results:
        _walk(result, found, seen, depth=0)
    return list(found.values())


def _walk(node: Any, found: dict[str, str], seen: set[int], depth: int) -> None:
    """Collect IDs from a nested result.

    `seen` holds the identity of every container already visited, so a
    self-referential structure terminates rather than hanging the turn. A
    container legitimately reachable twice is skipped the second time, which
    costs nothing: it holds the same IDs either way.
    """
    if depth > _MAX_DEPTH:
        return

    if isinstance(node, dict):
        if id(node) in seen:
            return
        seen.add(id(node))
        for key, value in node.items():
            if key in _ID_KEYS and isinstance(value, str):
                _add(value, found)
            elif key in _ID_LIST_KEYS and isinstance(value, list):
                for element in value:
                    if isinstance(element, str):
                        _add(element, found)
            _walk(value, found, seen, depth + 1)
        return

    if isinstance(node, list | tuple):
        if id(node) in seen:
            return
        seen.add(id(node))
        for element in node:
            _walk(element, found, seen, depth + 1)


def _add(value: str, found: dict[str, str]) -> None:
    """Keep a value if it is a table ID, deduplicating case-insensitively."""
    candidate = value.strip()
    if _TABLE_ID.fullmatch(candidate):
        found.setdefault(candidate.lower(), candidate)


def _first_mention(candidate: str, lowered_answer: str) -> int | None:
    """Where the answer first refers to this table, or None if it never does.

    Either spelling counts, and the earlier one wins: a model writes
    "fact_orders" far more often than "ordering/fact_orders", but it does
    sometimes write both.
    """
    positions: list[int] = []

    full = lowered_answer.find(candidate.lower())
    if full != -1:
        positions.append(full)

    _, _, bare = candidate.partition("/")
    match = _bare_name_pattern(bare).search(lowered_answer)
    if match is not None:
        positions.append(match.start())

    return min(positions) if positions else None


def _bare_name_pattern(bare: str) -> re.Pattern[str]:
    """A whole-word match for a bare table name.

    The boundaries are spelled out rather than using `\\b` because a name may
    end in `.` or `-`, which `\\b` treats as the boundary itself and so would
    match inside a longer name. Underscore counts as part of a word here, which
    is the point: `dim_date` must not match inside `dim_dates`, and `date` must
    not match inside `dim_date`.
    """
    return re.compile(rf"(?<![0-9a-z_]){re.escape(bare.lower())}(?![0-9a-z_])")
