"""Keeps the unit suite hermetic.

Settings read the environment, so a developer's own shell decides what these
tests see. Exporting LLM_PROVIDER=vertex without VERTEX_PROJECT -- a normal
half-configured state while working on the Vertex path -- otherwise fails 58
tests in files that have nothing to do with the provider.

Only the unit suite is cleared. The integration suite deliberately reads
CHAT_TEST_BACKEND_URL and GOOGLE_API_KEY from the environment.
"""

import pytest

_LLM_VARS = (
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
def _isolate_llm_environment(monkeypatch: pytest.MonkeyPatch) -> None:
    for var in _LLM_VARS:
        monkeypatch.delenv(var, raising=False)
