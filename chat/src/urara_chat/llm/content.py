"""Flattening a provider reply to text."""

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
