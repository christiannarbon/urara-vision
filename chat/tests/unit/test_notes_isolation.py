"""User-written notes in the prompt would be an injection path, so the agent never sees them."""

from pathlib import Path

import urara_chat
from urara_chat.backend.client import BackendClient
from urara_chat.tools.registry import build_tools

SRC = Path(urara_chat.__file__).parent


class _Client:
    """build_tools only closes over the client; nothing is called."""


def test_backend_client_has_no_notes_method() -> None:
    assert [a for a in dir(BackendClient) if "note" in a.lower()] == []


def test_no_tool_mentions_notes() -> None:
    for spec in build_tools(_Client(), "s1"):  # type: ignore[arg-type]
        assert "note" not in spec.name.lower(), spec.name
        assert "note" not in spec.description.lower(), spec.name


def test_no_source_file_calls_the_notes_routes() -> None:
    files = [p for p in SRC.rglob("*") if p.is_file() and "__pycache__" not in p.parts]
    assert files, "no source files found"
    offenders = [
        str(p.relative_to(SRC))
        for p in files
        if "/notes" in p.read_text(encoding="utf-8", errors="ignore")
    ]
    assert offenders == []
