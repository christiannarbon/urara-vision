"""Building the chat model, and describing it without giving anything away.

This is the only module that reads the API key, and the only one that names a
provider. Everything above it is written against `BaseChatModel` and never
learns which it got, so a third provider is one branch here and one block of
configuration rather than a change that spreads.

**Both providers are served by one class.** 03.1 established that `ChatVertexAI`
is deprecated -- removed in `langchain-google-vertexai` 4.0.0 -- and that
`ChatGoogleGenerativeAI` reaches Vertex through `vertexai=True` with a project
and location. The task note describes two classes from two packages; that was
true when it was written. See `dev_notes/PHASE-03/FINDINGS.md`.

The constructor parameter names here are the *field* names, not the aliases that
`inspect.signature` reports: `google_api_key` rather than `api_key`,
`max_output_tokens` rather than `max_tokens`, `timeout` rather than
`request_timeout`. Both spellings are accepted; these are the ones that match
the attributes.
"""

from __future__ import annotations

from langchain_core.language_models import BaseChatModel
from langchain_google_genai import ChatGoogleGenerativeAI

from urara_chat.config import Settings


def build_chat_model(settings: Settings) -> BaseChatModel:
    """Build the chat model the pipeline talks to.

    The two providers differ only in how they authenticate: Studio takes an API
    key and needs no infrastructure, Vertex takes Application Default
    Credentials against a project. Everything above this function is written
    against BaseChatModel and never learns which it got, so adding a third
    provider is one branch here and one block of configuration.
    """
    if settings.llm_provider == "gemini-studio":
        return ChatGoogleGenerativeAI(
            model=settings.llm_model,
            # The one place in the service that unwraps the key. It is a
            # SecretStr everywhere else precisely so that this call is easy to
            # find and easy to keep to one site.
            google_api_key=settings.google_api_key.get_secret_value(),
            temperature=settings.llm_temperature,
            max_output_tokens=settings.llm_max_output_tokens,
            timeout=settings.llm_timeout_seconds,
        )

    if settings.llm_provider == "vertex":
        return ChatGoogleGenerativeAI(
            model=settings.llm_model,
            # No key: Vertex authenticates with Application Default Credentials,
            # so the pod's service account is the credential and there is
            # nothing here to leak.
            vertexai=True,
            project=settings.vertex_project,
            location=settings.vertex_location,
            temperature=settings.llm_temperature,
            max_output_tokens=settings.llm_max_output_tokens,
            timeout=settings.llm_timeout_seconds,
        )

    # Unreachable: llm_provider is a Literal, so pydantic has already refused
    # anything else. Raising rather than returning a default, because a service
    # quietly answering from the wrong provider is worse than one that stops.
    raise ValueError(f"unsupported LLM provider: {settings.llm_provider!r}")


def describe_model(settings: Settings) -> dict[str, str]:
    """Provider, model ID and region, for /readyz and for message meta.

    Never includes a credential -- this ends up in a readiness response and in
    stored message metadata, both of which are read by people who should not be
    able to read the key.
    """
    described = {"provider": settings.llm_provider, "model": settings.llm_model}
    if settings.llm_provider == "vertex":
        # Studio has no region to report; an empty string would read as one
        # that failed to resolve rather than one that does not apply.
        described["location"] = settings.vertex_location
    return described
