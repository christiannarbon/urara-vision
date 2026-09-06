"""Request and response shapes for the service's own HTTP surface.

Requests arrive camelCase to match the Go backend and the frontend, so the
wire name is set explicitly per field rather than by a generator -- there are
few enough of them that being able to read the mapping is worth more than the
brevity.
"""

from __future__ import annotations

from typing import Any

from pydantic import BaseModel, ConfigDict, Field


class ToolInvokeRequest(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    snapshot_id: str = Field(
        alias="snapshotId",
        description="Snapshot to run the tool against. 'latest' is resolved as it is elsewhere.",
    )
    tool: str = Field(description="Name of the tool to invoke.")
    args: dict[str, Any] = Field(
        default_factory=dict, description="Arguments, validated against the tool's own schema."
    )


class ToolDescription(BaseModel):
    """One tool as /debug/tools reports it."""

    name: str
    description: str
    schema_: dict[str, Any] = Field(alias="schema")

    model_config = ConfigDict(populate_by_name=True)
