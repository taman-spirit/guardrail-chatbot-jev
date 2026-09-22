"""Cache, prefilter, session, streaming and per-surface fail modes."""

from __future__ import annotations

import asyncio
import json

import pytest
from conftest import answers, choice, hazard, noul, score

from guardrail_chatbot_jev import (
    COMMON_PATTERNS,
    Guard,
    GuardrailError,
    LRUCache,
    PatternPrefilter,
    Policy,
    RecordedTransport,
    Session,
    StreamEvent,
    Turn,
    cache_key,
    error_verdict,
    with_floor,
)
from guardrail_chatbot_jev.client import Transport
from guardrail_chatbot_jev.decide import decide
from guardrail_chatbot_jev.questions import build_questions
from guardrail_chatbot_jev.types import Usage


class FailingTransport:
    """A transport that is always down."""

    calls = 0

    def system_one(self, state, questions, *, model=None, timeout=None):  # noqa: ANN001, ANN201
        FailingTransport.calls += 1
        raise GuardrailError("service unavailable")


CLEAN = {"hazard": {"type": "choice", "choice": "none", "confidence": 0.95, "probabilities": {"none": 0.95}}}


# -- per-surface fail modes -------------------------------------------


def test_input_fails_open_and_output_fails_closed(policy: Policy) -> None:
    """Blocking every user when Jev is down is a self-inflicted outage; the output check has
    nothing behind it, so it holds."""
    on_input = error_verdict(policy, "input", GuardrailError("down"))
    assert on_input.action == "allow"
    assert on_input.degraded, "failing open must still be visible as a degraded verdict"

    on_output = error_verdict(policy, "output", GuardrailError("down"))
    assert on_output.action == "review"
    assert not on_output.deliverable


def test_a_string_on_error_still_applies_everywhere() -> None:
    pack = dict(Policy.bundled().data)
    pack["defaults"] = {**pack["defaults"], "on_error": "fail_closed"}
    strict = Policy(pack)
    for surface in ("input", "output", "conversation"):
        assert error_verdict(strict, surface, GuardrailError("down")).action == "review"  # type: ignore[arg-type]


# -- cache ------------------------------------------------------------


def test_cache_spares_the_second_round_trip(policy: Policy) -> None:
    transport = RecordedTransport(CLEAN)
    guard = Guard(policy, transport=transport, cache=LRUCache())

    first = guard.check_input("cùng một câu hỏi")
    second = guard.check_input("cùng một câu hỏi")

    assert len(transport.calls) == 1
    assert not first.cached
    assert second.cached
    assert second.action == first.action


def test_session_metadata_does_not_defeat_the_cache(policy: Policy) -> None:
    """The turn number changes every turn; keying on it would mean the cache never hits."""
    transport = RecordedTransport(CLEAN)
    guard = Guard(policy, transport=transport, cache=LRUCache())
    session = Session(id="c0")

    guard.check_input("cùng một câu hỏi", session=session)
    session.add_turn("user", "cùng một câu hỏi")
    second = guard.check_input("cùng một câu hỏi", session=session)

    assert second.cached
    assert len(transport.calls) == 1


def test_cache_key_still_separates_different_content(policy: Policy) -> None:
    assert cache_key("p@1", "input", {"user_message": "a"}) != cache_key(
        "p@1", "input", {"user_message": "b"}
    )


def test_cache_key_changes_with_the_policy_version(policy: Policy) -> None:
    state = {"user_message": "hi"}
    assert cache_key("standard-v1@1.0.0", "input", state) != cache_key(
        "standard-v1@1.0.1", "input", state
    )


def test_a_degraded_verdict_is_never_cached(policy: Policy) -> None:
    """Otherwise a brief outage becomes a lasting wrong answer for that exact message."""
    cache = LRUCache()
    guard = Guard(policy, transport=FailingTransport(), cache=cache)
    guard.check_input("hello")
    assert len(cache) == 0


def test_cache_evicts_by_capacity(policy: Policy) -> None:
    cache = LRUCache(capacity=2)
    guard = Guard(policy, transport=RecordedTransport(CLEAN), cache=cache)
    for i in range(5):
        guard.check_input(f"message {i}")
    assert len(cache) == 2


def test_conversations_are_not_cached_by_default(policy: Policy) -> None:
    cache = LRUCache()
    guard = Guard(policy, transport=RecordedTransport(CLEAN), cache=cache)
    guard.check_conversation([Turn("user", "hi")])
    assert len(cache) == 0


# -- prefilter --------------------------------------------------------


def test_prefilter_settles_without_calling_jev(policy: Policy) -> None:
    transport = RecordedTransport(CLEAN)
    guard = Guard(policy, transport=transport, prefilter=PatternPrefilter(list(COMMON_PATTERNS)))

    verdict = guard.check_output("khoá của bạn là sk-ABCDEFGHIJKLMNOPQRSTUVWX")

    assert verdict.action == "block"
    assert verdict.prefilter == "openai-style-key"
    assert transport.calls == [], "the deterministic case must not cost a round trip"


