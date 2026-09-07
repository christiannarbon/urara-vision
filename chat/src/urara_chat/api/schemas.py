"""Request and response shapes for the service's own HTTP surface.

Requests arrive camelCase to match the Go backend and the frontend, so the
wire name is set explicitly per field rather than by a generator -- there are
few enough of them that being able to read the mapping is worth more than the
brevity.
"""

from __future__ import annotations

from typing import Any

from pydantic import BaseModel, ConfigDict, Field, field_validator


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
