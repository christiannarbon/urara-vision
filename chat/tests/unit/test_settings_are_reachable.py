"""Every setting must be reachable from the compose file."""

import re
from pathlib import Path
from typing import Any

import pytest
from pydantic import SecretStr

from urara_chat.config import Settings

# Mounted by CHAT_RUN in the Makefile; the relative path is for a bare `uv run pytest` from the
# chat directory.
_CANDIDATES = (
    Path("/compose/docker-compose.yml"),
    Path(__file__).resolve().parents[3] / "docker-compose.yml",
)

# APP_ADDR is deliberately a committed literal: the published port is mapped from :8090, so moving
# the address alone would leave the service listening where nothing is published.
FIXED_ON_PURPOSE = frozenset({"APP_ADDR"})

# Compose defaults that deliberately differ from the field's own, and why.
DIFFERENT_ON_PURPOSE = {"BACKEND_API_TOKEN": "the compose backend expects the committed dev token"}


def compose_file() -> Path:
    for candidate in _CANDIDATES:
        if candidate.is_file():
            return candidate
    # Not a skip: this is the only thing standing between a new setting and a
    # silent default.
    pytest.fail(f"docker-compose.yml not found at any of {[str(c) for c in _CANDIDATES]}")


def chat_environment() -> dict[str, str]:
    """The chat service's environment block, as written."""
    text = compose_file().read_text()
    chat = text.split("\n  chat:\n", 1)[1]
    block = chat.split("environment:\n", 1)[1].split("\n    ports:", 1)[0]
    return dict(re.findall(r'^\s{6}([A-Z0-9_]+):\s*"?(.*?)"?\s*$', block, re.M))


def setting_names() -> set[str]:
    return {name.upper() for name in Settings.model_fields}


def test_every_setting_is_declared() -> None:
    """A setting compose does not name cannot be set from the host at all."""
    missing = sorted(setting_names() - set(chat_environment()))
    assert not missing, (
        f"settings absent from the chat service's environment block: {missing}. "
        'Add each as VAR: "${VAR:-default}", or it can only be changed by '
        "editing docker-compose.yml."
    )


def test_every_declared_setting_is_overridable() -> None:
    """Declaring one as a literal is only half the job: it still cannot be"""
    env = chat_environment()
    literals = sorted(
        name
        for name, value in env.items()
        if name in setting_names() and name not in FIXED_ON_PURPOSE and not value.startswith("${")
    )
    assert not literals, (
        f"declared but not overridable from the host: {literals}. "
        'Use VAR: "${VAR:-default}", or add it to FIXED_ON_PURPOSE with a reason.'
    )


def _agrees(declared: str, default: Any) -> bool:
    """Whether a compose default means the same as the field's own."""
    if isinstance(default, SecretStr):
        default = default.get_secret_value()
    if isinstance(default, bool):
        return declared.lower() == str(default).lower()
    if isinstance(default, int | float):
        try:
            return float(declared) == float(default)
        except ValueError:
            return False
    return declared == str(default)


def test_the_declared_defaults_match_the_code() -> None:
    """A compose default that has drifted from the field's own is worse than no"""
    env = chat_environment()
    drifted: list[str] = []
    for name, field in Settings.model_fields.items():
        if name.upper() in DIFFERENT_ON_PURPOSE:
            continue
        match = re.fullmatch(r"\$\{[A-Z0-9_]+:-(.*)\}", env.get(name.upper(), ""))
        if match and not _agrees(match.group(1), field.default):
            drifted.append(
                f"{name.upper()}: compose says {match.group(1)!r}, code says {field.default!r}"
            )
    assert not drifted, "compose defaults have drifted from config.py: " + "; ".join(drifted)
