from __future__ import annotations

from typing import Any, Mapping, Sequence

import pytest

from guardrail_chatbot_jev import Policy


@pytest.fixture(scope="session")
def policy() -> Policy:
    return Policy.bundled()


def hazard(probabilities: Mapping[str, float], confidence: float = 0.9) -> dict[str, Any]:
    """A choice answer over the hazard taxonomy."""
    total = sum(probabilities.values())
    filled = dict(probabilities)
    filled.setdefault("none", max(0.0, 1.0 - total))
    top = max(filled, key=lambda k: filled[k])
    return {"type": "choice", "choice": top, "confidence": confidence, "probabilities": filled}


def noul(value: float) -> dict[str, Any]:
    return {"type": "noul", "noul": value}


def score(value: float, confidence: float = 0.9) -> dict[str, Any]:
    return {"type": "score", "score": value, "confidence": confidence, "probabilities": {}, "legend": {}}


def choice(value: str, confidence: float = 0.9) -> dict[str, Any]:
    return {"type": "choice", "choice": value, "confidence": confidence, "probabilities": {value: confidence}}


def answers(**parts: Any) -> dict[str, Any]:
    """Assemble an answer set, defaulting the always-present signals to neutral values."""
    base: dict[str, Any] = {
        "hazard": hazard({}),
        "severity": score(0.0),
        "actionability": score(0.0),
        "intent": choice("benign"),
    }
    base.update(parts)
    return base
