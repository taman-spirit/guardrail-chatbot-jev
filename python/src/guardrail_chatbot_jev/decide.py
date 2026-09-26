"""The decision engine: Jev answers in, a verdict out.

This module makes no network calls and holds no state, so the whole policy can be tested
against recorded answers without an API key.
"""

from __future__ import annotations

import time
from dataclasses import replace
from typing import Any, Mapping

from .policy import Category, Policy, SentinelCorroboration
from .questions import HAZARD, NONE_LABEL, SENTINEL_PREFIX
from .types import (
    Action,
    Finding,
    Route,
    Surface,
    Usage,
    Verdict,
    rank,
    shift,
    stronger,
    weaker,
)

#: Handling modes a category may ask for when it fires below the block line.
_CATEGORY_ROUTES = frozenset({"redact", "guide", "crisis_support"})


def decide(
    policy: Policy,
    surface: Surface,
    answers: Mapping[str, Mapping[str, Any]],
    *,
    model: str = "",
    usage: Usage | None = None,
    latency_ms: float = 0.0,
) -> Verdict:
    """Turn one set of Jev answers into a verdict."""
    signals = _read_signals(policy, answers)
    probabilities, confidences, sentinel_sourced = _hazard_probabilities(policy, surface, answers)
    corroboration = policy.sentinel_corroboration()
    choice = (answers.get(HAZARD) or {}).get("probabilities") or {}

    findings = [
        f
        for cat in policy.for_surface(surface)
        if (
            f := _finding(
                cat,
                surface,
                probabilities.get(cat.id, 0.0),
                confidences.get(cat.id, 1.0),
                sentinel=cat.id in sentinel_sourced,
                uncorroborated=corroboration is not None
                and cat.id in sentinel_sourced
                and _num(choice.get(cat.id)) < corroboration.min_choice,
                never_weak=corroboration is not None and cat.id in corroboration.weak_except,
            )
        )
    ]

    findings, floors, applied = _apply_rules(policy, surface, findings, signals, probabilities)
    if corroboration is not None:
        if surface == "output":
            findings = _cap_uncorroborated_on_refusal(findings, signals, corroboration)
        if corroboration.redact_instead_of_block_below > 0:
            findings = [_redact_instead_of_block(policy, f, corroboration) for f in findings]
        if corroboration.weak_at_most_flag:
            findings = [
                replace(f, action="flag", notes=f.notes + (f"uncorroborated sentinel below its block band: {f.action} -> flag",))
                if f.weak and rank(f.action) > rank("flag")
                else f
                for f in findings
            ]

    action: Action = "allow"
    for finding in findings:
        action = stronger(action, finding.action)
    for floor in floors:
        action = stronger(action, floor)

    confidence = _overall_confidence(answers, findings, confidences)
    # A rule can vouch for the content strongly enough that low confidence is not a reason to hold
    # it, such as a pack's "this only mentions a place" rule. Floors still apply.
    gate_off = any(
        (rule.get("then") or {}).get("skip_confidence_gate") and rule.get("id") in applied
        for rule in policy.rules
    )
    if not gate_off:
        action, escalated = _confidence_gate(
            policy, action, confidence, findings, probabilities, surface, choice, signals,
            corroboration.min_choice if corroboration is not None else 0.0,
        )
        if escalated:
            applied.append("confidence-gate")

    findings.sort(key=lambda f: (-rank(f.action), -f.probability))
    route = _route(policy, findings, action)
    severity = _severity(signals, findings)

    return Verdict(
        action=action,
        surface=surface,
        findings=tuple(findings),
        signals=signals,
        confidence=confidence,
        severity=severity,
        route=route,
        applied_rules=tuple(applied),
        model=model,
        usage=usage or Usage(),
        latency_ms=latency_ms,
        policy_id=f"{policy.id}@{policy.version}",
    )


