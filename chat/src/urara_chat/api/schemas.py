"""Request and response shapes for the service's own HTTP surface.

Requests arrive camelCase to match the Go backend and the frontend, so the
wire name is set explicitly per field rather than by a generator -- there are
few enough of them that being able to read the mapping is worth more than the
brevity.
"""

from __future__ import annotations

from datetime import datetime
from typing import Any

from pydantic import BaseModel, ConfigDict, Field, field_validator
from pydantic.alias_generators import to_camel


class ToolInvokeRequest(BaseModel):
    # Unknown keys are refused rather than ignored, so a misspelled field is
    # reported instead of silently doing nothing.
    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    snapshot_id: str = Field(
        alias="snapshotId",
        description="Snapshot to run the tool against. 'latest' is resolved as it is elsewhere.",
    )
    tool: str = Field(description="Name of the tool to invoke.")
    args: dict[str, Any] = Field(
        default_factory=dict, description="Arguments, validated against the tool's own schema."
    )


# What the frontend can send. An unknown value falls back rather than failing:
# a question is worth answering in the wrong language, and is not worth a 400.
_LANGUAGES = frozenset({"EN", "JA"})
_DEFAULT_LANGUAGE = "EN"


class AnswerRequest(BaseModel):
    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    snapshot_id: str = Field(
        alias="snapshotId",
        description="Snapshot to answer about. 'latest' is resolved as it is elsewhere.",
    )
    question: str = Field(description="The reader's question, in any language.")
    language: str = Field(
        default=_DEFAULT_LANGUAGE, description="Language to answer in. 'EN' or 'JA'."
    )

    @field_validator("language")
    @classmethod
    def _known_language(cls, v: str) -> str:
        """Fall back rather than reject, as the log level in `config` does.

        An unknown language is a frontend that has moved on, not a bad question.
        Refusing the turn over it loses the answer as well as the language.
        """
        language = v.strip().upper()
        return language if language in _LANGUAGES else _DEFAULT_LANGUAGE


class AnswerResponse(BaseModel):
    """One turn's answer, and everything needed to explain it.

    Every diagnostic field is on the wire, not just the text. Without
    `toolCalls` and `iterations` a wrong answer is unexplainable, which is the
    whole reason the route it serves exists.
    """

    # `model` collides with pydantic's protected prefix, and the warning it
    # emits is noise: the field is the model's name and there is no better one.
    model_config = ConfigDict(populate_by_name=True, protected_namespaces=())

    text: str
    citations: list[str]
    tool_calls: list[dict[str, Any]] = Field(alias="toolCalls")
    iterations: int
    truncated: bool
    model: str
    latency_ms: int = Field(alias="latencyMs")
    usage: dict[str, int]


# --- conversations ----------------------------------------------------------
#
# Thin mirrors of what the Go backend stores. They are declared here rather than
# reused from `backend.models` so this service's own wire contract is visible in
# one file and in its OpenAPI schema -- but they are mirrors, not a second
# opinion: the backend owns the conversation schema, and nothing here validates
# a field it owns.


class CreateConversationRequest(BaseModel):
    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    snapshot_id: str = Field(
        alias="snapshotId",
        description=(
            "Snapshot the thread is about. 'latest' is passed through untouched: "
            "the backend resolves it and stores the concrete ID it resolved to."
        ),
    )
    title: str = Field(default="", description="Optional; 05.6 sets it from the first question.")


class MessageResponse(BaseModel):
    """One stored turn."""

    model_config = ConfigDict(populate_by_name=True, alias_generator=to_camel)

    ordinal: int
    role: str
    content: str
    # Always a list. "Drew on no tables" is a real answer, and it must not come
    # back as null and become indistinguishable from not having been asked.
    citations: list[str] = []
    meta: dict[str, Any] = {}
    created_at: datetime | None = None


class ConversationResponse(BaseModel):
    """One thread.

    `messages` is empty in a listing and populated when one conversation is
    fetched, which mirrors the backend exactly -- including the consequence that
    an empty thread and an unfetched one look the same.
    """

    model_config = ConfigDict(populate_by_name=True, alias_generator=to_camel)

    id: str
    snapshot_id: str = ""
    title: str = ""
    created_at: datetime | None = None
    updated_at: datetime | None = None
    messages: list[MessageResponse] = []


class ConversationListResponse(BaseModel):
    """Threads about one snapshot, newest first and without their transcripts."""

    conversations: list[ConversationResponse]
