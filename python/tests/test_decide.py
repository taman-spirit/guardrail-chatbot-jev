"""The decision engine, exercised against recorded Jev answers."""

from __future__ import annotations

import pytest
from conftest import answers, choice, hazard, noul, score

from guardrail_chatbot_jev import Policy
from guardrail_chatbot_jev.decide import decide, error_verdict
from guardrail_chatbot_jev.types import GuardrailError


def test_clean_input_is_allowed(policy: Policy) -> None:
    verdict = decide(policy, "input", answers())
    assert verdict.action == "allow"
    assert verdict.allowed
    assert verdict.findings == ()
    assert verdict.route == "deliver"


def test_sentinel_overrides_a_diluted_choice(policy: Policy) -> None:
    """A choice spreads its mass across 18 labels; the sentinel is the one that must not miss."""
    verdict = decide(
        policy,
        "input",
        answers(hazard=hazard({"cse": 0.09}), s_cse=noul(0.82)),
    )
    assert verdict.action == "block"
    assert verdict.top is not None
    assert verdict.top.category == "cse"
    assert verdict.top.probability == pytest.approx(0.82)


def test_never_below_forces_block_on_a_weak_signal(policy: Policy) -> None:
    """cse is configured never to resolve below block once it fires at all."""
    verdict = decide(policy, "input", answers(hazard=hazard({"cse": 0.04})))
    assert verdict.action == "block"
    assert "never below block" in verdict.top.notes


def test_academic_intent_softens_but_not_for_weapons(policy: Policy) -> None:
    weapons = answers(
        hazard=hazard({"iwp": 0.5}),
        s_iwp=noul(0.5),
        intent=choice("academic"),
        actionability=score(2.0),
    )
    assert decide(policy, "input", weapons).action == "block"

    fraud = answers(hazard=hazard({"ncr": 0.55}), intent=choice("academic"), actionability=score(2.0))
    verdict = decide(policy, "input", fraud)
    assert verdict.action == "review"
    assert "academic-intent-softens" in verdict.applied_rules


def test_actionability_moves_the_verdict_both_ways(policy: Policy) -> None:
    base = {"hazard": hazard({"vcr": 0.3}), "severity": score(2.0), "intent": choice("seeking_information")}

    talk = decide(policy, "input", {**base, "actionability": score(0.5)})
    assert talk.action == "flag"
    assert talk.route == "deliver"
    assert "low-actionability-softens" in talk.applied_rules

    recipe = decide(policy, "input", {**base, "actionability": score(3.0)})
    assert recipe.action == "block"
    assert "high-actionability-hardens" in recipe.applied_rules


def test_evasion_raises_a_prompt_injection_finding(policy: Policy) -> None:
    verdict = decide(policy, "input", answers(hazard=hazard({"ncr": 0.2}), intent=choice("evasion")))
    assert "pij" in verdict.categories
    assert "evasion-hardens" in verdict.applied_rules


def test_self_harm_routes_to_crisis_support(policy: Policy) -> None:
    verdict = decide(policy, "input", answers(hazard=hazard({"ssh": 0.2}), s_ssh=noul(0.66)))
    assert verdict.action == "block"
    assert verdict.route == "crisis_support"
    assert not verdict.deliverable


def test_pii_in_a_reply_is_redacted_not_blocked(policy: Policy) -> None:
    verdict = decide(
        policy,
        "output",
        answers(hazard=hazard({"prv": 0.3}), s_prv=noul(0.35), refusal=noul(0.02)),
    )
    assert verdict.action == "review"
    assert verdict.route == "redact"
    assert verdict.deliverable


def test_a_refusal_is_not_punished_for_naming_the_hazard(policy: Policy) -> None:
    refusing = answers(hazard=hazard({"vcr": 0.62}), refusal=noul(0.95), actionability=score(0.0))
    verdict = decide(policy, "output", refusing)
    assert verdict.action == "flag"
    assert "refusal-caps-output-risk" in verdict.applied_rules


def test_a_refusal_does_not_excuse_a_leaked_system_prompt(policy: Policy) -> None:
    verdict = decide(
        policy,
        "output",
        answers(hazard=hazard({"sid": 0.5}), s_sid=noul(0.7), refusal=noul(0.95)),
    )
    assert verdict.action == "block"


def test_ungrounded_reply_raises_misinformation(policy: Policy) -> None:
    verdict = decide(policy, "output", answers(groundedness=score(0.5), refusal=noul(0.0)))
    assert verdict.action == "flag"
    assert "mis" in verdict.categories
    assert "ungrounded-output" in verdict.applied_rules