def error_verdict(policy: Policy, surface: Surface, error: Exception, *, latency_ms: float = 0.0) -> Verdict:
    """The verdict to use when Jev could not be reached.

    Fail-closed is the default: an unavailable guardrail is not an approval.
    """
    action: Action = policy.error_action() if policy.fail_closed(surface) else "allow"  # type: ignore[assignment]
    # The route has to follow the action here too: a degraded "review" that still says "deliver"
    # reads as permission to send, which is the opposite of what failing closed means.
    route: Route = (
        "safe_response" if action == "block" else "human_review" if action == "review" else "deliver"
    )
    return Verdict(
        action=action,
        surface=surface,
        route=route,
        confidence=0.0,
        degraded=True,
        error=f"{type(error).__name__}: {error}",
        latency_ms=latency_ms,
        policy_id=f"{policy.id}@{policy.version}",
    )


# -- internals --------------------------------------------------------


def _read_signals(policy: Policy, answers: Mapping[str, Mapping[str, Any]]) -> dict[str, Any]:
    signals: dict[str, Any] = {}
    for name in policy.signals:
        answer = answers.get(name)
        if not answer:
            continue
        kind = answer.get("type")
        if kind == "noul":
            signals[name] = float(answer.get("noul", 0.0))
        elif kind == "score":
            signals[name] = float(answer.get("score", 0.0))
        elif kind == "choice":
            signals[name] = answer.get("choice")
    return signals


def _hazard_probabilities(
    policy: Policy, surface: Surface, answers: Mapping[str, Mapping[str, Any]]
) -> tuple[dict[str, float], dict[str, float], set[str]]:
    """Per-category probability, taking the stronger of the choice question and its sentinel."""
    probabilities: dict[str, float] = {}
    confidences: dict[str, float] = {}
    sentinel_sourced: set[str] = set()

    hazard = answers.get(HAZARD) or {}
    hazard_confidence = float(hazard.get("confidence", 1.0))
    for label, value in (hazard.get("probabilities") or {}).items():
        if label == NONE_LABEL:
            continue
        probabilities[label] = float(value)
        confidences[label] = hazard_confidence

    for cat in policy.sentinels(surface):
        answer = answers.get(SENTINEL_PREFIX + cat.id)
        if not answer:
            continue
        value = float(answer.get("noul", 0.0))
        if value >= probabilities.get(cat.id, 0.0):
            probabilities[cat.id] = value
            sentinel_sourced.add(cat.id)
            # A noul reports belief, not uncertainty: 0.4 means "40% likely", which the threshold
            # already accounts for. It carries no confidence of its own, so the request-level
            # confidence stands in. Reading distance from 0.5 as doubt would double-count the
            # probability and send every mid-range sentinel to review whatever the threshold says.
            confidences[cat.id] = hazard_confidence
    return probabilities, confidences, sentinel_sourced


def _finding(
    cat: Category,
    surface: Surface,
    probability: float,
    confidence: float,
    *,
    sentinel: bool = False,
    uncorroborated: bool = False,
    never_weak: bool = False,
) -> Finding | None:
    bands = cat.threshold(surface)
    if not bands:
        return None
    if probability >= bands["block"]:
        action: Action = "block"
    elif probability >= bands["review"]:
        action = "review"
    elif probability >= bands["flag"]:
        action = "flag"
    else:
        return None

    notes: list[str] = []
    if cat.route in _CATEGORY_ROUTES:
        notes.append(f"handled by {cat.route}")
    # A sentinel alone, which the hazard choice does not back, answers at its own band: it may still
    # reach block by crossing the block band, but never_below does not lift it there.
    # A category in weak_except (self-harm, for one) is never weakened: its crisis handling is
    # support, not a penalty, so a lone sentinel still reaches it.
    weak = uncorroborated and probability < bands["block"] and not never_weak
    if cat.never_below and weak:
        notes.append(f"uncorroborated sentinel: never below {cat.never_below} not applied")
    elif cat.never_below:
        action = stronger(action, cat.never_below)
        notes.append(f"never below {cat.never_below}")

    return Finding(
        category=cat.id,
        name=cat.name,
        probability=probability,
        confidence=confidence,
        action=action,
        severity=float(cat.base_severity),
        refs=cat.refs,
        source="sentinel" if sentinel else "category",
        notes=tuple(notes),
        uncorroborated=uncorroborated,
        weak=weak,
    )


