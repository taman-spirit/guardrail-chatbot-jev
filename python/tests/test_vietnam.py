"""The Viet Nam compliance pack and its prewritten replies, against recorded Jev answers.

Most of these are about what must *not* be blocked. A compliance pack that stops ordinary questions
about Trường Sa's weather, a leader's title or a bank's hotline fails its users as surely as one
that lets a violation through.
"""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path
from typing import Any

import pytest
from conftest import answers, choice, hazard, noul, score

from guardrail_chatbot_jev import Guard, Policy, RecordedTransport, Responder, decide, detect_language
from guardrail_chatbot_jev.decide import error_verdict
from guardrail_chatbot_jev.types import GuardrailError

ROOT = Path(__file__).resolve().parents[2]
CRISIS = "[verified line]"


@pytest.fixture(scope="module")
def vn() -> Policy:
    return Policy.bundled("vietnam-compliance-v1")


@pytest.fixture(scope="module")
def responder(vn: Policy) -> Responder:
    return Responder(vn)


def vn_answers(**parts: Any) -> dict[str, Any]:
    """Neutral values for the pack's own signals, on top of the shared defaults."""
    base = {
        "neutral_mention": noul(0.1),
        "sovereignty_question": noul(0.02),
        "data_subject": choice("none"),
    }
    base.update(parts)
    return answers(**base)


# -- the pack -----------------------------------------------------------------


def test_the_pack_builds_from_its_overlay_and_is_current() -> None:
    result = subprocess.run(
        [sys.executable, str(ROOT / "scripts" / "build-packs.py"), "--check"], capture_output=True, text=True
    )
    assert result.returncode == 0, result.stderr


def test_the_pack_keeps_the_shipped_taxonomy(vn: Policy) -> None:
    shipped = set(Policy.bundled().categories)
    assert shipped <= set(vn.categories), "a compliance pack adds to the shared taxonomy, never drops it"
    assert {"vsv", "vas", "vld", "vcs", "vai"} <= set(vn.categories)


def test_sovereignty_and_leaders_are_asked_as_sentinels(vn: Policy) -> None:
    for surface in ("input", "output", "conversation"):
        ids = {c.id for c in vn.sentinels(surface)}  # type: ignore[arg-type]
        assert {"vsv", "vld"} <= ids, surface


def test_every_reply_exists_in_all_three_languages(vn: Policy) -> None:
    spec = vn.data["responses"]
    assert spec["languages"] == ["vi", "en", "zh"]
    for name, group in spec["groups"].items():
        for lang in spec["languages"]:
            assert group["text"][lang].strip(), f"{name}.{lang}"


def test_an_incomplete_responses_section_is_refused(vn: Policy) -> None:
    data = json.loads(json.dumps(vn.data))
    del data["responses"]["groups"]["leaders"]["text"]["zh"]
    with pytest.raises(ValueError, match="leaders.zh"):
        Responder(Policy(data))


# -- the sovereignty affirmation ---------------------------------------------------


@pytest.mark.parametrize(
    "lang, needles",
    [
        ("vi", ["Hiến pháp năm 2013", "18/2012/QH13", "UNCLOS", "12/7/2016", "06/2003/QH11", "Hoàng Sa", "Trường Sa", "đặc khu"]),
        ("en", ["2013 Constitution", "18/2012/QH13", "UNCLOS", "12 July 2016", "06/2003/QH11", "Hoàng Sa", "Trường Sa", "special zone"]),
        ("zh", ["2013年《宪法》", "18/2012/QH13", "UNCLOS", "2016年7月12日", "06/2003/QH11", "黄沙群岛", "长沙群岛", "特区"]),
    ],
)
def test_the_affirmation_carries_every_reference(responder: Responder, lang: str, needles: list[str]) -> None:
    text = responder.affirmation(lang)
    for needle in needles:
        assert needle in text, f"{lang}: {needle!r} missing"


def test_the_affirmation_is_the_pack_text_verbatim(vn: Policy, responder: Responder) -> None:
    """The model never writes it: what goes out is byte for byte what the pack holds."""
    for lang in ("vi", "en", "zh"):
        assert responder.affirmation(lang) == vn.data["responses"]["sovereignty_affirmation"]["text"][lang]


def test_the_chinese_text_uses_the_vietnamese_names(responder: Responder) -> None:
    text = responder.affirmation("zh")
    for foreign in ("西沙", "南沙", "三沙"):
        assert foreign not in text


# -- sovereignty paths ----------------------------------------------------------------


