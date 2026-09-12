"""Settings parsing."""

import json
import logging

import pytest
from pydantic import ValidationError

from urara_chat.config import (
    MAX_ADMISSION_WAIT_SECONDS,
    MAX_ANSWER_TIMEOUT_SECONDS,
    Settings,
    configure_logging,
    get_settings,
)


@pytest.fixture(autouse=True)
def _llm_credentials(monkeypatch: pytest.MonkeyPatch) -> None:
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
    """Empty means "send no Authorization header", which is the backend's own"""
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
    """There is no sensible fallback for a listen address, so unlike a log level"""

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
    """The Go service logs JSON through slog; a second shape in the same cluster"""
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


class TestTheTurnDeadline:
    """A turn holds a connection for its whole duration, so the ceiling on that"""

    def test_a_generous_setting_is_clamped(self) -> None:
        assert (
            Settings(answer_timeout_seconds=3600).answer_timeout_seconds
            == MAX_ANSWER_TIMEOUT_SECONDS
        )

    def test_the_ceiling_is_five_minutes(self) -> None:
        assert MAX_ANSWER_TIMEOUT_SECONDS == 300.0

    def test_a_shorter_setting_is_left_alone(self) -> None:
        """Clamping is an upper bound, not a target: a deployment that wants to"""
        assert Settings(answer_timeout_seconds=30).answer_timeout_seconds == 30.0

    def test_the_default_is_already_under_the_ceiling(self) -> None:
        assert Settings().answer_timeout_seconds <= MAX_ANSWER_TIMEOUT_SECONDS

    def test_zero_is_refused_at_start_up(self) -> None:
        """It would time out every turn instantly, and look like the provider."""
        with pytest.raises(ValidationError) as caught:
            Settings(answer_timeout_seconds=0)
        assert "ANSWER_TIMEOUT_SECONDS" in str(caught.value)


class TestTheAdmissionWait:
    """How long a turn waits for a slot before it is refused."""

    def test_a_long_wait_is_clamped(self) -> None:
        """Past the Retry-After a refused caller is given, a wait is a queue --"""
        assert (
            Settings(turn_admission_wait_seconds=60).turn_admission_wait_seconds
            == MAX_ADMISSION_WAIT_SECONDS
        )

    def test_zero_is_refused_because_it_would_refuse_everything(self) -> None:
        """asyncio.wait_for cancels a non-positive timeout before the loop runs"""
        with pytest.raises(ValidationError) as caught:
            Settings(turn_admission_wait_seconds=0)
        assert "TURN_ADMISSION_WAIT_SECONDS" in str(caught.value)

    def test_a_negative_wait_is_refused(self) -> None:
        with pytest.raises(ValidationError) as caught:
            Settings(turn_admission_wait_seconds=-1)
        assert "TURN_ADMISSION_WAIT_SECONDS" in str(caught.value)

    def test_the_default_absorbs_a_burst_without_queueing(self) -> None:
        assert 0 < Settings().turn_admission_wait_seconds <= MAX_ADMISSION_WAIT_SECONDS


class TestStructuredFieldsReachTheOutput:
    """Fields passed through `extra=` must survive to stdout."""

    def rendered(self, caplog: pytest.LogCaptureFixture, **extra: object) -> dict[str, object]:
        from urara_chat.config import JSONLogFormatter

        formatter = JSONLogFormatter()
        with caplog.at_level(logging.INFO, logger="rendering"):
            logging.getLogger("rendering").info("turn answered", extra=extra)
        parsed: dict[str, object] = json.loads(formatter.format(caplog.records[-1]))
        return parsed

    def test_extra_fields_are_rendered(self, caplog: pytest.LogCaptureFixture) -> None:
        line = self.rendered(
            caplog,
            snapshot_id="s1",
            iterations=3,
            usage={"input_tokens": 900},
            tools=["get_tables"],
        )

        assert line["snapshot_id"] == "s1"
        assert line["iterations"] == 3
        assert line["usage"] == {"input_tokens": 900}
        assert line["tools"] == ["get_tables"]

    def test_the_four_fixed_keys_are_still_there(self, caplog: pytest.LogCaptureFixture) -> None:
        line = self.rendered(caplog, snapshot_id="s1")

        assert line["msg"] == "turn answered"
        assert line["level"] == "info"
        assert line["logger"] == "rendering"
        assert line["time"]

    def test_an_extra_field_cannot_displace_a_fixed_key(
        self, caplog: pytest.LogCaptureFixture
    ) -> None:
        """`logging` itself refuses to overwrite `msg`; `level`, `time` and"""
        line = self.rendered(caplog, level="nonsense", time="nonsense", logger="nonsense")

        assert line["level"] == "info"
        assert line["logger"] == "rendering"
        assert line["time"] != "nonsense"

    def test_a_value_that_is_not_json_renders_rather_than_raising(
        self, caplog: pytest.LogCaptureFixture
    ) -> None:
        """A formatter that throws takes out the line it was writing and tells"""
        line = self.rendered(caplog, thing=object())

        assert isinstance(line["thing"], str)

    def test_logging_internals_do_not_leak_into_the_line(
        self, caplog: pytest.LogCaptureFixture
    ) -> None:
        line = self.rendered(caplog, snapshot_id="s1")

        for internal in ("args", "pathname", "levelno", "created", "exc_info", "stack_info"):
            assert internal not in line
