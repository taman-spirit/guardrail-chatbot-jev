"""Question construction and policy loading."""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from guardrail_chatbot_jev import Guard, Policy, RecordedTransport, build_questions
from guardrail_chatbot_jev.questions import HAZARD, NONE_LABEL

ROOT = Path(__file__).resolve().parents[2]


def test_every_surface_builds_a_valid_question_set(policy: Policy) -> None:
    for surface in ("input", "output", "conversation"):
        questions = build_questions(policy, surface)  # type: ignore[arg-type]
        assert HAZARD in questions
        criteria = questions[HAZARD]["criteria"]
        assert NONE_LABEL in criteria
        assert len(criteria) <= 256, "choice questions take at most 255 labels"
        for name, question in questions.items():
            assert question["type"] in ("noul", "choice", "score"), name
            assert question["instructions"], name
            if question["type"] == "score":
                assert 2 <= len(question["criteria"]) <= 10, name
            if question["type"] == "choice":
                assert question["criteria"], name


def test_hazard_labels_match_the_surface(policy: Policy) -> None:
    criteria = build_questions(policy, "input")["criteria" if False else HAZARD]["criteria"]
    assert "mis" not in criteria, "output-only category leaked into the input question"
    assert "pij" in criteria
    assert "ipv" not in criteria


def test_groundedness_appears_only_with_context(policy: Policy) -> None:
    assert "groundedness" not in build_questions(policy, "output")
    assert "groundedness" in build_questions(policy, "output", has_context=True)


def test_disabled_categories_are_left_out(policy: Policy) -> None:
    assert "scp" not in build_questions(policy, "input")[HAZARD]["criteria"]


def test_policy_copies_stay_in_sync() -> None:
    packs = sorted((ROOT / "policies").glob("*.json"))
    assert {p.name for p in packs} >= {"standard-v1.json", "vietnam-compliance-v1.json"}
    for pack in packs:
        canonical = json.loads(pack.read_text("utf-8"))
        for copy in (
            ROOT / "python" / "src" / "guardrail_chatbot_jev" / "policies" / pack.name,
            ROOT / "ts" / "src" / "policies" / pack.name,
            ROOT / "go" / "policies" / pack.name,
        ):
            if copy.parts[-3] == "go" and not (ROOT / "go").exists():
                continue  # the Go module lives on its own branches
            assert json.loads(copy.read_text("utf-8")) == canonical, f"{copy} drifted; run scripts/sync-policies.sh"


def test_bad_thresholds_are_rejected() -> None:
    with pytest.raises(ValueError, match="not ordered"):
        Policy(
            {
                "id": "broken",
                "categories": {"x": {"thresholds": {"default": {"block": 0.1, "review": 0.5, "flag": 0.2}}}},
            }
        )


def test_guard_sends_one_request_per_check(policy: Policy) -> None:
    transport = RecordedTransport({"hazard": {"type": "choice", "choice": "none", "confidence": 0.95, "probabilities": {"none": 0.95}}})
    guard = Guard(policy, transport=transport)
    verdict = guard.check_input("hello there")
    assert verdict.action == "allow"
    assert len(transport.calls) == 1
    state, questions = transport.calls[0]
    assert state["user_message"] == "hello there"
    assert HAZARD in questions


def test_preview_needs_no_api_key(policy: Policy) -> None:
    guard = Guard(policy)
    preview = guard.preview("input", {"user_message": "hi"})
    assert preview["questions"][HAZARD]["type"] == "choice"
