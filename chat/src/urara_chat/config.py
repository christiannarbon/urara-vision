"""Runtime settings, read from the environment."""

from __future__ import annotations

import json
import logging
import sys
from functools import lru_cache
from typing import Any, Literal

from pydantic import SecretStr, field_validator, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict

# The levels the Go server accepts, mapped to Python's.
_LOG_LEVELS: dict[str, int] = {
    "debug": logging.DEBUG,
    "info": logging.INFO,
    "warn": logging.WARNING,
    "error": logging.ERROR,
}

_DEFAULT_LOG_LEVEL = "info"

# The longest a turn may hold a connection.
MAX_ANSWER_TIMEOUT_SECONDS = 300.0

# The longest a turn may wait for a free slot.
MAX_ADMISSION_WAIT_SECONDS = 5.0
# A container listens on every interface; the pod's NetworkPolicy is what narrows who may reach
# it.
_DEFAULT_HOST = "0.0.0.0"


class ConfigurationError(RuntimeError):
    """Settings are invalid, carrying only the reasons."""


class Settings(BaseSettings):
    """Everything the service needs, and nothing it does not."""

    model_config = SettingsConfigDict(
        # Field names are the environment variables, upper-cased: backend_base_url reads
        # BACKEND_BASE_URL, so no field needs an alias of its own.
        case_sensitive=False,
        extra="ignore",
    )

    backend_base_url: str = "http://backend:8080"
    backend_api_token: str = ""
    backend_timeout_seconds: float = 30.0
    log_level: str = _DEFAULT_LOG_LEVEL
    app_addr: str = ":8090"

    # A Literal rather than a string plus a check: an unknown provider is then refused by the
    # type, in one place, with a message pydantic writes.
    llm_provider: Literal["gemini-studio", "vertex"] = "gemini-studio"
    llm_model: str = "gemini-2.5-flash"
    llm_temperature: float = 0.2
    llm_max_output_tokens: int = 2048
    llm_timeout_seconds: float = 60.0

    # A snapshot is immutable once ingested, so a rendered context card stays true for as long as
    # the pod cares to keep it.
    context_cache_ttl_seconds: float = 300.0

    # An unbounded transcript is an unbounded bill: the whole history is resent on every turn.
    max_history_messages: int = 20
    # How many times the model may ask for tools before it must answer with what it has. Six is
    # generous for the nine tools available.
    max_tool_iterations: int = 6

    # How many turns may be in flight at once, across every conversation.
    max_concurrent_turns: int = 4

    # How long a turn waits for a free slot before it is refused.
    turn_admission_wait_seconds: float = 0.5

    # A question long enough to be a pasted document is not a question, and the prompt it would
    # build is paid for in full before the model reads a word of it.
    max_question_chars: int = 4000

    # A ceiling on the whole turn, not on one call.
    answer_timeout_seconds: float = 120.0

    # Refused on Content-Length, before the body is read.
    max_request_bytes: int = 1_048_576

    # SecretStr so redaction is the default rather than something to remember at every point the
    # settings are printed. No credential has a default value.
    google_api_key: SecretStr = SecretStr("")
    vertex_project: str = ""
    vertex_location: str = "us-central1"

    @field_validator("google_api_key")
    @classmethod
    def _strip_key(cls, v: SecretStr) -> SecretStr:
        return SecretStr(v.get_secret_value().strip())

    @field_validator("vertex_project", "vertex_location")
    @classmethod
    def _strip_identifier(cls, v: str) -> str:
        return v.strip()

    @model_validator(mode="after")
    def _credentials_match_the_provider(self) -> Settings:
        """Refuse to start without the credential the chosen provider needs."""
        # `not self.google_api_key` rather than unwrapping it: an empty SecretStr is already
        # falsy, and get_secret_value() belongs only in the factory that hands the key to the SDK.
        if self.llm_provider == "gemini-studio" and not self.google_api_key:
            raise ValueError("GOOGLE_API_KEY must be set when LLM_PROVIDER is 'gemini-studio'")
        if self.llm_provider == "vertex" and not self.vertex_project:
            # Vertex authenticates with Application Default Credentials, so it needs no key --
            # only somewhere to send the request.
            raise ValueError("VERTEX_PROJECT must be set when LLM_PROVIDER is 'vertex'")
        return self

    @field_validator("max_concurrent_turns")
    @classmethod
    def _at_least_one_turn(cls, v: int) -> int:
        """Refuse a cap that lets nothing through."""
        if v < 1:
            raise ValueError(f"MAX_CONCURRENT_TURNS must be at least 1, got {v}")
        return v

    @field_validator("turn_admission_wait_seconds")
    @classmethod
    def _bounded_admission_wait(cls, v: float) -> float:
        """Keep the grace period from becoming a queue, and from becoming zero."""
        if v <= 0:
            raise ValueError(
                f"TURN_ADMISSION_WAIT_SECONDS must be greater than zero, got {v}; "
                "zero would refuse every turn, not only the ones over the cap"
            )
        return min(v, MAX_ADMISSION_WAIT_SECONDS)

    @field_validator("answer_timeout_seconds")
    @classmethod
    def _bounded_answer_timeout(cls, v: float) -> float:
        """Cap the turn deadline, and refuse one that cannot be met."""
        if v <= 0:
            raise ValueError(f"ANSWER_TIMEOUT_SECONDS must be greater than zero, got {v}")
        return min(v, MAX_ANSWER_TIMEOUT_SECONDS)

    @field_validator("backend_base_url")
    @classmethod
    def _strip_trailing_slash(cls, v: str) -> str:
        return v.rstrip("/")

    @field_validator("log_level")
    @classmethod
    def _known_log_level(cls, v: str) -> str:
        """Fall back rather than raise, as the Go server's parseLevel does."""
        level = v.strip().lower()
        return level if level in _LOG_LEVELS else _DEFAULT_LOG_LEVEL

    @field_validator("app_addr")
    @classmethod
    def _parsable_address(cls, v: str) -> str:
        """Refuse an address whose port is not a number."""
        _, _, port = v.rpartition(":")
        if not port.isdigit():
            raise ValueError(f"app_addr must end in a port, got {v!r}")
        return v

    @property
    def host(self) -> str:
        """The interface to listen on."""
        host, _, _ = self.app_addr.rpartition(":")
        return host or _DEFAULT_HOST

    @property
    def port(self) -> int:
        _, _, port = self.app_addr.rpartition(":")
        return int(port)

    @property
    def log_level_number(self) -> int:
        return _LOG_LEVELS[self.log_level]


