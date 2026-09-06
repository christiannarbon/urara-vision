"""Runtime settings, read from the environment.

The Go service's `internal/config` does the same job and sets the posture this
file follows: defaults that work against the bundled compose stack, and a
setting that is wrong is corrected or refused here rather than surfacing later
as a puzzling failure in a request.
"""

from __future__ import annotations

import json
import logging
import sys
from functools import lru_cache
from typing import Any, Literal

from pydantic import SecretStr, field_validator, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict

# The levels the Go server accepts, mapped to Python's. Anything else falls
# back to info, matching its parseLevel: a typo in a log level should not stop
# a service from starting.
_LOG_LEVELS: dict[str, int] = {
    "debug": logging.DEBUG,
    "info": logging.INFO,
    "warn": logging.WARNING,
    "error": logging.ERROR,
}

_DEFAULT_LOG_LEVEL = "info"
# A container listens on every interface; the pod's NetworkPolicy is what
# narrows who may reach it.
_DEFAULT_HOST = "0.0.0.0"


class ConfigurationError(RuntimeError):
    """Settings are invalid, carrying only the reasons.

    Raised instead of letting pydantic's own ValidationError escape. That error
    embeds the input it was given, and pydantic elides the *middle* of a long
    value rather than the ends -- so a long API key leaves its tail in the
    message, and re-raising puts that tail in the container log on every restart
    of a crash loop.
    """


class Settings(BaseSettings):
    """Everything the service needs, and nothing it does not.

    There is deliberately no database setting: the service reaches Postgres and
    Neo4j only through the Go backend's HTTP API, and holding no credentials is
    what keeps it unable to write anything the backend has not given it an
    endpoint for.
    """

    model_config = SettingsConfigDict(
        # Field names are the environment variables, upper-cased: backend_base_url
        # reads BACKEND_BASE_URL, so no field needs an alias of its own.
        case_sensitive=False,
        extra="ignore",
    )

    backend_base_url: str = "http://backend:8080"
    backend_api_token: str = ""
    backend_timeout_seconds: float = 30.0
    log_level: str = _DEFAULT_LOG_LEVEL
    app_addr: str = ":8090"

    # A Literal rather than a string plus a check: an unknown provider is then
    # refused by the type, in one place, with a message pydantic writes.
    llm_provider: Literal["gemini-studio", "vertex"] = "gemini-studio"
    llm_model: str = "gemini-2.5-flash"
    llm_temperature: float = 0.2
    llm_max_output_tokens: int = 2048
    llm_timeout_seconds: float = 60.0

    # A snapshot is immutable once ingested, so a rendered context card stays
    # true for as long as the pod cares to keep it. The TTL exists to bound
    # memory and to pick up a re-ingest under the same ID, not for correctness.
    context_cache_ttl_seconds: float = 300.0

    # SecretStr so redaction is the default rather than something to remember at
    # every point the settings are printed. No credential has a default value.
    google_api_key: SecretStr = SecretStr("")
    vertex_project: str = ""
    vertex_location: str = "us-central1"

    @field_validator("google_api_key")
    @classmethod
    def _strip_key(cls, v: SecretStr) -> SecretStr:
        """Trim the credential before anything judges whether it is present.

        A trailing newline from `export GOOGLE_API_KEY=$(cat key.txt)` is the
        common case, and whitespace is otherwise truthy: the check below would
        pass and the provider would answer with a puzzling 400.

        This unwraps the secret, which the factory is otherwise the only place
        to do. The value never leaves the validator -- it is stripped and
        re-wrapped -- and `str(v)` would return the mask and strip nothing.
        """
        return SecretStr(v.get_secret_value().strip())

    @field_validator("vertex_project", "vertex_location")
    @classmethod
    def _strip_identifier(cls, v: str) -> str:
        return v.strip()

    @model_validator(mode="after")
    def _credentials_match_the_provider(self) -> Settings:
        """Refuse to start without the credential the chosen provider needs.

        The same posture as the Go service's NEO4J_PASSWORD check: a process
        that starts and then fails every request is worse than one that does not
        start. The message names the environment variable because that string is
        what someone reads in `kubectl logs` at the moment they are least
        inclined to go digging.
        """
        # `not self.google_api_key` rather than unwrapping it: an empty
        # SecretStr is already falsy, and get_secret_value() belongs only in the
        # factory that hands the key to the SDK.
        if self.llm_provider == "gemini-studio" and not self.google_api_key:
            raise ValueError("GOOGLE_API_KEY must be set when LLM_PROVIDER is 'gemini-studio'")
        if self.llm_provider == "vertex" and not self.vertex_project:
            # Vertex authenticates with Application Default Credentials, so it
            # needs no key -- only somewhere to send the request.
            raise ValueError("VERTEX_PROJECT must be set when LLM_PROVIDER is 'vertex'")
        return self

    @field_validator("backend_base_url")
    @classmethod
    def _strip_trailing_slash(cls, v: str) -> str:
        """Drop a trailing slash so callers can build `f"{base}/api/v1/..."`.

        A base URL ending in a slash yields a double slash in every path, which
        the backend answers with a 404 that reads like a missing route rather
        than like a misconfigured setting.
        """
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
        """Refuse an address whose port is not a number.

        Unlike a log level there is no sensible fallback for a listen address,
        and the alternative is a ValueError raised from a property long after
        start-up, where it reads as a bug rather than as a bad setting.
        """
        _, _, port = v.rpartition(":")
        if not port.isdigit():
            raise ValueError(f"app_addr must end in a port, got {v!r}")
        return v

    @property
    def host(self) -> str:
        """The interface to listen on.

        `app_addr` is Go-style, so ":8090" is a valid address meaning every
        interface; that form is what the backend's own APP_ADDR default uses.
        """
        host, _, _ = self.app_addr.rpartition(":")
        return host or _DEFAULT_HOST

    @property
    def port(self) -> int:
        _, _, port = self.app_addr.rpartition(":")
        return int(port)

    @property
    def log_level_number(self) -> int:
        return _LOG_LEVELS[self.log_level]


class JSONLogFormatter(logging.Formatter):
    """Renders a record as one JSON object per line.

    The Go service logs JSON through slog, and two services in one cluster
    emitting different shapes is a small permanent tax on anyone reading the
    logs. The keys match slog's: time, level, msg.
    """

    def format(self, record: logging.LogRecord) -> str:
        payload: dict[str, Any] = {
            "time": self.formatTime(record, "%Y-%m-%dT%H:%M:%S%z"),
            "level": record.levelname.lower(),
            "msg": record.getMessage(),
            "logger": record.name,
        }
        if record.exc_info:
            payload["error"] = self.formatException(record.exc_info)
        return json.dumps(payload)


def configure_logging(settings: Settings) -> None:
    """Send structured logs to stdout at the configured level.

    Handlers are replaced rather than added to, so calling this twice -- which
    uvicorn's reloader does -- cannot double every line.
    """
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(JSONLogFormatter())

    root = logging.getLogger()
    root.handlers = [handler]
    root.setLevel(settings.log_level_number)


@lru_cache
def get_settings() -> Settings:
    """The process's settings, read once.

    Cached because the environment does not change under a running process, and
    because every call site would otherwise re-read and re-validate it.
    """
    return Settings()
