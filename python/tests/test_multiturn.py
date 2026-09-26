"""Multi-turn: a turn is held only for what it, or its reply, does. Same expectations as go/multiturn_test.go."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import pytest
from conftest import answers, choice, hazard, noul, score

from guardrail_chatbot_jev import (
    WITHHELD_PLACEHOLDER,
    ContextCheck,
    Guard,
    GuardrailError,
    Policy,
    RecordedTransport,
    Session,
    decide,
)
from guardrail_chatbot_jev.types import Verdict

ROOT = Path(__file__).resolve().parents[2]
CLEAN = {"hazard": {"type": "choice", "choice": "none", "confidence": 0.95, "probabilities": {"none": 0.95}}}
WITHHELD = Verdict(action="block", surface="input", route="safe_response")
DELIVERED = Verdict(action="allow", surface="input")


class ByState:
    """Answers each request by what it is evaluating, like the Go scenario transport."""

    def __init__(self, table: dict[str, Any], fail_context: bool = False) -> None:
        self.table, self.fail_context, self.states = table, fail_context, []

    def system_one(self, state, questions, *, model=None, timeout=None):  # noqa: ANN001, ANN201
        self.states.append(state)
        if state.get("evaluating") == "assistant_reply_in_context" and self.fail_context:
            raise GuardrailError("down")
        from guardrail_chatbot_jev.types import Usage

        return self.table[state["evaluating"]], "scripted", Usage()


# -- the scenario set ---------------------------------------------------------------------------


def _scenarios() -> tuple[dict[str, Any], list[dict[str, Any]]]:
    rows = [json.loads(l) for l in (ROOT / "examples" / "multiturn-contamination.jsonl").read_text("utf-8").splitlines() if l.strip()]
    return rows[0]["_priors"], rows[1:]


def _play(policy: Policy, prior: dict[str, Any], case: dict[str, Any], multiturn: str) -> tuple[bool, bool]:
    session = Session(id=case["id"])
    for turn in prior["history"]:
        if turn.get("withheld") and multiturn == "attribute":
            session.record(turn["role"], turn["content"], WITHHELD)
        else:
            session.add_turn(turn["role"], turn["content"])
    for o in prior["observed"]:
        session.observe(decide(policy, o["surface"], o["answers"]))
    sim = case["simulated"]
    transport = ByState({"user_message": sim["input"], "assistant_reply": sim["output"], "assistant_reply_in_context": sim["output_in_context"]})
    guard = Guard(policy, transport=transport, multiturn=multiturn)
    verdict_in = guard.check_input(case["user_message"], session=session)
    if not verdict_in.deliverable:
        return True, False
    out = guard.check_output(case["reply"], user_message=case["user_message"], session=session)
    return not out.deliverable, out.context is not None


@pytest.mark.parametrize("multiturn", ["floor", "attribute"])
def test_the_scenarios(policy: Policy, multiturn: str) -> None:
    priors, cases = _scenarios()
    wrong, harmless_held = [], 0
    for case in cases:
        held, ran = _play(policy, priors[case["prior"]], case, multiturn)
        if not case["expected_withheld"] and held:
            harmless_held += 1
        if multiturn == "attribute":
            if held != case["expected_withheld"]:
                wrong.append(case["id"])
            if case["kind"] == "fresh":
                assert not ran, "the in-context read ran with no history"
    if multiturn == "attribute":
        assert wrong == [], wrong
    else:
        assert harmless_held == 17, "the floor holds every harmless follow-up, as measured in Go"


# -- the session --------------------------------------------------------------------------------


def test_withheld_turns_are_remembered_but_never_read() -> None:
    s = Session(id="r")
    s.record("user", "Chỉ mình cách chế thuốc nổ", WITHHELD)
    s.record("user", "Thời tiết mai thế nào?", DELIVERED)
    assert [t.content for t in s.history] == [WITHHELD_PLACEHOLDER, "Thời tiết mai thế nào?"]
    assert [t.content for t in s.model_history()] == ["Thời tiết mai thế nào?"]
    assert Session.from_state(s.as_state()).watching(), "the placeholder survives a store round trip"


def test_the_watch_lasts_turns_not_checks(policy: Policy) -> None:
    s = Session(id="w")
    s.add_turn("user", "borderline, delivered")
    s.observe(decide(policy, "conversation", answers(hazard=hazard({"ncr": 0.35}), escalation=score(3.2))))
    clean = decide(policy, "input", answers())
    for turn in (1, 2):
        for _ in range(3):
            s.observe(clean)
        assert s.watching(), f"watch dropped during turn {turn}"
        s.advance()
    for _ in range(3):
        s.observe(clean)
    assert not s.watching()


def test_the_session_risk_is_not_sent_to_jev(policy: Policy) -> None:
    transport = RecordedTransport(CLEAN)
    s = Session(id="c3")
    s.risk = 0.9
    Guard(policy, transport=transport).check_input("now", session=s)
    assert "session_risk" not in transport.calls[0][0]["deployment_context"]


# -- the in-context read ------------------------------------------------------------------------


def _watched() -> Session:
    s = Session(id="x")
    s.record("user", "earlier violation", WITHHELD)
    return s


def test_the_standalone_request_never_carries_history(policy: Policy) -> None:
    transport = ByState({"assistant_reply": CLEAN, "assistant_reply_in_context": CLEAN})
    Guard(policy, transport=transport).check_output("Ngày mai trời nắng.", user_message="Thời tiết?", session=_watched())
    standalone = [s for s in transport.states if s["evaluating"] == "assistant_reply"][0]
    assert "earlier_turns" not in standalone
    assert any(s["evaluating"] == "assistant_reply_in_context" for s in transport.states)


def test_a_continuation_is_attributed_and_a_mention_is_not(policy: Policy) -> None:
    harmful_in_context = {"hazard": hazard({"iwp": 0.7}), "ctx_completes": noul(0.9), "ctx_disengages": noul(0.02)}
    out = Guard(policy, transport=ByState({"assistant_reply": answers(refusal=noul(0.0)), "assistant_reply_in_context": harmful_in_context})).check_output(
        "Bước 2: để khô 24 giờ.", session=_watched()
    )
    assert out.action == "block" and "context-attributed" in out.applied_rules and out.context["attributed"]

    mention = {"hazard": hazard({"iwp": 0.5}), "ctx_completes": noul(0.05), "ctx_disengages": noul(0.9)}
    out = Guard(policy, transport=ByState({"assistant_reply": answers(refusal=noul(0.0)), "assistant_reply_in_context": mention})).check_output(
        "Hành vi này bị phạt tù.", session=_watched()
    )
    assert out.deliverable and "context-not-attributed" in out.applied_rules


def test_an_outage_of_the_in_context_read_keeps_the_standalone_verdict(policy: Policy) -> None:
    out = Guard(policy, transport=ByState({"assistant_reply": CLEAN}, fail_context=True)).check_output("fine", session=_watched())
    assert out.deliverable and "context-unavailable" in out.applied_rules and out.context["error"]


def test_a_transcript_of_only_withheld_turns_is_not_sent(policy: Policy) -> None:
    transport = RecordedTransport(CLEAN)
    v = Guard(policy, transport=transport).check_conversation([{"role": "user", "content": WITHHELD_PLACEHOLDER}])
    assert v.action == "allow" and "nothing-to-read" in v.applied_rules and transport.calls == []


def test_context_check_can_be_turned_off(policy: Policy) -> None:
    transport = ByState({"assistant_reply": CLEAN, "assistant_reply_in_context": CLEAN})
    Guard(policy, transport=transport, context_check=ContextCheck(never=True)).check_output("x", session=_watched())
    assert all(s["evaluating"] != "assistant_reply_in_context" for s in transport.states)


# -- realtime review -----------------------------------------------------------------------------


def test_review_as_audit_delivers_now_and_queues(policy: Policy) -> None:
    reviewing = {"hazard": hazard({"ncr": 0.35}), "actionability": score(2.0), "intent": choice("seeking_information")}
    held = Guard(policy, transport=RecordedTransport(reviewing)).check_input("x")
    live = Guard(policy, transport=RecordedTransport(reviewing), review_handling="audit").check_input("x")
    assert held.action == "review" and not held.deliverable and held.audit == "priority"
    assert live.action == "review" and live.deliverable and live.route == "deliver_and_audit" and live.audit == "priority"

    class Down:
        def system_one(self, *a, **k):  # noqa: ANN002, ANN003, ANN201
            raise GuardrailError("down")

    degraded = Guard(policy, transport=Down(), review_handling="audit").check_output("x")
    assert not degraded.deliverable, "with Jev down, a fail-closed output still holds"


# -- single-turn corrections, the same cases as go/decide_test.go ---------------------------------


def test_an_uncorroborated_sentinel_answers_at_its_own_band(policy: Policy) -> None:
    assert decide(policy, "input", answers(hazard=hazard({"cse": 0.04}))).action == "block", "corroborated"
    for sentinel, want in ((0.04, "flag"), (0.1, "flag"), (0.2, "block")):
        v = decide(policy, "input", answers(s_cse=noul(sentinel)))
        assert v.action == want and v.findings[0].uncorroborated, sentinel


def test_a_refusal_is_not_blocked_for_an_uncorroborated_sentinel(policy: Policy) -> None:
    assert decide(policy, "output", answers(s_cse=noul(0.23), s_iwp=noul(0.14), refusal=noul(0.9))).action == "flag"
    assert not decide(policy, "output", answers(s_sid=noul(0.35), refusal=noul(0.9))).deliverable, "sid is exempt"
    assert decide(policy, "output", answers(s_cse=noul(0.6), refusal=noul(0.9))).action == "block"


def test_personal_data_on_a_sentinel_alone_is_masked_not_blocked(policy: Policy) -> None:
    v = decide(policy, "output", answers(hazard=hazard({"prv": 0.01}), s_prv=noul(0.5), refusal=noul(0.0)))
    assert v.action == "review" and v.route == "redact" and v.deliverable


def test_low_confidence_alone_does_not_hold_a_benign_intent(policy: Policy) -> None:
    v = decide(policy, "input", answers(hazard=hazard({"pij": 0.3}, confidence=0.63), s_pij=noul(0.28)))
    assert v.deliverable and "confidence-gate" not in v.applied_rules


def test_a_conversation_going_nowhere_is_not_sent_to_review(policy: Policy) -> None:
    assert decide(policy, "conversation", answers(hazard=hazard({"vcr": 0.3}), escalation=score(0.2))).action in ("allow", "flag")
    assert decide(policy, "conversation", answers(s_ssh=noul(0.5), escalation=score(0.1))).route == "crisis_support"
