"""What a failed call to the Go API raises."""

from __future__ import annotations


class BackendError(Exception):
    """The Go API answered with something other than success."""

    def __init__(self, status: int, message: str) -> None:
        self.status = status
        self.message = message
        super().__init__(f"backend returned {status}: {message}")


# N818 wants an "Error" suffix. The name is what the tools and handlers are
# written against, and it subclasses BackendError, so `except BackendError`
# still catches it.
class BackendNotFound(BackendError):  # noqa: N818
    """404 -- the snapshot, table or conversation does not exist."""


class BackendRejected(BackendError):  # noqa: N818
    """4xx other than 404 -- the backend refused the request this service built."""


class BackendUnavailable(BackendError):  # noqa: N818
    """The backend could not be reached at all: refused, unresolved or timed out."""
