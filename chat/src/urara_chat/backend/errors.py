"""What a failed call to the Go API raises.

The backend answers a failure with `{"error": "..."}` and a status, so the
message a caller sees is the backend's own words where it sent any -- a tool
reporting "not found" is far less useful than one reporting which ID missed.
"""

from __future__ import annotations


class BackendError(Exception):
    """The Go API answered with something other than success."""

    def __init__(self, status: int, message: str) -> None:
        self.status = status
        self.message = message
        super().__init__(f"backend returned {status}: {message}")


# N818 wants an "Error" suffix. The name is the one the tools and handlers are
# written against, and it already carries the meaning the rule is protecting --
# it subclasses BackendError, so an `except BackendError` still catches it.
class BackendNotFound(BackendError):  # noqa: N818
    """404 -- the snapshot, table or conversation does not exist."""


class BackendRejected(BackendError):  # noqa: N818
    """4xx other than 404 -- the backend refused the request this service built.

    Held apart from its parent because it is **not** the backend failing. The
    backend worked: it read a request, found it invalid, and said so. Reporting
    that as an upstream outage sends whoever is on call to the wrong service and
    leaves the actual bug -- a request this service assembled wrongly -- looking
    like someone else's problem.

    A caller sees a plain internal error, because that is what it is. The
    backend's own words go to the log with the request ID.
    """


class BackendUnavailable(BackendError):  # noqa: N818
    """The backend could not be reached at all: refused, unresolved or timed out.

    A subclass of BackendError so it maps to the same 502 an upstream failure
    already does. The distinction that matters to a caller is only ever "the
    fault is upstream", and the message carries the detail.
    """
