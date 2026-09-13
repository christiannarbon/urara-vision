"""Working out what an answer actually rested on."""

from __future__ import annotations

import re
from collections.abc import Sequence
from typing import Any

# A table ID is `domain/table`: exactly one slash, each half starting with a letter, digit or
# underscore.
_TABLE_ID = re.compile(r"^[a-z0-9_][a-z0-9_.-]*/[a-z0-9_][a-z0-9_.-]*$", re.IGNORECASE)

# Keys whose string value is an identifier rather than prose. Not `description`
# or `grain`: a mention there is a coincidence, not a retrieval.
_ID_KEYS = frozenset({"id", "tableId", "from", "to", "source", "target"})

# Keys holding a list of bare IDs. `JoinPath.tables` is a list of strings rather than of objects,
# so its elements never pass under an `id` key.
_ID_LIST_KEYS = frozenset({"tables"})

# Tool results are a handful of levels deep at most. The cap is a guard against a malformed or
# adversarial result hanging a turn, not a real limit.
_MAX_DEPTH = 12

MAX_CITATIONS = 20


def extract_citations(tool_results: Sequence[Any], answer: str) -> list[str]:
    """Table IDs the answer drew on."""
    if not answer:
        return []

    lowered = answer.lower()
    candidates = _candidates(tool_results)
    # A full ID names one table, so its bare name inside that ID must not cite a namesake too.
    unpinned = lowered
    for candidate in candidates:
        unpinned = _full_id_pattern(candidate).sub(lambda m: " " * len(m.group()), unpinned)
    mentioned: list[tuple[int, int, str]] = []

    for order, candidate in enumerate(candidates):
        position = _first_mention(candidate, lowered, unpinned)
        if position is not None:
            # Discovery order is the tie-break, so two tables sharing a bare name come back in a
            # stable order rather than an arbitrary one.
            mentioned.append((position, order, candidate))

    # First mention in the answer, not the order tools returned them: the reader scans the answer
    # top to bottom and the chips should follow.
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
    """Collect IDs from a nested result."""
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


def _first_mention(candidate: str, lowered_answer: str, unpinned: str) -> int | None:
    """Where the answer first refers to this table, or None if it never does."""
    positions: list[int] = []

    full = _full_id_pattern(candidate).search(lowered_answer)
    if full is not None:
        positions.append(full.start())

    _, _, bare = candidate.partition("/")
    match = _bare_name_pattern(bare).search(unpinned)
    if match is not None:
        positions.append(match.start())

    return min(positions) if positions else None


def _full_id_pattern(candidate: str) -> re.Pattern[str]:
    return re.compile(rf"(?<![0-9a-z_]){re.escape(candidate.lower())}(?![0-9a-z_])")


def _bare_name_pattern(bare: str) -> re.Pattern[str]:
    name = re.escape(bare.lower())
    # A plain lowercase word such as `film` is also prose, so only a code span counts as naming it.
    if re.fullmatch(r"[a-z]+", bare):
        return re.compile(rf"`{name}`")
    return re.compile(rf"(?<![0-9a-z_]){name}(?![0-9a-z_])")
