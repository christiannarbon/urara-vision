"""The model factory.

Two things are worth asserting here and both are about what reaches the SDK: the
generation parameters, because a temperature that silently fails to arrive
changes every answer without failing anything; and the credential, because this
is the only place in the service that unwraps it.

Nothing is called. Constructing a chat model makes no network request, so the
real classes are used and the arguments are read back off the instance.
"""

import json

import pytest
from langchain_core.language_models import BaseChatModel
from langchain_google_genai import ChatGoogleGenerativeAI

from urara_chat.config import Settings
from urara_chat.llm.factory import build_chat_model, describe_model

REAL_KEY = "AIza-this-is-the-real-key-value-0123456789"


@pytest.fixture(autouse=True)
def clean_env(monkeypatch: pytest.MonkeyPatch) -> None:
    """A developer's own key or project must not decide what these assert."""
    for var in (
        "LLM_PROVIDER",
        "LLM_MODEL",
        "LLM_TEMPERATURE",
        "LLM_MAX_OUTPUT_TOKENS",
        "LLM_TIMEOUT_SECONDS",
        "GOOGLE_API_KEY",
        "VERTEX_PROJECT",
        "VERTEX_LOCATION",
    ):
        monkeypatch.delenv(var, raising=False)


def studio_settings(**over: object) -> Settings:
    base: dict[str, object] = {
        "llm_provider": "gemini-studio",
        "llm_model": "gemini-2.5-flash",
        "llm_temperature": 0.2,
        "llm_max_output_tokens": 2048,
        "llm_timeout_seconds": 60.0,
        "google_api_key": REAL_KEY,
    }
    return Settings(**(base | over))  # type: ignore[arg-type]


def vertex_settings(**over: object) -> Settings:
    base: dict[str, object] = {
        "llm_provider": "vertex",
        "llm_model": "gemini-2.5-pro",
        "llm_temperature": 0.5,
        "llm_max_output_tokens": 1024,
        "llm_timeout_seconds": 30.0,
        "vertex_project": "my-project",
        "vertex_location": "europe-west2",
    }
    return Settings(**(base | over))  # type: ignore[arg-type]


class TestStudio:
    def test_returns_a_chat_model(self) -> None:
        model = build_chat_model(studio_settings())
        assert isinstance(model, BaseChatModel)
        assert isinstance(model, ChatGoogleGenerativeAI)

    def test_the_generation_parameters_reach_the_sdk(self) -> None:
        """A temperature that fails to arrive changes every answer without
        failing anything, so the values are read back rather than assumed."""
        model = build_chat_model(
            studio_settings(
                llm_model="gemini-2.5-pro",
                llm_temperature=0.9,
                llm_max_output_tokens=512,
                llm_timeout_seconds=12.5,
            )
        )
        assert model.model.endswith("gemini-2.5-pro")
        assert model.temperature == 0.9
        assert model.max_output_tokens == 512
        assert model.timeout == 12.5

    def test_the_key_arrives_unwrapped(self) -> None:
        model = build_chat_model(studio_settings())
        assert model.google_api_key is not None
        assert model.google_api_key.get_secret_value() == REAL_KEY

    def test_it_is_not_put_into_vertex_mode(self) -> None:
        model = build_chat_model(studio_settings())
        assert not model.vertexai


class TestVertex:
    def test_returns_a_chat_model(self) -> None:
        model = build_chat_model(vertex_settings())
        assert isinstance(model, BaseChatModel)
        assert isinstance(model, ChatGoogleGenerativeAI)

    def test_project_and_location_reach_the_sdk(self) -> None:
        model = build_chat_model(vertex_settings())
        assert model.vertexai is True
        assert model.project == "my-project"
        assert model.location == "europe-west2"

    def test_the_generation_parameters_reach_the_sdk(self) -> None:
        model = build_chat_model(vertex_settings())
        assert model.model.endswith("gemini-2.5-pro")
        assert model.temperature == 0.5
        assert model.max_output_tokens == 1024
        assert model.timeout == 30.0

    def test_no_api_key_is_passed(self) -> None:
        """Vertex authenticates with Application Default Credentials. Passing a
        key would be both unnecessary and a credential in a second place."""
        model = build_chat_model(vertex_settings())
        assert not model.google_api_key or not model.google_api_key.get_secret_value()

    def test_a_key_in_the_environment_is_still_not_forwarded(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Someone with both configured should not have the key sent to Vertex
        just because it happened to be set."""
        model = build_chat_model(vertex_settings(google_api_key=REAL_KEY))
        assert not model.google_api_key or model.google_api_key.get_secret_value() != REAL_KEY


class TestDescribeModel:
    def test_studio(self) -> None:
        described = describe_model(studio_settings())
        assert described["provider"] == "gemini-studio"
        assert described["model"] == "gemini-2.5-flash"
        # Studio has no region; an empty string would read as one that failed to
        # resolve rather than one that does not apply.
        assert "location" not in described

    def test_vertex(self) -> None:
        described = describe_model(vertex_settings())
        assert described["provider"] == "vertex"
        assert described["model"] == "gemini-2.5-pro"
        assert described["location"] == "europe-west2"

    @pytest.mark.parametrize("build", [studio_settings, vertex_settings])
    def test_carries_no_part_of_the_key(self, build: object) -> None:
        """This ends up in a readiness response and in stored message metadata,
        both read by people who should not be able to read the key."""
        settings = build(google_api_key=REAL_KEY)  # type: ignore[operator]
        rendered = json.dumps(describe_model(settings))

        assert REAL_KEY not in rendered
        assert "AIza" not in rendered
        assert "**" not in rendered, "not even the mask belongs in a description"

    @pytest.mark.parametrize("build", [studio_settings, vertex_settings])
    def test_is_json_serialisable(self, build: object) -> None:
        """It goes into a JSON response and into stored message metadata."""
        described = describe_model(build())  # type: ignore[operator]
        assert json.loads(json.dumps(described)) == described


class TestNoSilentFallback:
    def test_an_unknown_provider_is_refused_by_the_settings(self) -> None:
        """The Literal is the guard; the factory's raise is only for a provider
        added to the type and forgotten here."""
        from pydantic import ValidationError

        with pytest.raises(ValidationError):
            Settings(llm_provider="openai", google_api_key=REAL_KEY)  # type: ignore[arg-type]

    def test_the_factory_raises_rather_than_defaulting(self) -> None:
        """A service quietly answering from the wrong provider is worse than one
        that stops."""
        settings = studio_settings()
        object.__setattr__(settings, "llm_provider", "openai")

        with pytest.raises(ValueError, match="unsupported LLM provider"):
            build_chat_model(settings)
