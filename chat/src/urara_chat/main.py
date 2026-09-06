"""The FastAPI application.

One backend client is built at startup and closed at shutdown. Per-request
clients would leak connections and throw away pooling, and the client holds the
only network identity this service has.

There is no authentication here, by design: the service is not exposed outside
the cluster, and Phase 07 handles reachability at the network layer rather than
with a second token to rotate.
"""

from __future__ import annotations

import logging
import sys
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI
from pydantic import ValidationError

from urara_chat.api.routes import router
from urara_chat.backend.client import BackendClient
from urara_chat.config import ConfigurationError, configure_logging, get_settings


def _reasons(exc: ValidationError) -> list[str]:
    """The messages from a validation error, without pydantic's framing.

    A settings error is almost always one missing environment variable, and the
    name of it is the whole content of the message worth printing.
    """
    return [str(err.get("msg", "")).removeprefix("Value error, ") for err in exc.errors()]


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    try:
        settings = get_settings()
    except ValidationError as exc:
        # Logged as one line, then re-raised as a ConfigurationError carrying
        # only the reasons. `from None` drops the pydantic error from the
        # traceback deliberately: it embeds the input it was given, and a long
        # API key leaves its tail in that text.
        reasons = "; ".join(_reasons(exc))
        logging.basicConfig(stream=sys.stdout, level=logging.ERROR, format="%(message)s")
        logging.getLogger("urara_chat").error(
            "configuration is invalid, refusing to start: %s", reasons
        )
        raise ConfigurationError(reasons) from None

    configure_logging(settings)

    log = logging.getLogger("urara_chat")
    log.info("starting, backend at %s", settings.backend_base_url)

    app.state.settings = settings
    app.state.client = BackendClient(settings)
    try:
        yield
    finally:
        await app.state.client.aclose()
        log.info("stopped")


app = FastAPI(
    title="urara-vision chat",
    description="Answers questions about a documented data model.",
    lifespan=lifespan,
)
app.include_router(router)
