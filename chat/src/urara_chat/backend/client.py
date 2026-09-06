"""The only part of the service that knows an HTTP call is involved.

Everything reaches Postgres and Neo4j through here, and here reaches them only
through the Go backend's API. That is the load-bearing constraint of the whole
design: no database credentials live in this process.

**A table ID is `domain/table` and contains a slash.** Every one travels as a
query parameter and is encoded by httpx. Interpolating one into a path produces
a request for a route that does not exist, and the 404 that comes back is
indistinguishable from a table that is genuinely absent.
"""

from __future__ import annotations

from collections.abc import Sequence
from typing import Any

import httpx

from urara_chat.backend.errors import BackendError, BackendNotFound
from urara_chat.backend.models import (
    Conversation,
    Diagnostic,
    Domain,
    Graph,
    JoinPath,
    LineageEntry,
    Message,
    SearchHit,
    Snapshot,
    SnapshotContext,
    SourceTable,
    TableDetail,
    TablesDetailResponse,
    TableSummary,
)
from urara_chat.config import Settings

_API = "/api/v1"


class BackendClient:
    """An async client for one backend, holding one connection pool."""

    def __init__(self, settings: Settings) -> None:
        headers: dict[str, str] = {"Accept": "application/json"}
        # An empty token is the backend's documented unauthenticated mode, and
        # sending "Bearer " with nothing after it would be refused rather than
        # treated as absent.
        if settings.backend_api_token:
            headers["Authorization"] = f"Bearer {settings.backend_api_token}"

        self._client = httpx.AsyncClient(
            base_url=settings.backend_base_url,
            timeout=settings.backend_timeout_seconds,
            headers=headers,
        )

    async def aclose(self) -> None:
        await self._client.aclose()

    # --- plumbing -----------------------------------------------------------

    async def _get(self, path: str, params: dict[str, Any] | None = None) -> Any:
        """Issue a GET and return decoded JSON, or raise.

        Parameters go through httpx rather than into the path, which is what
        keeps a table ID's slash from being read as a route separator.
        """
        response = await self._client.get(path, params=params)
        if response.status_code >= 400:
            raise self._error(response)
        return response.json()

    async def _post(self, path: str, body: dict[str, Any]) -> Any:
        """Issue a POST and return decoded JSON, or raise."""
        response = await self._client.post(path, json=body)
        if response.status_code >= 400:
            raise self._error(response)
        return response.json()

    async def _delete(self, path: str) -> None:
        """Issue a DELETE, raising on failure.

        Nothing is decoded: a successful delete answers 204 with no body, and
        asking for JSON that is not there would turn a success into an error.
        """
        response = await self._client.delete(path)
        if response.status_code >= 400:
            raise self._error(response)

    @staticmethod
    def _error(response: httpx.Response) -> BackendError:
        """Turn a failed response into the right exception.

        The backend's own `error` field is preferred over the status reason: it
        says which ID was wrong, where the reason phrase only says that one was.
        """
        message = response.reason_phrase or f"HTTP {response.status_code}"
        try:
            body = response.json()
        except ValueError:
            # A proxy or a panic can answer with HTML or nothing at all, and a
            # client that raises while building an error message hides the
            # failure it was reporting.
            pass
        else:
            if isinstance(body, dict) and isinstance(body.get("error"), str):
                message = body["error"]

        cls = BackendNotFound if response.status_code == 404 else BackendError
        return cls(response.status_code, message)

    # --- reads --------------------------------------------------------------

    async def health(self) -> bool:
        """Whether the backend is ready.

        A bool rather than an exception: /readyz answering 503 is information
        about a dependency, not an error in the caller. It sits outside
        /api/v1 because kubelet cannot carry a credential.
        """
        try:
            response = await self._client.get("/readyz")
        except httpx.HTTPError:
            return False
        return response.status_code == 200

    async def resolve_snapshot(self, sid: str) -> str:
        """Turn a snapshot reference into a concrete ID.

        The only place the alias "latest" is allowed to appear. Everything
        downstream is pinned to the ID this returns, so an ingest part-way
        through a conversation cannot change what is being talked about.
        """
        data = await self._get(f"{_API}/snapshots/{sid}")
        return Snapshot.model_validate(data).id

    async def get_context(self, sid: str) -> SnapshotContext:
        return SnapshotContext.model_validate(await self._get(f"{_API}/snapshots/{sid}/context"))

    async def list_domains(self, sid: str) -> list[Domain]:
        data = await self._get(f"{_API}/snapshots/{sid}/domains")
        return [Domain.model_validate(d) for d in data["domains"]]

    async def list_tables(self, sid: str, domain: str | None = None) -> list[TableSummary]:
        params = {"domain": domain} if domain else None
        data = await self._get(f"{_API}/snapshots/{sid}/tables", params)
        return [TableSummary.model_validate(t) for t in data["tables"]]

    async def get_table(self, sid: str, table_id: str) -> TableDetail:
        data = await self._get(f"{_API}/snapshots/{sid}/table", {"id": table_id})
        return TableDetail.model_validate(data)

    async def get_tables(self, sid: str, ids: Sequence[str]) -> TablesDetailResponse:
        """Several table documents in one call.

        The IDs are one comma-separated `ids` parameter, which is the shape the
        batch endpoint takes; an ID it cannot find comes back in `missing`
        rather than failing the call.
        """
        data = await self._get(f"{_API}/snapshots/{sid}/tables/detail", {"ids": ",".join(ids)})
        return TablesDetailResponse.model_validate(data)

    async def search(self, sid: str, query: str, limit: int = 20) -> list[SearchHit]:
        data = await self._get(f"{_API}/snapshots/{sid}/search", {"q": query, "limit": limit})
        return [SearchHit.model_validate(h) for h in data["hits"]]

    async def neighbourhood(
        self, sid: str, table_id: str, depth: int = 1, sources: bool = False
    ) -> Graph:
        """The subgraph within `depth` hops of a table.

        The route is spelled the American way because the Neo4j store's method
        is; the Python name follows the house's British prose.
        """
        data = await self._get(
            f"{_API}/snapshots/{sid}/neighborhood",
            {"table": table_id, "depth": depth, "sources": str(sources).lower()},
        )
        return Graph.model_validate(data)

    async def join_paths(
        self, sid: str, frm: str, to: str, max_depth: int = 4, limit: int = 10
    ) -> list[JoinPath]:
        data = await self._get(
            f"{_API}/snapshots/{sid}/paths",
            {"from": frm, "to": to, "maxDepth": max_depth, "limit": limit},
        )
        return [JoinPath.model_validate(p) for p in data["paths"]]

    async def lineage(
        self, sid: str, table_id: str, direction: str = "upstream"
    ) -> list[LineageEntry]:
        """Upstream sources feeding a table, or downstream tables fed by a source.

        The response names the direction it answered as well as the entries;
        only the entries are of interest here, since the caller chose it.
        """
        data = await self._get(
            f"{_API}/snapshots/{sid}/lineage", {"id": table_id, "direction": direction}
        )
        return [LineageEntry.model_validate(e) for e in data["entries"]]

    async def diagnostics(self, sid: str, severity: str | None = None) -> list[Diagnostic]:
        params = {"severity": severity} if severity else None
        data = await self._get(f"{_API}/snapshots/{sid}/diagnostics", params)
        return [Diagnostic.model_validate(d) for d in data["diagnostics"]]

    async def list_sources(self, sid: str) -> list[SourceTable]:
        data = await self._get(f"{_API}/snapshots/{sid}/sources")
        return [SourceTable.model_validate(s) for s in data["sources"]]

    # --- conversations ------------------------------------------------------
    #
    # The only methods in this service that cause a write, and every one goes
    # through the Go API. A conversation ID is a UUID and carries no slash, so
    # unlike a table ID it is safe as a path segment.

    async def create_conversation(self, snapshot_id: str, title: str = "") -> Conversation:
        """Start a thread about one snapshot.

        `snapshot_id` may be "latest". It is passed through untouched: the
        backend resolves the alias and stores the concrete ID it resolved to, so
        resolving here as well would put a second opinion in the system about
        which snapshot a thread is pinned to.
        """
        data = await self._post(
            f"{_API}/conversations", {"snapshotId": snapshot_id, "title": title}
        )
        return Conversation.model_validate(data)

    async def list_conversations(self, snapshot_id: str) -> list[Conversation]:
        data = await self._get(f"{_API}/conversations", {"snapshot": snapshot_id})
        return [Conversation.model_validate(c) for c in data["conversations"]]

    async def get_conversation(self, cid: str) -> Conversation:
        """One thread with its full transcript."""
        return Conversation.model_validate(await self._get(f"{_API}/conversations/{cid}"))

    async def delete_conversation(self, cid: str) -> None:
        await self._delete(f"{_API}/conversations/{cid}")

    async def append_message(
        self,
        cid: str,
        role: str,
        content: str,
        citations: list[str] | None = None,
        meta: dict[str, Any] | None = None,
    ) -> Message:
        """Add one turn, returning it as stored.

        The ordinal is the database's to assign, so what comes back is the
        stored row rather than what was sent. `citations` is always a list:
        "drew on no tables" is a real answer, and sending null or omitting the
        key would make it indistinguishable from not having been asked.
        """
        body: dict[str, Any] = {
            "role": role,
            "content": content,
            "citations": citations if citations is not None else [],
        }
        if meta is not None:
            body["meta"] = meta

        data = await self._post(f"{_API}/conversations/{cid}/messages", body)
        return Message.model_validate(data)