def _num(value: Any) -> float:
    try:
        return float(value or 0.0)
    except (TypeError, ValueError):
        return 0.0


def _cap_uncorroborated_on_refusal(
    findings: list[Finding], signals: Mapping[str, Any], c: SentinelCorroboration
) -> list[Finding]:
    """Lower to flag an uncorroborated sentinel finding on a reply that declines.

    Measured against Jev, a plain refusal drew sentinel answers of up to 0.26 for categories the
    reply never touched, while the hazard choice gave them nothing. The categories a refusal can
    still leak, such as a system prompt or personal data, are exempt.
    """
    refusal = signals.get("refusal")
    if not isinstance(refusal, float) or refusal < c.refusal:
        return findings
    out = []
    for f in findings:
        if (
            f.uncorroborated
            and f.probability < c.refusal_max_sentinel
            and f.category not in c.refusal_except
            and rank(f.action) > rank("flag")
        ):
            f = replace(f, action="flag", notes=f.notes + (f"refusal with an uncorroborated sentinel: {f.action} -> flag",))
        out.append(f)
    return out


def _redact_instead_of_block(policy: Policy, f: Finding, c: SentinelCorroboration) -> Finding:
    """Personal data is handled by masking it; blocking a whole reply on a sentinel alone is the
    wrong remedy. Measured: a reply naming "anh Nam" drew s_prv up to 0.50 with the choice at 0.01."""
    if (
        f.uncorroborated
        and f.action == "block"
        and f.probability < c.redact_instead_of_block_below
        and policy.categories[f.category].route == "redact"
    ):
        return replace(f, action="review", notes=f.notes + ("uncorroborated sentinel on a redacted category: block -> review",))
    return f


def _matches(op: str, value: Any, target: Any) -> bool:
    if op in ("==", "!="):
        equal = str(value) == str(target)
        return equal if op == "==" else not equal
    try:
        left, right = float(value), float(target)
    except (TypeError, ValueError):
        return False
    return {
        ">=": left >= right,
        "<=": left <= right,
        ">": left > right,
        "<": left < right,
    }.get(op, False)


def _apply_rules(
    policy: Policy,
    surface: Surface,
    findings: list[Finding],
    signals: Mapping[str, Any],
    probabilities: Mapping[str, float],
) -> tuple[list[Finding], list[str], list[str]]:
    floors: list[str] = []
    applied: list[str] = []

    for rule in policy.rules:
        when = rule.get("when") or {}
        signal = when.get("signal")
        if signal not in signals:
            continue
        if not _matches(str(when.get("op", "==")), signals[signal], when.get("value")):
            continue

        then = rule.get("then") or {}
        rule_id = str(rule.get("id", "rule"))
        applied.append(rule_id)
        exempt = set(rule.get("except_categories") or ())

        added = then.get("add_finding")
        if added and added in policy.categories and added not in {f.category for f in findings}:
            cat = policy.categories[added]
            if cat.applies(surface):
                findings.append(
                    Finding(
                        category=cat.id,
                        name=cat.name,
                        probability=probabilities.get(cat.id, 0.0),
                        confidence=1.0,
                        action="flag",
                        severity=float(cat.base_severity),
                        refs=cat.refs,
                        source=f"rule:{rule_id}",
                        notes=(f"raised by {rule_id}",),
                    )
                )

        steps = int(then.get("upgrade", 0)) - int(then.get("downgrade", 0))
        cap = then.get("cap_action")
        if steps or cap:
            findings = [_adjust(f, steps, cap, rule_id, policy) if f.category not in exempt else f for f in findings]

        if floor := then.get("floor_action"):
            floors.append(str(floor))

    return findings, floors, applied


