"""Turning a question into a conversation title.

Pure text, no HTTP. This lived in `chat_routes.py` first, which is the least
likely place anybody would look for rune-safe truncation rules: a file of route
handlers is where you go to find out what a URL does, not how a string is cut.

The model is deliberately not asked to write the title. That is a second
provider call, paid for on every new thread, for a string nobody reads closely,
and the question is already the most accurate summary of the question.
"""

from __future__ import annotations

# How long a derived title may be. Counted in runes: a Japanese question cut at
# 60 *bytes* lands mid-character and renders as mojibake in the one place the
# reader looks to tell two conversations apart.
MAX_TITLE_RUNES = 60

# How far back to look for a word boundary before giving up and cutting hard.
# Wide enough to save most English questions from ending mid-word, narrow enough
# that a title never loses a quarter of itself to the search.
TITLE_BOUNDARY_WINDOW = 15

# Quotes a pasted question tends to arrive wrapped in, in the scripts this
# service is asked about. Stripped from both ends so a title does not open with
# a mark that never closes.
_QUOTES = "\"'\u201c\u201d\u2018\u2019\u300c\u300d\u300e\u300f"


def title_from_question(question: str) -> str:
    """A conversation title derived from its first question.

    The model is deliberately not asked to write one. That is a second provider
    call, paid for on every new thread, for a string nobody reads closely -- and
    the question itself is already the most accurate summary of the question.

    Truncation is by rune throughout. `str` indexes codepoints in Python, so
    slicing and `rfind` here are both safe for text that is not ASCII; the byte
    length of the result is nobody's business but the database's.

    A word boundary is used only when one falls within the last few runes. A
    language that does not put spaces between words has no boundary to find, and
    hunting further back for one would throw away half a Japanese title to end
    it at the only space in the sentence.
    """
    # Collapsed first: a question pasted across three lines would otherwise
    # carry its newlines into a list row and break the layout.
    collapsed = " ".join(question.split()).strip(_QUOTES).strip()

    if len(collapsed) <= MAX_TITLE_RUNES:
        return collapsed

    head = collapsed[:MAX_TITLE_RUNES]
    boundary = head.rfind(" ")
    if boundary >= MAX_TITLE_RUNES - TITLE_BOUNDARY_WINDOW:
        head = head[:boundary]
    return head.rstrip() + "\u2026"