# Everything logging puts on a record itself.
_RESERVED_RECORD_KEYS = frozenset(logging.LogRecord("", 0, "", 0, "", None, None).__dict__) | {
    "message",
    "asctime",
    "taskName",
}

# The four keys the shape is defined by. Held apart so a caller cannot displace one of them with
# an `extra` field of the same name.
_FIXED_KEYS = ("time", "level", "msg", "logger")


class JSONLogFormatter(logging.Formatter):
    """Renders a record as one JSON object per line."""

    def format(self, record: logging.LogRecord) -> str:
        payload: dict[str, Any] = {
            "time": self.formatTime(record, "%Y-%m-%dT%H:%M:%S%z"),
            "level": record.levelname.lower(),
            "msg": record.getMessage(),
            "logger": record.name,
        }
        # Merged after the fixed keys and with them removed, so `extra={"msg": ...}` adds a field
        # rather than rewriting the message.
        payload.update(
            {
                key: value
                for key, value in record.__dict__.items()
                if key not in _RESERVED_RECORD_KEYS and key not in _FIXED_KEYS
            }
        )
        if record.exc_info:
            payload["error"] = self.formatException(record.exc_info)
        # default=str rather than letting a value raise: a formatter that throws takes out the
        # line it was writing and tells nobody why, and a log call is the last place that should
        # be able to fail a request.
        return json.dumps(payload, default=str)


def configure_logging(settings: Settings) -> None:
    """Send structured logs to stdout at the configured level."""
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(JSONLogFormatter())

    root = logging.getLogger()
    root.handlers = [handler]
    root.setLevel(settings.log_level_number)


@lru_cache
def get_settings() -> Settings:
    return Settings()