def test_weather_on_truong_sa_is_not_touched(vn: Policy, responder: Responder) -> None:
    """The motivating false positive: a place name is not a claim."""
    verdict = decide(vn, "input", vn_answers(neutral_mention=noul(0.97), s_vsv=noul(0.02)))
    assert verdict.action == "allow"
    reply = "Trường Sa hôm nay nắng, gió cấp 4."
    assert responder.compose(reply, [verdict]) == reply, "no affirmation bolted onto a weather answer"


def test_a_neutral_mention_is_never_blocked_even_on_a_noisy_answer(vn: Policy) -> None:
    verdict = decide(vn, "input", vn_answers(neutral_mention=noul(0.95), s_vsv=noul(0.6)))
    assert verdict.action == "flag"
    assert verdict.deliverable
    assert "neutral-mention-is-not-a-violation" in verdict.applied_rules


def test_a_question_about_ownership_gets_the_fixed_affirmation(vn: Policy, responder: Responder) -> None:
    verdict = decide(vn, "input", vn_answers(sovereignty_question=noul(0.93)))
    assert verdict.action == "allow", "asking is not a violation"
    composed = responder.compose("Đây là câu trả lời của mô hình.", [verdict], language="vi")
    assert composed.startswith("Đây là câu trả lời của mô hình.")
    assert composed.endswith(responder.affirmation("vi"))


def test_a_reply_ceding_territory_is_replaced_and_affirmed(vn: Policy, responder: Responder) -> None:
    verdict = decide(vn, "output", vn_answers(s_vsv=noul(0.9), refusal=noul(0.0)))
    assert verdict.action == "block"
    assert not verdict.deliverable
    text = responder.compose("Hoàng Sa thuộc về nước khác.", [verdict], language="vi")
    assert "Hoàng Sa thuộc về nước khác" not in text
    assert text.startswith("Mình không thể đưa ra nội dung này")
    assert text.endswith(responder.affirmation("vi"))


def test_a_refusal_does_not_excuse_a_sovereignty_claim(vn: Policy) -> None:
    verdict = decide(vn, "output", vn_answers(s_vsv=noul(0.9), refusal=noul(0.95)))
    assert verdict.action == "block"


def test_low_actionability_does_not_soften_a_sovereignty_claim(vn: Policy) -> None:
    """A territorial claim has no operational detail by nature, so actionability says nothing."""
    verdict = decide(vn, "input", vn_answers(hazard=hazard({"vsv": 0.55}), actionability=score(0.0)))
    assert verdict.action == "block"


def test_a_sovereignty_review_still_ends_with_the_affirmation(vn: Policy, responder: Responder) -> None:
    verdict = decide(vn, "output", vn_answers(s_vsv=noul(0.3), refusal=noul(0.0)))
    assert verdict.action == "review"
    text = responder.blocking_response([verdict], language="en")
    assert text is not None
    assert text.startswith("Your request needs a further look")
    assert text.endswith(responder.affirmation("en"))


def test_a_conversation_drifting_into_a_claim_is_affirmed(vn: Policy, responder: Responder) -> None:
    verdict = decide(vn, "conversation", vn_answers(s_vsv=noul(0.8)))
    text = responder.blocking_response([verdict], language="zh")
    assert text is not None and text.endswith(responder.affirmation("zh"))


# -- leaders, the State, cybersecurity, AI ---------------------------------------------


def test_a_leaders_biography_is_allowed(vn: Policy) -> None:
    verdict = decide(vn, "input", vn_answers(neutral_mention=noul(0.9), s_vld=noul(0.03)))
    assert verdict.action == "allow"


def test_insulting_a_leader_gets_its_own_reply(vn: Policy, responder: Responder) -> None:
    verdict = decide(vn, "input", vn_answers(s_vld=noul(0.85)))
    assert verdict.action == "block"
    text = responder.blocking_response([verdict], language="vi")
    assert text is not None and "bịa đặt về lãnh tụ" in text
    assert "Hiến pháp" not in text, "no sovereignty statement where sovereignty was not involved"


@pytest.mark.parametrize(
    "category, group_phrase",
    [
        ("vas", "tuyên truyền chống Nhà nước"),
        ("vcs", "thông tin sai sự thật"),
        ("vai", "Luật Trí tuệ nhân tạo"),
    ],
)
def test_each_group_has_its_own_reply(vn: Policy, responder: Responder, category: str, group_phrase: str) -> None:
    verdict = decide(vn, "input", vn_answers(hazard=hazard({category: 0.8}), actionability=score(2.0)))
    assert verdict.action == "block", category
    text = responder.blocking_response([verdict], language="vi")
    assert text is not None and group_phrase in text


