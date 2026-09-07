"""Flattening a provider reply to text.

A model may answer with a string or with a list of parts, and which one arrives
depends on the provider, the model and whether the turn used tools. Anything
that reports `[{'type': 'text', ...}]` to a reader has failed at its job.

It lives beside the factory because the shape it copes with is the provider's,
not the graph's or the HTTP layer's: a provider that starts answering in a new
part shape is fixed here, once. Both the graph's finalise step and the /debug/llm
probe carried their own near-identical copy until 04.R, and the two had already
drifted apart in their signatures.
"""

from __future__ import annotations

from typing import Any


def flatten_content(content: Any) -> str:
    """A message's content as plain text."""
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        parts = [p if isinstance(p, str) else p.get("text", "") for p in content]
        return "".join(str(p) for p in parts)
    return str(content)
