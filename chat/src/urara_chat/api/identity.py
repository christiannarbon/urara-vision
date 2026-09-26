"""Who a chat request is for. nginx sets the header after checking the session."""

from __future__ import annotations

from collections.abc import Iterator
from contextlib import contextmanager
from contextvars import ContextVar

from fastapi import Request

USER_ID_HEADER = "X-User-Id"
ACTING_USER_HEADER = "X-Acting-User"

user_id_var: ContextVar[str | None] = ContextVar("user_id", default=None)


class NotSignedIn(Exception):  # noqa: N818
    """No user identity on a request that needs one."""


def current_user_id() -> str | None:
    return user_id_var.get()


async def require_user(request: Request) -> str:
    """Router dependency: the caller's user ID, remembered for backend calls."""
    user_id = request.headers.get(USER_ID_HEADER, "").strip()
    if not user_id:
        raise NotSignedIn
    user_id_var.set(user_id)
    return user_id


@contextmanager
def as_service() -> Iterator[None]:
    """Backend calls inside run as the service, not the current user."""
    token = user_id_var.set(None)
    try:
        yield
    finally:
        user_id_var.reset(token)