def _adjust(finding: Finding, steps: int, cap: str | None, rule_id: str, policy: Policy) -> Finding:
    action: Action = shift(finding.action, steps) if steps else finding.action
    if steps < 0:
        # A softening rule lowers the response, it does not erase the record: a finding that
        # fired stays visible at "flag" so the deployment can still count and audit it.
        action = stronger(action, "flag")
    if cap:
        action = weaker(action, cap)
    never_below = policy.categories[finding.category].never_below
    if never_below and not finding.weak:
        action = stronger(action, never_below)
    if action == finding.action:
        return finding
    return replace(finding, action=action, notes=finding.notes + (f"{rule_id}: {finding.action} -> {action}",))


def _overall_confidence(
    answers: Mapping[str, Mapping[str, Any]],
    findings: list[Finding],
    confidences: Mapping[str, float],
) -> float:
    if findings:
        return min(confidences.get(f.category, 1.0) for f in findings)
    hazard = answers.get(HAZARD) or {}
    return float(hazard.get("confidence", 1.0))


def _confidence_gate(
    policy: Policy,
    action: Action,
    confidence: float,
    findings: list[Finding],
    probabilities: Mapping[str, float],
    surface: Surface,
    choice: Mapping[str, Any] | None = None,
    signals: Mapping[str, Any] | None = None,
    min_choice: float = 0.0,
) -> tuple[Action, bool]:
    """A low-confidence answer is not evidence of safety, so it escalates toward review."""
    if confidence >= policy.min_confidence() or policy.on_low_confidence() != "escalate":
        return action, False
    opts = policy.confidence_gate()
    intent = (signals or {}).get("intent")
    if isinstance(intent, str) and intent in opts.skip_when_intent and confidence >= opts.skip_min_confidence:
        return action, False
    if opts.needs_corroboration:
        # Only what the hazard choice backs counts: a sentinel on its own is already answered at
        # its own band, and is not a reason to hold on top of that.
        findings = [f for f in findings if not f.uncorroborated]
        probabilities = {cid: p for cid, p in probabilities.items() if _num((choice or {}).get(cid)) >= min_choice}
    near_miss = any(
        probability >= (policy.categories[cid].threshold(surface).get("flag", 1.0) * 0.5)
        for cid, probability in probabilities.items()
        if cid in policy.categories
    )
    if not findings and not near_miss:
        return action, False
    if rank(action) >= rank("review"):
        return action, False
    return "review", True


def _route(policy: Policy, findings: list[Finding], action: Action) -> Route:
    """How the deployment should handle the content, given the decision and the hazard.

    The action says whether the content goes out; the route says what to do about it.
    """
    # Only a finding that still counts sets the handling. One a rule capped to allow is a record, and
    # a sentinel the hazard choice does not back, still at flag, must not replace an ordinary answer
    # with a crisis message.
    hazard_route = next(
        (
            policy.categories[f.category].route
            for f in findings
            if rank(f.action) >= rank("flag")
            and not (f.uncorroborated and f.action == "flag")
            and policy.categories[f.category].route in _CATEGORY_ROUTES
        ),
        None,
    )
    if hazard_route == "crisis_support" and action != "allow":
        return "crisis_support"
    if action == "block":
        return "safe_response"
    if action == "allow":
        return "deliver"
    if hazard_route in ("redact", "guide"):
        return hazard_route  # type: ignore[return-value]
    return "human_review" if action == "review" else "deliver"


def _severity(signals: Mapping[str, Any], findings: list[Finding]) -> float:
    reported = signals.get("severity")
    if isinstance(reported, (int, float)):
        return float(reported)
    return max((f.severity for f in findings), default=0.0)


def with_floor(policy: Policy, verdict: Verdict, floor: Action, *, note: str) -> Verdict:
    """Raise a verdict to at least ``floor``, recomputing the route to match.

    Used to carry risk forward: a session that has already shown an escalation pattern should not
    have its next turn judged as if the conversation had just started.
    """
    action = stronger(verdict.action, floor)
    if action == verdict.action:
        return verdict
    return replace(
        verdict,
        action=action,
        route=_route(policy, list(verdict.findings), action),
        applied_rules=verdict.applied_rules + (note,),
    )


def now_ms() -> float:
    return time.perf_counter() * 1000.0
