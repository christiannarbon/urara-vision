"""Building the chat model, and describing it without giving anything away."""

from __future__ import annotations

from langchain_core.language_models import BaseChatModel
from langchain_google_genai import ChatGoogleGenerativeAI

from urara_chat.config import Settings


def build_chat_model(settings: Settings) -> BaseChatModel:
    """Build the chat model the pipeline talks to."""
    if settings.llm_provider == "gemini-studio":
        return ChatGoogleGenerativeAI(
            model=settings.llm_model,
            # The one place that unwraps the key. It is a SecretStr everywhere else so this
            # call stays easy to find and easy to keep to one site.
            google_api_key=settings.google_api_key.get_secret_value(),
            temperature=settings.llm_temperature,
            max_output_tokens=settings.llm_max_output_tokens,
            timeout=settings.llm_timeout_seconds,
        )

    if settings.llm_provider == "vertex":
        return ChatGoogleGenerativeAI(
            model=settings.llm_model,
            # No key: Vertex authenticates with Application Default Credentials, so the pod's
            # service account is the credential and there is nothing here to leak.
            vertexai=True,
            project=settings.vertex_project,
            location=settings.vertex_location,
            temperature=settings.llm_temperature,
            max_output_tokens=settings.llm_max_output_tokens,
            timeout=settings.llm_timeout_seconds,
        )

    # Unreachable: llm_provider is a Literal, so pydantic has already refused anything else.
    raise ValueError(f"unsupported LLM provider: {settings.llm_provider!r}")


def describe_model(settings: Settings) -> dict[str, str]:
    """Provider, model ID and region, for /readyz and for message meta."""
    described = {"provider": settings.llm_provider, "model": settings.llm_model}
    if settings.llm_provider == "vertex":
        # Studio has no region to report; an empty string would read as one that failed to resolve
        # rather than one that does not apply.
        described["location"] = settings.vertex_location
    return described
