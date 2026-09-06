"""Settings parsing.

The cases here are the ones that would otherwise be found in production: a base
URL with a trailing slash produces a double slash and a 404 that reads like a
missing route, and a mistyped log level should not stop a service from starting.
"""

import json
import logging

import pytest
from pydantic import ValidationError

from urara_chat.config import Settings, configure_logging, get_settings


@pytest.fixture(autouse=True)
def _llm_credentials(monkeypatch: pytest.MonkeyPatch) -> None:
    """Settings now refuses to construct without the credential its provider
    needs. These tests are about the backend settings, so a dummy key keeps each
    one about the thing it actually asserts."""
    monkeypatch.setenv("GOOGLE_API_KEY", "test-key-not-real")


def test_defaults_when_the_environment_is_empty(monkeypatch: pytest.MonkeyPatch) -> None:
    for var in (
        "BACKEND_BASE_URL",
        "BACKEND_API_TOKEN",
        "BACKEND_TIMEOUT_SECONDS",
        "LOG_LEVEL",
        "APP_ADDR",
    ):
        monkeypatch.delenv(var, raising=False)

    s = Settings()

    assert s.backend_base_url == "http://backend:8080"
    assert s.backend_api_token == ""
    assert s.backend_timeout_seconds == 30.0
    assert s.log_level == "info"
    assert s.app_addr == ":8090"


def test_every_setting_reads_its_environment_variable(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("BACKEND_BASE_URL", "http://elsewhere:9000")
    monkeypatch.setenv("BACKEND_API_TOKEN", "a-token")
    monkeypatch.setenv("BACKEND_TIMEOUT_SECONDS", "5.5")
    monkeypatch.setenv("LOG_LEVEL", "debug")
    monkeypatch.setenv("APP_ADDR", "127.0.0.1:9999")

    s = Settings()

    assert s.backend_base_url == "http://elsewhere:9000"
    assert s.backend_api_token == "a-token"
    assert s.backend_timeout_seconds == 5.5
    assert s.log_level == "debug"
    assert s.app_addr == "127.0.0.1:9999"


class TestBaseURL:
    def test_trailing_slash_is_stripped(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("BACKEND_BASE_URL", "http://x:8080/")
        assert Settings().backend_base_url == "http://x:8080"

    def test_a_url_without_one_is_unchanged(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("BACKEND_BASE_URL", "http://x:8080")
        assert Settings().backend_base_url == "http://x:8080"


def test_an_empty_token_is_valid(monkeypatch: pytest.MonkeyPatch) -> None:
    """Empty means "send no Authorization header", which is the backend's own
    documented unauthenticated mode -- not a misconfiguration."""
    monkeypatch.setenv("BACKEND_API_TOKEN", "")
    assert Settings().backend_api_token == ""


class TestAppAddr:
    def test_go_style_address_listens_on_every_interface(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setenv("APP_ADDR", ":8090")
        s = Settings()
        assert s.host == "0.0.0.0"
        assert s.port == 8090

    def test_an_explicit_host_is_kept(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("APP_ADDR", "127.0.0.1:9000")
        s = Settings()
        assert s.host == "127.0.0.1"
        assert s.port == 9000


class TestLogLevel:
    def test_an_unknown_level_falls_back(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Matching the Go server's parseLevel: a typo should not stop a start."""
        monkeypatch.setenv("LOG_LEVEL", "shouting")
        assert Settings().log_level == "info"

    @pytest.mark.parametrize("level", ["debug", "info", "warn", "error"])
    def test_known_levels_survive(self, level: str, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("LOG_LEVEL", level)
        assert Settings().log_level == level


def test_get_settings_is_cached() -> None:
    get_settings.cache_clear()
    assert get_settings() is get_settings()


class TestAppAddrIsRefusedWhenMalformed:
    """There is no sensible fallback for a listen address, so unlike a log level
    a bad one stops the service at start rather than at first use."""

    @pytest.mark.parametrize("addr", ["", "not-an-address", "localhost:", "localhost:http"])
    def test_a_port_that_is_not_a_number_is_refused(
        self, addr: str, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setenv("APP_ADDR", addr)
        with pytest.raises(ValidationError):
            Settings()


def test_configure_logging_emits_json_at_the_configured_level(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    """The Go service logs JSON through slog; a second shape in the same cluster
    is a permanent tax on whoever is reading the logs."""
    monkeypatch.setenv("LOG_LEVEL", "warn")
    configure_logging(Settings())

    logging.getLogger("test").info("filtered out, below the level")
    logging.getLogger("test").warning("kept")

    lines = [ln for ln in capsys.readouterr().out.splitlines() if ln.strip()]
    assert len(lines) == 1, f"expected only the warning, got {lines}"

    record = json.loads(lines[0])
    assert record["level"] == "warning"
    assert record["msg"] == "kept"
    assert record["time"]