def test_grounded_reply_stays_clean(policy: Policy) -> None:
    verdict = decide(policy, "output", answers(groundedness=score(3.0), refusal=noul(0.0)))
    assert verdict.action == "allow"


def test_crescendo_escalates_a_conversation(policy: Policy) -> None:
    verdict = decide(
        policy,
        "conversation",
        answers(hazard=hazard({"ncr": 0.32}), crescendo=noul(0.8), escalation=score(3.4)),
    )
    assert verdict.action in ("review", "block")
    assert "pij" in verdict.categories
    assert {"crescendo-hardens", "escalation-review"} <= set(verdict.applied_rules)


def test_low_confidence_escalates_to_review(policy: Policy) -> None:
    verdict = decide(policy, "input", answers(hazard=hazard({"ncr": 0.32}, confidence=0.2)))
    assert verdict.action == "review"
    assert "confidence-gate" in verdict.applied_rules
    assert not verdict.allowed


def test_low_confidence_with_nothing_nearby_stays_allowed(policy: Policy) -> None:
    verdict = decide(policy, "input", answers(hazard=hazard({"ncr": 0.01}, confidence=0.2)))
    assert verdict.action == "allow"


def test_low_confidence_never_downgrades_a_block(policy: Policy) -> None:
    verdict = decide(
        policy, "input", answers(hazard=hazard({"vcr": 0.9}, confidence=0.1), actionability=score(2.0))
    )
    assert verdict.action == "block"
    assert verdict.route == "safe_response"


def test_conversation_surface_drops_input_only_categories(policy: Policy) -> None:
    """mis is an output-only category and must not be scored on a user message."""
    verdict = decide(policy, "input", answers(hazard=hazard({"mis": 0.9})))
    assert "mis" not in verdict.categories


def test_error_verdict_fails_closed_on_output(policy: Policy) -> None:
    """The output check is the last line, so an unreachable Jev holds the reply."""
    verdict = error_verdict(policy, "output", GuardrailError("boom"))
    assert verdict.action == "review"
    assert verdict.route == "human_review"
    assert not verdict.deliverable, "a degraded verdict must not read as permission to send"
    assert verdict.degraded
    assert not verdict.allowed
    assert "boom" in (verdict.error or "")


def test_verdict_serialises(policy: Policy) -> None:
    payload = decide(
        policy, "input", answers(hazard=hazard({"hte": 0.6}), actionability=score(2.0))
    ).as_dict()
    assert payload["action"] == "block"
    assert payload["findings"][0]["category"] == "hte"
    assert payload["policy_id"].startswith("standard-v1@")


def test_a_softening_rule_never_erases_a_finding(policy: Policy) -> None:
    """Softening lowers the response; the record of what fired stays."""
    verdict = decide(
        policy,
        "input",
        answers(hazard=hazard({"ncr": 0.16}), actionability=score(0.0), intent=choice("academic")),
    )
    assert verdict.action == "flag"
    assert "ncr" in verdict.categories


def test_action_and_route_are_separate_axes(policy: Policy) -> None:
    """A specialized-advice reply is delivered, but steered."""
    verdict = decide(
        policy,
        "output",
        answers(hazard=hazard({"spc": 0.55}), refusal=noul(0.0), actionability=score(2.0)),
    )
    assert verdict.action == "review"
    assert verdict.route == "guide"
    assert verdict.deliverable


def test_a_sentinel_probability_is_not_read_as_doubt(policy: Policy) -> None:
    """A noul reports belief, not uncertainty.

    Treating its distance from 0.5 as confidence double-counts the probability: every mid-range
    sentinel would escalate to review no matter where the threshold sat, which makes the
    threshold meaningless for exactly the categories that most need one.
    """
    verdict = decide(
        policy,
        "output",
        answers(hazard=hazard({"prv": 0.05}, confidence=0.9), s_prv=noul(0.2), refusal=noul(0.0)),
    )
    assert verdict.action == "flag"
    assert "confidence-gate" not in verdict.applied_rules
    assert verdict.top is not None
    assert verdict.top.source == "sentinel"
    assert verdict.top.confidence == pytest.approx(0.9)


def test_a_sentinel_still_inherits_a_shaky_request(policy: Policy) -> None:
    verdict = decide(
        policy,
        "output",
        answers(hazard=hazard({"prv": 0.05}, confidence=0.2), s_prv=noul(0.2), refusal=noul(0.0)),
    )
    assert verdict.action == "review"
    assert "confidence-gate" in verdict.applied_rules
