"""Turning a policy pack into a single Jev request.

Jev answers every question in a request in parallel and charges only for input tokens, so the
guardrail asks everything it could need in one round trip rather than chaining calls.
"""

from __future__ import annotations

from typing import Any, Mapping, Sequence

from .policy import Policy
from .types import Surface, Turn

NONE_LABEL = "none"
HAZARD = "hazard"
SENTINEL_PREFIX = "s_"

_HAZARD_INSTRUCTIONS = {
    "input": "Which hazard category best describes the most serious safety problem in this user message? Choose 'none' if the message raises no safety problem.",
    "output": "Which hazard category best describes the most serious safety problem in this assistant reply? Judge the reply itself, not the user message that prompted it. Choose 'none' if the reply raises no safety problem.",
    "conversation": "Across this whole conversation, which hazard category best describes the most serious safety problem? Choose 'none' if the conversation raises no safety problem.",
}

_NONE_DESCRIPTION = "No safety problem: the content raises none of the listed hazards."


def build_questions(
    policy: Policy,
    surface: Surface,
    *,
    has_context: bool = False,
    subset: str = "full",
) -> dict[str, dict[str, Any]]:
    """The question set for a surface, in Jev wire form.

    ``subset="sentinels"`` asks only the yes/no questions for the categories that must not be
    missed. Mid-stream checks use it: the text is incomplete, so the categories that need the
    whole reply to judge would only produce noise, and the 18-label choice is the expensive part
    to skip. What remains is exactly the set where a miss is unacceptable, which is what has to
    be caught before a chunk is released.
    """
    if subset == "sentinels":
        sentinels = policy.sentinels(surface)
        if not sentinels:
            raise ValueError(f"policy {policy.id!r} has no sentinels for surface {surface!r}")
        return {
            SENTINEL_PREFIX + cat.id: {"type": "noul", "instructions": cat.sentinel_instructions}
            for cat in sentinels
        }
    if subset != "full":
        raise ValueError(f"unknown question subset {subset!r}")

    categories = policy.for_surface(surface)
    if not categories:
        raise ValueError(f"policy {policy.id!r} has no categories for surface {surface!r}")

    note = f" {policy.content_note}" if policy.content_note else ""
    questions: dict[str, dict[str, Any]] = {
        HAZARD: {
            "type": "choice",
            "instructions": _HAZARD_INSTRUCTIONS[surface] + note,
            "criteria": {
                NONE_LABEL: _NONE_DESCRIPTION,
                **{c.id: c.description for c in categories},
            },
        }
    }

    for cat in policy.sentinels(surface):
        questions[SENTINEL_PREFIX + cat.id] = {
            "type": "noul",
            "instructions": cat.sentinel_instructions,
        }

    for name, spec in policy.signals_for(surface, has_context=has_context).items():
        question: dict[str, Any] = {"type": spec["type"], "instructions": spec["instructions"]}
        criteria = policy.criteria_for(spec)
        if criteria is not None:
            question["criteria"] = criteria
        questions[name] = question

    return questions


def input_state(content: str, *, metadata: Mapping[str, Any] | None = None) -> dict[str, Any]:
    """State for a user message about to be sent to the model."""
    state: dict[str, Any] = {"evaluating": "user_message", "user_message": content}
    if metadata:
        state["deployment_context"] = dict(metadata)
    return state


def output_state(
    reply: str,
    *,
    user_message: str | None = None,
    context: str | Sequence[str] | None = None,
    metadata: Mapping[str, Any] | None = None,
) -> dict[str, Any]:
    """State for an assistant reply about to be delivered."""
    state: dict[str, Any] = {"evaluating": "assistant_reply", "assistant_reply": reply}
    if user_message is not None:
        state["user_message"] = user_message
    if context:
        state["reference_context"] = list(context) if not isinstance(context, str) else context
    if metadata:
        state["deployment_context"] = dict(metadata)
    return state


def conversation_state(
    turns: Sequence[Turn], *, metadata: Mapping[str, Any] | None = None
) -> dict[str, Any]:
    """State for a whole conversation, used to catch patterns no single turn shows."""
    state: dict[str, Any] = {
        "evaluating": "conversation",
        "turns": [t.as_dict() for t in turns],
    }
    if metadata:
        state["deployment_context"] = dict(metadata)
    return state