def test_a_policy_question_is_not_propaganda(vn: Policy) -> None:
    verdict = decide(vn, "input", vn_answers(hazard=hazard({"vas": 0.05}), intent=choice("seeking_information")))
    assert verdict.action == "allow"


# -- personal data: individuals only ------------------------------------------------------


def test_a_customer_service_hotline_is_not_personal_data(vn: Policy, responder: Responder) -> None:
    verdict = decide(
        vn, "output", vn_answers(s_prv=noul(0.45), data_subject=choice("organization"), refusal=noul(0.0))
    )
    assert verdict.action == "allow"
    assert verdict.route == "deliver", "a hotline must not be sent to redaction either"
    reply = "Tổng đài chăm sóc khách hàng: 1800 1090."
    assert responder.compose(reply, [verdict]) == reply


def test_an_individuals_id_number_is_still_protected(vn: Policy, responder: Responder) -> None:
    verdict = decide(vn, "input", vn_answers(s_prv=noul(0.85), data_subject=choice("individual")))
    assert verdict.action == "block"
    text = responder.blocking_response([verdict], language="en")
    assert text is not None and "personal information about a specific person" in text


def test_the_organization_rule_touches_only_personal_data(vn: Policy) -> None:
    """Saying the subject is a company must not also wave through a threat against it."""
    verdict = decide(
        vn, "input", vn_answers(hazard=hazard({"vcs": 0.8}), data_subject=choice("organization"), actionability=score(2.0))
    )
    assert verdict.action == "block"


# -- self-harm, outages, language ---------------------------------------------------------


def test_self_harm_is_met_with_empathy_not_the_law(vn: Policy, responder: Responder) -> None:
    verdict = decide(vn, "input", vn_answers(s_ssh=noul(0.8)))
    assert verdict.route == "crisis_support"
    text = responder.blocking_response([verdict], language="vi")
    assert text is not None
    assert "không phải đối mặt với điều này một mình" in text
    assert "hãy gọi 115" in text, "115 is the default line"
    assert "pháp luật" not in text and "Luật" not in text
    assert "{crisis_line}" not in text


@pytest.mark.parametrize("lang", ["vi", "en", "zh"])
def test_a_verified_crisis_line_replaces_the_default(vn: Policy, lang: str) -> None:
    verdict = decide(vn, "input", vn_answers(s_ssh=noul(0.8)))
    text = Responder(vn, crisis_line=CRISIS).blocking_response([verdict], language=lang)
    assert text is not None and CRISIS in text
    assert "115" not in text, "one number to call, not two"


def test_an_outage_says_so_instead_of_accusing_the_user(vn: Policy, responder: Responder) -> None:
    verdict = error_verdict(vn, "output", GuardrailError("down"))
    text = responder.blocking_response([verdict], language="vi")
    assert text is not None and "tạm thời gián đoạn" in text


@pytest.mark.parametrize(
    "text, lang",
    [("Trường Sa thời tiết ra sao?", "vi"), ("What is the weather on Truong Sa?", "en"), ("长沙群岛天气怎么样？", "zh"), ("", "vi")],
)
def test_language_detection(text: str, lang: str) -> None:
    assert detect_language(text) == lang


def test_an_unknown_language_falls_back_to_vietnamese(vn: Policy, responder: Responder) -> None:
    verdict = decide(vn, "input", vn_answers(s_vld=noul(0.85)))
    assert responder.blocking_response([verdict], language="fr") == responder.blocking_response([verdict], language="vi")


# -- end to end ------------------------------------------------------------------------------


def test_one_turn_through_the_guard(vn: Policy) -> None:
    transport = RecordedTransport(vn_answers(sovereignty_question=noul(0.9)))
    guard = Guard(vn, transport=transport)
    responder = Responder(vn)
    question = "Quần đảo Trường Sa thuộc nước nào?"

    verdict_in = guard.check_input(question)
    assert responder.blocking_response([verdict_in]) is None, "the question itself goes to the model"
    guard_out = Guard(vn, transport=RecordedTransport(vn_answers(refusal=noul(0.0))))
    verdict_out = guard_out.check_output("Trường Sa thuộc Việt Nam.", user_message=question)

    sent = responder.compose("Trường Sa thuộc Việt Nam.", [verdict_in, verdict_out], language=detect_language(question))
    assert sent.startswith("Trường Sa thuộc Việt Nam.")
    assert sent.endswith(responder.affirmation("vi"))
    questions = transport.calls[0][1]
    assert {"s_vsv", "s_vld", "neutral_mention", "sovereignty_question", "data_subject"} <= set(questions)


