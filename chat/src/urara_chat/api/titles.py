"""Turning a question into a conversation title."""

from __future__ import annotations

# How long a derived title may be.
MAX_TITLE_RUNES = 60

# How far back to look for a word boundary before giving up and cutting hard.
TITLE_BOUNDARY_WINDOW = 15

# Quotes a pasted question tends to arrive wrapped in, in the scripts this service is asked about.
_QUOTES = "\"'\u201c\u201d\u2018\u2019\u300c\u300d\u300e\u300f"


def title_from_question(question: str) -> str:
    """A conversation title derived from its first question."""
    # Collapsed first: a question pasted across three lines would otherwise carry its newlines
    # into a list row and break the layout.
    collapsed = " ".join(question.split()).strip(_QUOTES).strip()

    if len(collapsed) <= MAX_TITLE_RUNES:
        return collapsed

    head = collapsed[:MAX_TITLE_RUNES]
    boundary = head.rfind(" ")
    if boundary >= MAX_TITLE_RUNES - TITLE_BOUNDARY_WINDOW:
        head = head[:boundary]
    return head.rstrip() + "\u2026"
