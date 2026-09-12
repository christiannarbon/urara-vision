"""The only part of the service that knows an HTTP call is involved."""

from __future__ import annotations

from collections.abc import Sequence
from typing import Any

import httpx

from urara_chat.api.middleware import REQUEST_ID_HEADER, current_request_id
from urara_chat.backend.errors import (
    BackendError,
    BackendNotFound,
    BackendRejected,
    BackendUnavailable,
)
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


def _classify(status: int) -> type[BackendError]:
    """Which exception a failed status deserves."""
    if status == 404:
        return BackendNotFound
    if 400 <= status < 500:
        return BackendRejected
    return BackendError


class BackendClient:
    """An async client for one backend, holding one connection pool."""

    def __init__(self, settings: Settings) -> None:
        headers: dict[str, str] = {"Accept": "application/json"}
        # An empty token is the backend's documented unauthenticated mode, and sending "Bearer "
        # with nothing after it would be refused rather than treated as absent.
        if settings.backend_api_token:
            headers["Authorization"] = f"Bearer {settings.backend_api_token}"

        self._client = httpx.AsyncClient(
            base_url=settings.backend_base_url,
            timeout=settings.backend_timeout_seconds,
            headers=headers,
        )

    async def aclose(self) -> None:
        await self._client.aclose()

    async def _get(self, path: str, params: dict[str, Any] | None = None) -> Any:
        """Issue a GET and return decoded JSON, or raise."""
        response = await self._send("GET", path, params=params)
        if response.status_code >= 400:
            raise self._error(response)
        return response.json()

    async def _post(self, path: str, body: dict[str, Any]) -> Any:
        """Issue a POST and return decoded JSON, or raise."""
        response = await self._send("POST", path, json=body)
        if response.status_code >= 400:
            raise self._error(response)
        return response.json()

    async def _patch(self, path: str, body: dict[str, Any]) -> Any:
        """Issue a PATCH and return decoded JSON, or raise."""
        response = await self._send("PATCH", path, json=body)
        if response.status_code >= 400:
            raise self._error(response)
        return response.json()

    async def _delete(self, path: str) -> None:
        """Issue a DELETE, raising on failure."""
        response = await self._send("DELETE", path)
        if response.status_code >= 400:
            raise self._error(response)

    async def _send(self, method: str, path: str, **kwargs: Any) -> httpx.Response:
        """Issue a request, turning a transport failure into a BackendError."""
        # The request ID is forwarded on every call, which is what makes one ID span both
        # services: the Go side reads this header rather than minting its own, so a turn's log
        # lines here and there carry the same value.
        request_id = current_request_id()
        headers = {REQUEST_ID_HEADER: request_id} if request_id else None
        try:
            return await self._client.request(method, path, headers=headers, **kwargs)
        except httpx.HTTPError as exc:
            raise BackendUnavailable(502, f"backend unreachable: {exc}") from exc

    @staticmethod
    def _error(response: httpx.Response) -> BackendError:
        """Turn a failed response into the right exception."""
        message = response.reason_phrase or f"HTTP {response.status_code}"
        try:
            body = response.json()
        except ValueError:
            # A proxy or a panic can answer with HTML or nothing at all, and a client that raises
            # while building an error message hides the failure it was reporting.
            pass
        else:
            if isinstance(body, dict) and isinstance(body.get("error"), str):
                message = body["error"]

        return _classify(response.status_code)(response.status_code, message)

    async def health(self) -> bool:
        """Whether the backend is ready."""
        try:
            response = await self._send("GET", "/readyz")
        except BackendUnavailable:
            return False
        return response.status_code == 200

    async def resolve_snapshot(self, sid: str) -> str:
        """Turn a snapshot reference into a concrete ID."""
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
        """Several table documents in one call."""
        data = await self._get(f"{_API}/snapshots/{sid}/tables/detail", {"ids": ",".join(ids)})
        return TablesDetailResponse.model_validate(data)

    async def search(self, sid: str, query: str, limit: int = 20) -> list[SearchHit]:
        data = await self._get(f"{_API}/snapshots/{sid}/search", {"q": query, "limit": limit})
        return [SearchHit.model_validate(h) for h in data["hits"]]

    async def neighbourhood(
        self, sid: str, table_id: str, depth: int = 1, sources: bool = False
    ) -> Graph:
        """The subgraph within `depth` hops of a table."""
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
        """Upstream sources feeding a table, or downstream tables fed by a source."""
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

    # The only methods in this service that cause a write, and every one goes through the Go API.

    async def create_conversation(self, snapshot_id: str, title: str = "") -> Conversation:
        """Start a thread about one snapshot."""
        data = await self._post(
            f"{_API}/conversations", {"snapshotId": snapshot_id, "title": title}
        )
        return Conversation.model_validate(data)

    async def list_conversations(
        self, snapshot_id: str, limit: int | None = None
    ) -> list[Conversation]:
        """The most recent threads about one snapshot."""
        params: dict[str, Any] = {"snapshot": snapshot_id}
        if limit is not None:
            params["limit"] = limit
        data = await self._get(f"{_API}/conversations", params)
        return [Conversation.model_validate(c) for c in data["conversations"]]

    async def get_conversation(self, cid: str) -> Conversation:
        return Conversation.model_validate(await self._get(f"{_API}/conversations/{cid}"))

    async def set_conversation_title(self, cid: str, title: str) -> Conversation:
        """Retitle a thread."""
        data = await self._patch(f"{_API}/conversations/{cid}", {"title": title})
        return Conversation.model_validate(data)

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
        """Add one turn, returning it as stored."""
        body: dict[str, Any] = {
            "role": role,
            "content": content,
            "citations": citations if citations is not None else [],
        }
        if meta is not None:
            body["meta"] = meta

        data = await self._post(f"{_API}/conversations/{cid}/messages", body)
        return Message.model_validate(data)
