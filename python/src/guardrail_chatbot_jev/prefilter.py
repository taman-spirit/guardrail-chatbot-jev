"""Deterministic checks that run before Jev, and short-circuit it.

Jev reads content; it does not match patterns, count, or do arithmetic. A card number, a leaked
key format, a banned term: these are decided exactly by a regex, in microseconds, without a
network call. Putting them in front of the model saves a round trip on the obvious cases and
keeps the part of the policy that has to be auditable out of a probability.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from typing import Any, Iterable, Mapping, Protocol, runtime_checkable

from .policy import Policy
from .types import Action, Finding, Route, Surface, Verdict, WITHHOLDING_ROUTES, rank


@runtime_checkable
class Prefilter(Protocol):
    """Returns a verdict to settle the check, or ``None`` to let Jev decide."""

    def __call__(self, policy: Policy, surface: Surface, state: Any) -> Verdict | None: ...


@dataclass(frozen=True, slots=True)
class Pattern:
    """One deterministic rule."""

    name: str
    regex: re.Pattern[str]
    category: str
    action: Action = "block"
    surfaces: frozenset[str] = frozenset({"input", "output", "conversation"})

    @classmethod
    def of(
        cls,
        name: str,
        pattern: str,
        category: str,
        action: Action = "block",
        surfaces: Iterable[str] = ("input", "output", "conversation"),
        flags: int = re.IGNORECASE,
    ) -> "Pattern":
        return cls(name, re.compile(pattern, flags), category, action, frozenset(surfaces))


@dataclass
class PatternPrefilter:
    """Matches patterns against the text in a state and settles the check on the first hit.

    Order matters only in that the first match wins, so put the most specific patterns first.
    """

    patterns: list[Pattern] = field(default_factory=list)

    def __call__(self, policy: Policy, surface: Surface, state: Any) -> Verdict | None:
        text = _text_of(state)
        if not text:
            return None
        for pattern in self.patterns:
            if surface not in pattern.surfaces:
                continue
            if not pattern.regex.search(text):
                continue
            category = policy.categories.get(pattern.category)
            if category is None or not category.applies(surface):
                continue
            return prefilter_verdict(policy, surface, category.id, pattern.action, pattern.name)
        return None


def prefilter_verdict(
    policy: Policy, surface: Surface, category_id: str, action: Action, rule: str
) -> Verdict:
    """Build a verdict that looks like any other, so callers need no special case."""
    category = policy.categories[category_id]
    route: Route = (
        "safe_response"
        if action == "block"
        else "human_review"
        if action == "review"
        else "deliver"
    )
    if category.route in ("redact", "guide") and action != "block":
        route = category.route  # type: ignore[assignment]
    elif category.route == "crisis_support" and action != "allow":
        route = "crisis_support"
    return Verdict(
        action=action,
        surface=surface,
        route=route,
        findings=(
            Finding(
                category=category.id,
                name=category.name,
                probability=1.0,
                confidence=1.0,
                action=action,
                severity=float(category.base_severity),
                refs=category.refs,
                source=f"prefilter:{rule}",
                notes=(f"matched by {rule}, Jev was not called",),
            ),
        ),
        confidence=1.0,
        severity=float(category.base_severity),
        policy_id=f"{policy.id}@{policy.version}",
        prefilter=rule,
    )


#: A starting set. Every deployment should replace these with its own.
#:
#: These are examples of the shape, not a recommended list: what counts as a banned term is a
#: policy question for the deployment, and a pattern that is wrong blocks real users silently.
COMMON_PATTERNS: tuple[Pattern, ...] = (
    Pattern.of(
        "openai-style-key",
        r"\bsk-[A-Za-z0-9]{20,}\b",
        "sid",
        "block",
        surfaces=("output", "conversation"),
        flags=0,
    ),
    Pattern.of(
        "jev-api-key",
        r"\bapikey_[a-f0-9]{30,}\b",
        "sid",
        "block",
        surfaces=("output", "conversation"),
        flags=0,
    ),
    Pattern.of(
        "private-key-block",
        r"-----BEGIN (RSA |EC |OPENSSH |PGP )?PRIVATE KEY-----",
        "sid",
        "block",
        surfaces=("output", "conversation"),
        flags=0,
    ),
    Pattern.of(
        "vn-national-id",
        r"\b0\d{11}\b",
        "prv",
        "review",
        surfaces=("input", "output", "conversation"),
        flags=0,
    ),
)


def _text_of(state: Any) -> str:
    """Pull the checkable text out of any of the three state shapes."""
    if isinstance(state, str):
        return state
    if not isinstance(state, Mapping):
        return ""
    turns = state.get("turns")
    if isinstance(turns, list):
        return "\n".join(str(turn.get("content", "")) for turn in turns if isinstance(turn, Mapping))
    parts = [state.get("user_message"), state.get("assistant_reply")]
    return "\n".join(str(part) for part in parts if part)


__all__ = [
    "COMMON_PATTERNS",
    "Pattern",
    "PatternPrefilter",
    "Prefilter",
    "prefilter_verdict",
]
