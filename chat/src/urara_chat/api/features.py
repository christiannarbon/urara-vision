"""Refuses chat routes while chat is switched off in the backend."""

from __future__ import annotations

import logging
import time
from collections.abc import Callable
from typing import TYPE_CHECKING

from fastapi import Request

from urara_chat.backend.errors import BackendError

if TYPE_CHECKING:
    from urara_chat.backend.client import BackendClient

log = logging.getLogger(__name__)


class ChatDisabled(Exception):  # noqa: N818
    """Chat is switched off."""


class FeatureGate:
    def __init__(
        self,
        client: BackendClient,
        ttl_seconds: float,
        clock: Callable[[], float] = time.monotonic,
    ) -> None:
        self._client = client
        self._ttl = ttl_seconds
        self._clock = clock
        self._enabled: bool | None = None
        self._expires = 0.0

    async def require_chat(self) -> None:
        if self._enabled is None or self._clock() >= self._expires:
            try:
                features = await self._client.features()
            except BackendError as exc:
                # A feature switch, not a security control: fail open, uncached.
                log.warning("could not read backend features, allowing chat: %s", exc.message)
                return
            self._enabled = features.chat.enabled
            self._expires = self._clock() + self._ttl
        if not self._enabled:
            raise ChatDisabled


async def require_chat(request: Request) -> None:
    """Router dependency for every /api/chat route."""
    # Only the lifespan builds a gate; apps assembled in unit tests run ungated.
    gate: FeatureGate | None = getattr(request.app.state, "feature_gate", None)
    if gate is not None:
        await gate.require_chat()
