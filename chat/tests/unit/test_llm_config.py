"""LLM settings, the credential checks, and redaction."""

import pytest
from pydantic import SecretStr, ValidationError

from urara_chat.config import Settings

# Deliberately not shaped like a real Google key.
FAKE_KEY = "test-key-shaped-value-0123456789abcdef"

LLM_VARS = (
    "LLM_PROVIDER",
    "LLM_MODEL",
    "LLM_TEMPERATURE",
    "LLM_MAX_OUTPUT_TOKENS",
    "LLM_TIMEOUT_SECONDS",
    "GOOGLE_API_KEY",
    "VERTEX_PROJECT",
    "VERTEX_LOCATION",
)


@pytest.fixture(autouse=True)
def clean_env(monkeypatch: pytest.MonkeyPatch) -> None:
    """A developer's own key in the shell must not decide what these assert."""
    for var in LLM_VARS:
        monkeypatch.delenv(var, raising=False)


def studio(**over: object) -> Settings:
    base: dict[str, object] = {"google_api_key": FAKE_KEY}
    return Settings(**(base | over))  # type: ignore[arg-type]


class TestDefaults:
    def test_documented_defaults(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("GOOGLE_API_KEY", FAKE_KEY)
        s = Settings()

        assert s.llm_provider == "gemini-studio"
        assert s.llm_model == "gemini-2.5-flash"
        assert s.llm_temperature == 0.2
        assert s.llm_max_output_tokens == 2048
        assert s.llm_timeout_seconds == 60.0
        assert s.vertex_location == "us-central1"
        assert s.vertex_project == ""

    def test_every_setting_reads_its_variable(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("LLM_PROVIDER", "vertex")
        monkeypatch.setenv("LLM_MODEL", "gemini-2.5-pro")
        monkeypatch.setenv("LLM_TEMPERATURE", "0.9")
        monkeypatch.setenv("LLM_MAX_OUTPUT_TOKENS", "512")
        monkeypatch.setenv("LLM_TIMEOUT_SECONDS", "12.5")
        monkeypatch.setenv("VERTEX_PROJECT", "my-project")
        monkeypatch.setenv("VERTEX_LOCATION", "europe-west2")

        s = Settings()

        assert s.llm_provider == "vertex"
        assert s.llm_model == "gemini-2.5-pro"
        assert s.llm_temperature == 0.9
        assert s.llm_max_output_tokens == 512
        assert s.llm_timeout_seconds == 12.5
        assert s.vertex_project == "my-project"
        assert s.vertex_location == "europe-west2"


class TestCredentialsMustMatchTheProvider:
    def test_studio_without_a_key_is_refused(self) -> None:
        with pytest.raises(ValidationError) as caught:
            Settings()
        assert "GOOGLE_API_KEY" in str(caught.value), "the message must name the variable to set"

    def test_vertex_without_a_project_is_refused(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("LLM_PROVIDER", "vertex")
        with pytest.raises(ValidationError) as caught:
            Settings()
        assert "VERTEX_PROJECT" in str(caught.value)

    def test_vertex_needs_no_api_key(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Vertex authenticates with Application Default Credentials, so a"""
        monkeypatch.setenv("LLM_PROVIDER", "vertex")
        monkeypatch.setenv("VERTEX_PROJECT", "my-project")

        s = Settings()

        assert s.vertex_project == "my-project"
        assert s.google_api_key.get_secret_value() == ""

    def test_studio_with_a_key_is_valid(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("GOOGLE_API_KEY", FAKE_KEY)
        assert Settings().llm_provider == "gemini-studio"

    @pytest.mark.parametrize("value", ["   ", "\n", "\t "])
    def test_a_whitespace_only_key_is_refused(
        self, value: str, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """`export GOOGLE_API_KEY=$(cat key.txt)` picks up a trailing newline,"""
        monkeypatch.setenv("GOOGLE_API_KEY", value)
        with pytest.raises(ValidationError) as caught:
            Settings()
        assert "GOOGLE_API_KEY" in str(caught.value)

    def test_a_key_with_surrounding_whitespace_is_trimmed(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setenv("GOOGLE_API_KEY", f"  {FAKE_KEY}\n")
        assert Settings().google_api_key.get_secret_value() == FAKE_KEY

    def test_a_whitespace_only_project_is_refused(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("LLM_PROVIDER", "vertex")
        monkeypatch.setenv("VERTEX_PROJECT", "   ")
        with pytest.raises(ValidationError) as caught:
            Settings()
        assert "VERTEX_PROJECT" in str(caught.value)

    def test_an_unknown_provider_is_refused_by_the_type(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setenv("LLM_PROVIDER", "openai")
        monkeypatch.setenv("GOOGLE_API_KEY", FAKE_KEY)
        with pytest.raises(ValidationError) as caught:
            Settings()
        assert "llm_provider" in str(caught.value).lower()


class TestTheKeyDoesNotLeak:
    """A key in a repr is a key in a log line, and from there in whatever ships"""

    def test_repr_carries_no_part_of_the_key(self) -> None:
        rendered = repr(studio())
        assert FAKE_KEY not in rendered
        assert FAKE_KEY[:12] not in rendered
        assert "**********" in rendered

    def test_json_carries_no_part_of_the_key(self) -> None:
        rendered = studio().model_dump_json()
        assert FAKE_KEY not in rendered
        assert FAKE_KEY[:12] not in rendered

    def test_str_of_the_field_is_masked(self) -> None:
        assert str(studio().google_api_key) == "**********"

    def test_model_dump_does_not_expose_it_either(self) -> None:
        """model_dump keeps the SecretStr wrapper rather than unwrapping it."""
        dumped = studio().model_dump()
        assert FAKE_KEY not in str(dumped)

    def test_get_secret_value_returns_the_real_key(self) -> None:
        """The one deliberate way through, used only by the factory."""
        assert studio().google_api_key.get_secret_value() == FAKE_KEY

    def test_an_empty_key_is_falsy(self) -> None:
        """What the credential check relies on, so it never has to unwrap."""
        assert not SecretStr("")
        assert SecretStr("x")


class TestTheStartupFailureLeaksNothing:
    """pydantic's ValidationError embeds the input it was given, and it elides"""

    def test_the_raw_validation_error_does_carry_part_of_the_key(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Documents why the lifespan does not re-raise it."""
        monkeypatch.setenv("GOOGLE_API_KEY", FAKE_KEY)
        monkeypatch.setenv("LLM_PROVIDER", "vertex")

        with pytest.raises(ValidationError) as caught:
            Settings()

        assert FAKE_KEY[-12:] in str(caught.value), (
            "if this ever stops being true the sanitising in main.py can be simplified"
        )

    def test_the_reasons_extracted_for_the_log_carry_none_of_it(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        from urara_chat.main import _reasons

        monkeypatch.setenv("GOOGLE_API_KEY", FAKE_KEY)
        monkeypatch.setenv("LLM_PROVIDER", "vertex")

        with pytest.raises(ValidationError) as caught:
            Settings()

        logged = "; ".join(_reasons(caught.value))
        assert "VERTEX_PROJECT" in logged
        assert FAKE_KEY[-12:] not in logged
        assert FAKE_KEY not in logged

    def test_the_lifespan_raises_a_sanitised_error(self, monkeypatch: pytest.MonkeyPatch) -> None:
        from fastapi.testclient import TestClient

        import urara_chat.main as main
        from urara_chat.config import ConfigurationError, get_settings

        get_settings.cache_clear()
        monkeypatch.setenv("GOOGLE_API_KEY", FAKE_KEY)
        monkeypatch.setenv("LLM_PROVIDER", "vertex")
        monkeypatch.delenv("VERTEX_PROJECT", raising=False)

        with pytest.raises(ConfigurationError) as caught:  # noqa: SIM117
            with TestClient(main.app):
                pass

        assert "VERTEX_PROJECT" in str(caught.value)
        assert FAKE_KEY[-12:] not in str(caught.value)
        get_settings.cache_clear()
