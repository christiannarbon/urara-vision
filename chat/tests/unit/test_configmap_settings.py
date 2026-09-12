"""Every setting must be reachable from the Kubernetes ConfigMap."""

from pathlib import Path
from typing import Any

import pytest
import yaml
from pydantic import SecretStr

from urara_chat.config import Settings

# Mounted by CHAT_RUN in the Makefile; the relative path is for a bare `uv run pytest` from the
# chat directory.
_CANDIDATES = (
    Path("/manifests/chat.yaml"),
    Path(__file__).resolve().parents[3] / "k8s" / "base" / "chat.yaml",
)

CONFIGMAP_NAME = "relviz-chat-config"

# Reaches the pod another way, so it does not belong in a ConfigMap.
SUPPLIED_ELSEWHERE = {
    "BACKEND_API_TOKEN": "a secretKeyRef on the Deployment",
    "GOOGLE_API_KEY": "unused; Vertex authenticates with ADC, not a key",
}

# ConfigMap values that deliberately differ from the field's own default.
DIFFERENT_ON_PURPOSE = {
    "LLM_PROVIDER": "the cluster runs Vertex, the code defaults to the Studio API",
    "VERTEX_PROJECT": "no default is possible; each overlay sets its own",
}


def manifest_file() -> Path:
    for candidate in _CANDIDATES:
        if candidate.is_file():
            return candidate
    # Not a skip: this is the only thing standing between a new setting and a
    # silent default in the cluster.
    pytest.fail(f"k8s/base/chat.yaml not found at any of {[str(c) for c in _CANDIDATES]}")


def documents() -> list[dict[str, Any]]:
    return [d for d in yaml.safe_load_all(manifest_file().read_text()) if d]


def configmap_data() -> dict[str, str]:
    """Matched by kind as well as name: the Deployment's envFrom carries the same name."""
    for doc in documents():
        if doc.get("kind") == "ConfigMap" and doc["metadata"]["name"] == CONFIGMAP_NAME:
            return {k: str(v) for k, v in doc["data"].items()}
    pytest.fail(f"no ConfigMap named {CONFIGMAP_NAME} in {manifest_file()}")


def deployment_spec() -> dict[str, Any]:
    for doc in documents():
        if doc.get("kind") == "Deployment" and doc["metadata"]["name"] == "chat":
            spec: dict[str, Any] = doc["spec"]["template"]["spec"]
            return spec
    pytest.fail(f"no chat Deployment in {manifest_file()}")


def setting_names() -> set[str]:
    return {name.upper() for name in Settings.model_fields}


def test_every_setting_is_declared() -> None:
    """A setting the ConfigMap does not name cannot be changed in the cluster at all."""
    missing = sorted(setting_names() - set(configmap_data()) - set(SUPPLIED_ELSEWHERE))
    assert not missing, (
        f"settings absent from {CONFIGMAP_NAME}: {missing}. Add each with the "
        "default from config.py, or add it to SUPPLIED_ELSEWHERE with a reason."
    )


def _agrees(declared: str, default: Any) -> bool:
    """Whether a declared value means the same as the field's own default."""
    if isinstance(default, SecretStr):
        default = default.get_secret_value()
    if isinstance(default, bool):
        return declared.lower() == str(default).lower()
    if isinstance(default, int | float):
        try:
            return float(declared) == float(default)
        except ValueError:
            return False
    return declared == str(default)


def test_the_declared_defaults_match_the_code() -> None:
    """A ConfigMap default that has drifted from the field's own is worse than none."""
    data = configmap_data()
    drifted: list[str] = []
    for name, field in Settings.model_fields.items():
        key = name.upper()
        if key in DIFFERENT_ON_PURPOSE or key not in data:
            continue
        if not _agrees(data[key], field.default):
            drifted.append(f"{key}: ConfigMap says {data[key]!r}, code says {field.default!r}")
    assert not drifted, "ConfigMap defaults have drifted from config.py: " + "; ".join(drifted)


def test_the_grace_period_covers_a_whole_turn() -> None:
    """A shorter grace period would SIGKILL a turn the service is still allowed to be running."""
    grace = deployment_spec()["terminationGracePeriodSeconds"]
    timeout = float(configmap_data()["ANSWER_TIMEOUT_SECONDS"])
    assert grace >= timeout, (
        f"terminationGracePeriodSeconds is {grace}s but a turn may run "
        f"{timeout}s; a rollout would kill it mid-answer."
    )