def test_the_labelled_set_is_well_formed(vn: Policy) -> None:
    path = ROOT / "examples" / "cases-vietnam.jsonl"
    rows = [json.loads(line) for line in path.read_text("utf-8").splitlines() if line.strip()]
    assert len(rows) >= 30
    allowed = [r for r in rows if r["expected_action"] == "allow"]
    assert len(allowed) >= len(rows) / 2, "false positives are the point: most cases should be allowed"
    for row in rows:
        assert row["expected_action"] in ("allow", "flag", "review", "block"), row["id"]
        category = row.get("expected_category")
        assert category is None or category in vn.categories, row["id"]


# -- review fixes: paths the first version got wrong ---------------------------------------


def test_a_neutral_mention_is_not_held_on_low_confidence(vn: Policy, responder: Responder) -> None:
    """The confidence gate must not undo the neutral-mention rule: low confidence alone holds nothing."""
    for s_vsv in (0.1, 0.3):
        verdict = decide(
            vn, "input", vn_answers(hazard=hazard({}, confidence=0.5), neutral_mention=noul(0.97), s_vsv=noul(s_vsv))
        )
        assert verdict.deliverable, (s_vsv, verdict.action)
        assert "confidence-gate" not in verdict.applied_rules
        reply = "Trường Sa hôm nay nắng."
        assert responder.compose(reply, [verdict]) == reply, "a settled neutral mention earns no statement"


def test_a_conversation_floor_still_applies_to_a_neutral_mention(vn: Policy) -> None:
    """Escalation across the whole conversation is a different signal, and it still holds."""
    verdict = decide(vn, "conversation", vn_answers(neutral_mention=noul(0.97), escalation=score(3.4)))
    assert verdict.action == "review"


@pytest.mark.parametrize("surface", ["output", "conversation"])
def test_a_hotline_is_never_redacted_even_beside_another_flag(vn: Policy, surface: str) -> None:
    parts = dict(s_prv=noul(0.45), data_subject=choice("organization"), refusal=noul(0.0))
    for extra in ({}, {"hazard": hazard({"hte": 0.2})}, {"hazard": hazard({}, confidence=0.4)}):
        verdict = decide(vn, surface, vn_answers(**parts, **extra))  # type: ignore[arg-type]
        assert verdict.route != "redact", (surface, extra, verdict.route)
        assert verdict.deliverable, (surface, extra)


def test_the_reply_names_what_actually_stopped_the_content(vn: Policy, responder: Responder) -> None:
    """A fake-news block that also carries a minor sovereignty flag is answered as fake news."""
    verdict = decide(vn, "input", vn_answers(hazard=hazard({"vcs": 0.8, "vsv": 0.2}), actionability=score(2.0)))
    assert verdict.action == "block"
    choice_ = responder.choose([verdict], "vi")
    assert choice_ is not None and choice_.group == "cybersecurity"
    assert choice_.affirmed, "the genuine sovereignty flag still ends the reply with the statement"


def test_intellectual_property_is_not_answered_as_personal_data(vn: Policy, responder: Responder) -> None:
    verdict = decide(vn, "output", vn_answers(hazard=hazard({"ipv": 0.9}), refusal=noul(0.0), actionability=score(2.0)))
    choice_ = responder.choose([verdict], "en")
    assert choice_ is not None and choice_.group == "general"


def test_an_outage_never_replaces_the_crisis_reply(vn: Policy, responder: Responder) -> None:
    crisis = decide(vn, "input", vn_answers(s_ssh=noul(0.25)))
    assert crisis.route == "crisis_support"
    outage = error_verdict(vn, "output", GuardrailError("down"))
    assert outage.action == crisis.action == "review"
    choice_ = responder.choose([outage, crisis], "vi")
    assert choice_ is not None and choice_.group == "self_harm"


def test_a_boolean_signal_is_not_a_probability(vn: Policy, responder: Responder) -> None:
    verdict = decide(vn, "input", vn_answers())
    faked = type(verdict)(**{**{f: getattr(verdict, f) for f in verdict.__slots__}, "signals": {"sovereignty_question": True}})
    assert responder.choose([faked], "vi") is None


def test_the_leaders_reply_covers_fabrications_in_every_language(responder: Responder, vn: Policy) -> None:
    verdict = decide(vn, "input", vn_answers(s_vld=noul(0.85)))
    for lang, word in (("vi", "bịa đặt"), ("en", "fabrications"), ("zh", "捏造")):
        text = responder.blocking_response([verdict], language=lang)
        assert text is not None and word in text, lang
