"""Admission control for turns: one lock per conversation, one cap overall."""

from __future__ import annotations

import asyncio
import logging
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

log = logging.getLogger(__name__)

# What a refused caller is told to wait. Roughly one turn: by then a slot has usually freed, and a
# number the frontend can honour beats a bare refusal.
RETRY_AFTER_SECONDS = 5


class TurnsBusy(Exception):  # noqa: N818
    """Every slot is taken, and the caller should come back later."""

    def __init__(self, limit: int, retry_after: int = RETRY_AFTER_SECONDS) -> None:
        self.limit = limit
        self.retry_after = retry_after
        super().__init__(f"all {limit} turn slots are busy; retry in {retry_after}s")


class ConversationLocks:
    """Serialises turns within one conversation."""

    def __init__(self) -> None:
        self._locks: dict[str, asyncio.Lock] = {}
        # How many turns hold or wait for each lock. Without it a long-running pod keeps
        # one lock per conversation it has ever seen, which is a leak measured in months.
        self._holders: dict[str, int] = {}

    @property
    def tracked(self) -> set[str]:
        return set(self._locks)

    @asynccontextmanager
    async def hold(self, conversation_id: str) -> AsyncIterator[None]:
        """Hold one conversation's lock for the duration of the block."""
        lock = self._checkout(conversation_id)
        try:
            async with lock:
                yield
        finally:
            self._release(conversation_id)

    def _checkout(self, conversation_id: str) -> asyncio.Lock:
        """Take a reference to a conversation's lock, creating it if needed."""
        lock = self._locks.get(conversation_id)
        if lock is None:
            lock = self._locks[conversation_id] = asyncio.Lock()
        self._holders[conversation_id] = self._holders.get(conversation_id, 0) + 1
        return lock

    def _release(self, conversation_id: str) -> None:
        """Drop a reference, and evict the lock when the last one goes."""
        remaining = self._holders.get(conversation_id, 1) - 1
        if remaining > 0:
            self._holders[conversation_id] = remaining
            return
        self._holders.pop(conversation_id, None)
        self._locks.pop(conversation_id, None)


class TurnLimiter:
    """Caps how many turns run at once, across every conversation."""

    def __init__(self, limit: int, wait_seconds: float) -> None:
        self._semaphore = asyncio.Semaphore(limit)
        self._limit = limit
        self._wait_seconds = wait_seconds

    @asynccontextmanager
    async def hold(self) -> AsyncIterator[None]:
        """Occupy a slot for the duration of the block, or refuse with a 429."""
        try:
            await asyncio.wait_for(self._semaphore.acquire(), timeout=self._wait_seconds)
        except TimeoutError:
            log.warning("turn refused: all slots busy", extra={"limit": self._limit})
            # `from None`: the caller is being told the service is busy, and a TimeoutError in the
            # chain reads as the provider having timed out, which is a different problem with a
            # different fix.
            raise TurnsBusy(self._limit) from None
        try:
            yield
        finally:
            self._semaphore.release()
