"""Typed results returned by the guardrail."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Literal, Mapping, Sequence

Action = Literal["allow", "flag", "review", "block"]
Surface = Literal["input", "output", "conversation"]
Route = Literal["deliver", "redact", "guide", "crisis_support", "human_review", "safe_response", "deliver_and_audit"]

#: Routes that withhold the content and put something else in its place.
WITHHOLDING_ROUTES: frozenset[str] = frozenset({"safe_response", "crisis_support", "human_review"})

#: The enforcement decision, ordered from least to most restrictive.
#:
#: Only four steps, and every one of them is a different answer to "does this content go out?".
#: How to handle it once decided -- mask the personal data, steer the reply, hand it to a human --
#: is a separate axis, carried by ``Verdict.route``. Keeping them apart is what lets a rule move a
#: verdict one step without landing on a handling mode that makes no sense for the hazard.
LADDER: tuple[Action, ...] = ("allow", "flag", "review", "block")


def rank(action: str) -> int:
    """Position of an action on the ladder; unknown actions sort as ``allow``."""
    try:
        return LADDER.index(action)  # type: ignore[arg-type]
    except ValueError:
        return 0


def stronger(a: str, b: str) -> Action:
    """The more restrictive of two actions."""
    return a if rank(a) >= rank(b) else b  # type: ignore[return-value]


def weaker(a: str, b: str) -> Action:
    """The less restrictive of two actions."""
    return a if rank(a) <= rank(b) else b  # type: ignore[return-value]


def shift(action: str, steps: int) -> Action:
    """Move an action along the ladder, clamped at both ends."""
    return LADDER[max(0, min(len(LADDER) - 1, rank(action) + steps))]


@dataclass(frozen=True, slots=True)
class Finding:
    """One hazard category that fired, with the evidence behind it."""

    category: str
    name: str
    probability: float
    confidence: float
    action: Action
    severity: float
    refs: tuple[str, ...] = ()
    source: str = "category"
    notes: tuple[str, ...] = ()
    #: Raised by a sentinel alone while the hazard choice gave its category next to nothing.
    uncorroborated: bool = False
    #: Uncorroborated and below its block band: never_below does not lift it.
    weak: bool = False

    def as_dict(self) -> dict[str, Any]:
        return {
            "category": self.category,
            "name": self.name,
            "probability": round(self.probability, 4),
            "confidence": round(self.confidence, 4),
            "action": self.action,
            "severity": round(self.severity, 2),
            "refs": list(self.refs),
            "source": self.source,
            "notes": list(self.notes),
        }


@dataclass(frozen=True, slots=True)
class Usage:
    input_tokens: int = 0
    output_tokens: int = 0

    def as_dict(self) -> dict[str, int]:
        return {"input_tokens": self.input_tokens, "output_tokens": self.output_tokens}


@dataclass(frozen=True, slots=True)
class Verdict:
    """The decision for one piece of content."""

    action: Action
    surface: Surface
    findings: tuple[Finding, ...] = ()
    signals: Mapping[str, Any] = field(default_factory=dict)
    confidence: float = 1.0
    severity: float = 0.0
    route: Route = "deliver"
    applied_rules: tuple[str, ...] = ()
    model: str = ""
    usage: Usage = field(default_factory=Usage)
    latency_ms: float = 0.0
    degraded: bool = False
    error: str | None = None
    policy_id: str = ""
    #: True when this verdict came from the cache rather than a fresh call.
    cached: bool = False
    #: True when only the sentinel questions were asked, as mid-stream checks do.
    partial: bool = False
    #: Set when a prefilter decided without calling Jev.
    prefilter: str | None = None
    #: What the in-context output check found, when it ran.
    context: Mapping[str, Any] | None = None
    #: "priority" for review or worse, "sample" for a flag: what a person should look at later.
    audit: str | None = None

    @property
    def allowed(self) -> bool:
        """True when the content may be delivered as-is or with a flag only."""
        return rank(self.action) <= rank("flag")

    @property
    def blocked(self) -> bool:
        return self.action == "block"

    @property
    def needs_human(self) -> bool:
        return self.action == "review"

    @property
    def deliverable(self) -> bool:
        """True when the content still reaches the user, possibly redacted or steered first."""
        return self.route not in WITHHOLDING_ROUTES

    @property
    def top(self) -> Finding | None:
        return self.findings[0] if self.findings else None

    @property
    def categories(self) -> tuple[str, ...]:
        return tuple(f.category for f in self.findings)

    def as_dict(self) -> dict[str, Any]:
        return {
            "action": self.action,
            "allowed": self.allowed,
            "surface": self.surface,
            "severity": round(self.severity, 2),
            "confidence": round(self.confidence, 4),
            "route": self.route,
            "findings": [f.as_dict() for f in self.findings],
            "signals": dict(self.signals),
            "applied_rules": list(self.applied_rules),
            "policy_id": self.policy_id,
            "model": self.model,
            "usage": self.usage.as_dict(),
            "latency_ms": round(self.latency_ms, 1),
            "degraded": self.degraded,
            "cached": self.cached,
            "partial": self.partial,
            "prefilter": self.prefilter,
            "error": self.error,
            "context": dict(self.context) if self.context is not None else None,
            "audit": self.audit,
        }


@dataclass(frozen=True, slots=True)
class Turn:
    """One message in a conversation."""

    role: Literal["user", "assistant", "system", "tool"]
    content: str

    def as_dict(self) -> dict[str, str]:
        return {"role": self.role, "content": self.content}


def as_turns(items: Sequence[Any]) -> tuple[Turn, ...]:
    """Accept ``Turn`` objects, ``{"role", "content"}`` dicts, or ``(role, content)`` pairs."""
    out: list[Turn] = []
    for item in items:
        if isinstance(item, Turn):
            out.append(item)
        elif isinstance(item, Mapping):
            out.append(Turn(role=item.get("role", "user"), content=str(item.get("content", ""))))
        elif isinstance(item, (tuple, list)) and len(item) == 2:
            out.append(Turn(role=item[0], content=str(item[1])))
        else:
            raise TypeError(f"cannot read a conversation turn from {item!r}")
    return tuple(out)


class GuardrailError(RuntimeError):
    """The guardrail could not reach a verdict."""
