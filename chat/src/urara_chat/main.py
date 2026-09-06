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
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI

from urara_chat.api.routes import router
from urara_chat.backend.client import BackendClient
from urara_chat.config import configure_logging, get_settings


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    settings = get_settings()
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