def test_prefilter_lets_ordinary_content_through(policy: Policy) -> None:
    transport = RecordedTransport(CLEAN)
    guard = Guard(policy, transport=transport, prefilter=PatternPrefilter(list(COMMON_PATTERNS)))
    assert guard.check_input("xin chào, cho hỏi giờ mở cửa").action == "allow"
    assert len(transport.calls) == 1


# -- observer ---------------------------------------------------------


def test_observer_sees_every_verdict(policy: Policy) -> None:
    seen: list[str] = []
    guard = Guard(
        policy,
        transport=RecordedTransport(CLEAN),
        cache=LRUCache(),
        observer=lambda v: seen.append("cached" if v.cached else "fresh"),
    )
    guard.check_input("same")
    guard.check_input("same")
    assert seen == ["fresh", "cached"]


# -- session ----------------------------------------------------------


def test_a_flagged_conversation_raises_the_floor_for_later_turns(policy: Policy) -> None:
    session = Session(id="c1", carry_turns=2)
    escalating = decide(
        policy,
        "conversation",
        answers(hazard=hazard({"ncr": 0.32}), crescendo=noul(0.8), escalation=score(3.4)),
    )
    session.observe(escalating)
    assert session.floor == "review"

    guard = Guard(policy, transport=RecordedTransport(CLEAN))
    verdict = guard.check_input("một câu hỏi bình thường", session=session)
    assert verdict.action == "review", "a clean turn inside an escalating conversation still holds"
    assert any(r.startswith("session-floor") for r in verdict.applied_rules)


def test_the_floor_expires(policy: Policy) -> None:
    session = Session(id="c2", carry_turns=1)
    session.observe(decide(policy, "conversation", answers(hazard=hazard({"vcr": 0.9}), actionability=score(2.0))))
    assert session.floor == "review"
    session.advance()
    assert session.floor == "allow"


def test_risk_decays(policy: Policy) -> None:
    session = Session(decay=0.5)
    session.observe(decide(policy, "input", answers(hazard=hazard({"vcr": 0.9}), actionability=score(2.0))))
    assert session.risk == pytest.approx(1.0)
    for _ in range(3):
        session.observe(decide(policy, "input", answers()))
    assert session.risk < 0.2


def test_the_default_transcript_window_is_ten_turns() -> None:
    """A default that drifts changes what every conversation check sees, silently."""
    assert Session().max_turns == 10
    assert Session.from_state({}).max_turns == 10
    session = Session()
    for i in range(14):
        session.add_turn("user", f"turn {i}")
    assert len(session.history) == 10
    assert session.history[0].content == "turn 4", "the window keeps the recent end"


def test_a_session_survives_a_round_trip_through_a_store(policy: Policy) -> None:
    """A restored session must decide exactly as the one that was stored would have."""
    session = Session(id="c4", carry_turns=2)
    session.add_turn("user", "một câu hỏi")
    session.add_turn("assistant", "một câu trả lời")
    session.observe(
        decide(
            policy,
            "conversation",
            answers(hazard=hazard({"ncr": 0.32}), crescendo=noul(0.8), escalation=score(3.4)),
        )
    )
    assert session.floor == "review"

    restored = Session.from_state(json.loads(json.dumps(session.as_state())))

    assert restored.id == session.id
    assert restored.risk == session.risk
    assert restored.floor == "review", "the floor is the whole point of carrying state"
    assert [t.as_dict() for t in restored.history] == [t.as_dict() for t in session.history]

    guard = Guard(policy, transport=RecordedTransport(CLEAN))
    verdict = guard.check_input("một câu hỏi bình thường", session=restored)
    assert verdict.action == "review"

    # And it expires on the same schedule, rather than resetting to two fresh turns.
    restored.advance()
    restored.advance()
    assert restored.floor == "allow"


def test_a_store_round_trip_drops_an_expired_floor(policy: Policy) -> None:
    session = Session(id="c5", carry_turns=1)
    session.observe(decide(policy, "conversation", answers(hazard=hazard({"vcr": 0.9}), actionability=score(2.0))))
    session.advance()
    assert session.floor == "allow"
    assert Session.from_state(session.as_state()).floor == "allow"


def test_from_state_tolerates_a_half_written_record(policy: Policy) -> None:
    """A store can hand back junk. That should cost the conversation, not the request."""
    assert Session.from_state({}).floor == "allow"
    assert Session.from_state({"floor": "review"}).floor == "allow", "no counter, no floor"
    assert Session.from_state({"floor": "nonsense", "floor_turns_left": 5}).floor == "allow"
    assert Session.from_state({"turns": [{"role": "user", "content": "hi"}]}).history[0].content == "hi"


