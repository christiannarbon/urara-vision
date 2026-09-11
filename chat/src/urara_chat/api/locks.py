"""Admission control for turns: one lock per conversation, one cap overall.

Two problems that look alike and are not. A conversation's turns must not
interleave, which is about *ordering within one thread*. The number of turns in
flight must have a ceiling, which is about *cost across all of them*. Solving
one does nothing for the other, so there are two objects here.

Both live for the life of the process and hold no state worth persisting: a
restart loses nothing but the right to be serialising, which is the correct
thing to lose when the process holding the lock has gone.
"""

from __future__ import annotations

import asyncio
import logging
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import HTTPException

log = logging.getLogger(__name__)

# What a refused caller is told to wait. Roughly one turn: by then a slot has
# usually freed, and a number the frontend can honour beats a bare refusal.
RETRY_AFTER_SECONDS = 5


class ConversationLocks:
    """Serialises turns within one conversation.

    An in-process asyncio.Lock is correct for a single replica, which is what
    the dev overlay runs. It is NOT correct across replicas: two pods each
    hold their own lock and interleave freely. If this deployment is ever
    scaled past one replica, the honest fix is a Postgres advisory lock taken
    through the backend -- not a bigger dictionary here.

    Phase 07 sets two replicas in the base manifest. Read that sentence again
    before relying on this for anything but the dev overlay.

    What it prevents: two turns on one thread each read the history, then each
    append, and the transcript comes out question, question, answer, answer.
    The Phase 01 store guarantees ordinals are unique, not that they are
    sensible.
    """

    def __init__(self) -> None:
        self._locks: dict[str, asyncio.Lock] = {}
        # How many turns are holding or waiting for each lock. Without it a
        # long-running pod accumulates one lock per conversation it has ever
        # seen, which is a leak measured in months rather than minutes -- slow
        # enough to reach production and never be noticed in a test.
        self._holders: dict[str, int] = {}

    @property
    def tracked(self) -> set[str]:
        """The conversations currently holding a lock. For tests and debugging."""
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
        """Take a reference to a conversation's lock, creating it if needed.

        No await anywhere in here, which is what makes it safe: on one event
        loop a function that never suspends cannot be interleaved with another
        copy of itself, so the count and the dictionary cannot disagree.
        """
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
    """Caps how many turns run at once, across every conversation.

    Every turn in flight is a provider call being paid for, so this is a bill
    ceiling as much as a resource one: without it, a page stuck in a refresh
    loop is an unbounded invoice.

    **Past the cap a turn is refused, not queued.** A queued request holds a
    connection while the reader watches nothing happen, decides it has hung and
    reloads -- which adds another one. A 429 with Retry-After is something a
    frontend can act on and a reader can be told about.
    """

    def __init__(self, limit: int, wait_seconds: float) -> None:
        self._semaphore = asyncio.Semaphore(limit)
        self._limit = limit
        self._wait_seconds = wait_seconds

    @asynccontextmanager
    async def hold(self) -> AsyncIterator[None]:
        """Occupy a slot for the duration of the block, or refuse with a 429.

        The wait must be positive; `Settings` refuses zero. `asyncio.wait_for`
        cancels a non-positive timeout before the loop runs the acquisition, so
        a zero wait would refuse every turn rather than only the ones that found
        the cap full.
        """
        try:
            await asyncio.wait_for(self._semaphore.acquire(), timeout=self._wait_seconds)
        except TimeoutError:
            log.warning("turn refused: all slots busy", extra={"limit": self._limit})
            # `from None`: the caller is being told the service is busy, and a
            # TimeoutError in the chain reads as the provider having timed out,
            # which is a different problem with a different fix.
            raise HTTPException(
                status_code=429,
                detail=(
                    f"too many turns in flight; the limit is {self._limit}. "
                    f"Retry in {RETRY_AFTER_SECONDS} seconds."
                ),
                headers={"Retry-After": str(RETRY_AFTER_SECONDS)},
            ) from None
        try:
            yield
        finally:
            self._semaphore.release()