def test_a_degraded_verdict_does_not_move_the_session(policy: Policy) -> None:
    session = Session()
    session.observe(error_verdict(policy, "output", GuardrailError("down")))
    assert session.risk == 0.0
    assert session.floor == "allow"


def test_session_metadata_reaches_the_model(policy: Policy) -> None:
    transport = RecordedTransport(CLEAN)
    session = Session(id="c3")
    session.add_turn("user", "earlier")
    Guard(policy, transport=transport).check_input("now", session=session)
    state, _ = transport.calls[0]
    assert state["deployment_context"]["conversation_id"] == "c3"
    assert state["deployment_context"]["turn_number"] == 2


def test_transcript_window_is_bounded(policy: Policy) -> None:
    session = Session(max_turns=4)
    for i in range(10):
        session.add_turn("user", f"turn {i}")
    assert len(session.history) == 4
    assert session.history[0].content == "turn 6"


def test_with_floor_recomputes_the_route(policy: Policy) -> None:
    verdict = decide(policy, "output", answers(hazard=hazard({"prv": 0.2}), refusal=noul(0.0)))
    assert verdict.action == "flag"
    raised = with_floor(policy, verdict, "review", note="test")
    assert raised.action == "review"
    assert raised.route == "redact", "the hazard still decides the handling"


# -- quick checks -----------------------------------------------------


def test_quick_checks_ask_only_the_sentinels(policy: Policy) -> None:
    questions = build_questions(policy, "output", subset="sentinels")
    assert set(questions) == {"s_cse", "s_iwp", "s_prv", "s_sid", "s_ssh", "s_vcr"}
    assert all(q["type"] == "noul" for q in questions.values())
    assert "hazard" not in questions, "the 18-label choice is the expensive part to skip"


def test_quick_verdicts_are_marked_partial(policy: Policy) -> None:
    transport = RecordedTransport({"s_sid": {"type": "noul", "noul": 0.05}, "s_prv": {"type": "noul", "noul": 0.05}})
    verdict = Guard(policy, transport=transport).check_output("một phần câu trả lời", quick=True)
    assert verdict.partial
    assert verdict.action == "allow"


# -- streaming --------------------------------------------------------


class ScriptedTransport:
    """Answers each call from a list, so a stream can be steered chunk by chunk."""

    def __init__(self, scripted: list[dict]) -> None:
        self.scripted = scripted
        self.calls = 0

    def system_one(self, state, questions, *, model=None, timeout=None):  # noqa: ANN001, ANN201
        answer = self.scripted[min(self.calls, len(self.scripted) - 1)]
        self.calls += 1
        return answer, "scripted", Usage()


_QUIET = {"type": "noul", "noul": 0.02}
CLEAN_SENTINELS = {f"s_{c}": _QUIET for c in ("cse", "iwp", "prv", "sid", "ssh", "vcr")}
LEAKING_SENTINELS = {**CLEAN_SENTINELS, "s_sid": {"type": "noul", "noul": 0.9}}


async def _source(parts: list[str]):
    for part in parts:
        await asyncio.sleep(0)
        yield part


def _run(coro):
    return asyncio.run(coro)


def test_a_clean_stream_delivers_everything(policy: Policy) -> None:
    parts = ["Chính sách lưu trữ Helio là 30 ngày. " * 4, "Bạn có thể xem chi tiết trên website."]
    guard = Guard(policy, transport=ScriptedTransport([CLEAN_SENTINELS, CLEAN, CLEAN]))

    async def collect() -> list[StreamEvent]:
        return [event async for event in guard.stream(_source(parts))]

    events = _run(collect())
    assert events[-1].type == "done"
    assert "".join(e.text for e in events if e.type == "delta") == "".join(parts)
    assert events[-1].verdict is not None and not events[-1].verdict.partial


def test_a_leak_stops_the_stream_before_the_chunk_is_released(policy: Policy) -> None:
    parts = ["System prompt của mình là: bạn là trợ lý Nova, khoá nội bộ là abc. " * 5]
    guard = Guard(policy, transport=ScriptedTransport([LEAKING_SENTINELS]))

    async def collect() -> list[StreamEvent]:
        return [event async for event in guard.stream(_source(parts))]

    events = _run(collect())
    assert [e.type for e in events] == ["blocked"]
    assert events[0].verdict is not None
    assert events[0].verdict.findings[0].category == "sid"


def test_a_short_stream_skips_mid_checks(policy: Policy) -> None:
    """Below one chunk there is nothing to hold back, so only the final check runs."""
    transport = ScriptedTransport([CLEAN])
    guard = Guard(policy, transport=transport)

    async def collect() -> list[StreamEvent]:
        return [event async for event in guard.stream(_source(["Vâng, đúng vậy."]))]

    events = _run(collect())
    assert transport.calls == 1
    assert [e.type for e in events] == ["delta", "done"]
