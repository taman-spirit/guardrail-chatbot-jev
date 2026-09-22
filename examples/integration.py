"""One guarded turn, end to end.

Run it against a recorded transport (no API key, no network):

    python3 examples/integration.py
"""

from __future__ import annotations

import asyncio
from typing import AsyncIterator

from guardrail_chatbot_jev import (
    COMMON_PATTERNS,
    Guard,
    LRUCache,
    PatternPrefilter,
    RecordedTransport,
    Session,
    Verdict,
)

# One Guard per process. The cache, prefilter and observer are process-wide; the session is not.
guard = Guard(
    cache=LRUCache(capacity=8192, ttl=300),
    prefilter=PatternPrefilter(list(COMMON_PATTERNS)),
    observer=lambda v: emit_metric(v),
    timeout=2.0,
)


#: Strong references to fire-and-forget checks, so the event loop cannot drop them mid-flight.
_background: set[asyncio.Task[Verdict]] = set()


def emit_metric(verdict: Verdict) -> None:
    """Every verdict, including cached and degraded ones.

    Count `degraded` separately from `block`. A week with 5% degraded verdicts means the guardrail
    was only actually running 95% of the time, and that must not hide inside the block rate.
    """
    print(
        f"  [metric] {verdict.surface:<12} {verdict.action:<6} route={verdict.route:<14} "
        f"{verdict.latency_ms:6.1f}ms "
        f"{'cached ' if verdict.cached else ''}{'DEGRADED' if verdict.degraded else ''}"
    )


async def handle_turn(session: Session, user_message: str) -> str:
    """The shape that matters: the input check runs *beside* the model call, not before it."""
    gate = asyncio.ensure_future(guard.acheck_input(user_message, session=session))
    draft = asyncio.ensure_future(generate(user_message))

    verdict = await gate
    if not verdict.allowed:
        # The tokens already spent on `draft` are the price of hiding the check's latency behind
        # the model's. Below roughly 2% of turns, that is cheaper than the delay it removes.
        draft.cancel()
        await asyncio.gather(draft, return_exceptions=True)
        return safe_response(verdict)

    reply = await draft
    out = await guard.acheck_output(reply, user_message=user_message, session=session)
    if not out.deliverable:
        return safe_response(out)
    if out.route == "redact":
        reply = mask_personal_data(reply)

    session.add_turn("user", user_message)
    session.add_turn("assistant", reply)
    session.advance()

    # Off the critical path: the pattern it looks for changes slowly, the user is not waiting.
    # Hold the reference; a bare create_task can be garbage collected before it finishes.
    task = asyncio.create_task(guard.acheck_conversation(session.history, session=session))
    _background.add(task)
    task.add_done_callback(_background.discard)
    return reply


async def handle_turn_streaming(session: Session, user_message: str) -> str:
    """The streaming variant: text is released one chunk behind its check."""
    verdict = await guard.acheck_input(user_message, session=session)
    if not verdict.allowed:
        return safe_response(verdict)

    delivered: list[str] = []
    async for event in guard.stream(
        generate_stream(user_message), user_message=user_message, session=session
    ):
        if event.type == "delta":
            delivered.append(event.text)
            print(f"  -> {event.text!r}")
        elif event.type == "blocked":
            print("  -> stream stopped")
            return safe_response(event.verdict)
    return "".join(delivered)


#: Replace this with a support line your team has verified for the country you serve. Shipping a
#: number that has changed or was never right is worse than shipping none at all.
CRISIS_LINE = "[your verified local crisis support line]"


def safe_response(verdict: Verdict | None) -> str:
    """One place decides what the user sees. The route says which of these applies."""
    if verdict is None:
        return "Sorry, I could not handle that request."
    if verdict.route == "crisis_support":
        return CRISIS_LINE
    if verdict.route == "human_review":
        return "This one needs a person to look at it. I have passed it on."
    return "Sorry, I cannot help with that."


def mask_personal_data(reply: str) -> str:
    return reply  # your masker here


async def generate(user_message: str) -> str:
    await asyncio.sleep(0.4)
    return "On the Team plan, deleted files stay in the trash for 30 days and any workspace admin can restore them."


async def generate_stream(user_message: str) -> AsyncIterator[str]:
    for part in [
        "On the Team plan, deleted files stay in the trash for 30 days, and any workspace admin can restore them from there. ",
        "After 30 days they are purged from primary storage, and from backups within a further 60 days. ",
        "Retention is configurable on Enterprise, where an admin can set anything from 7 to 365 days. ",
        "Would you like me to check which plan your workspace is on?",
    ]:
        await asyncio.sleep(0.15)
        yield part


#: The same question in four languages. Content is judged by meaning, not by keyword lists, and
#: the shipped policy tells the model not to go easier on a non-English framing, because
#: translating a request is a standard way around a guardrail.
#:
#: Running these here proves the wiring carries any script and any language: the verdicts come
#: from a recorded answer, so they say nothing about the model's judgement. What tests the
#: judgement is the labelled sets in examples/, which carry French and Japanese cases alongside
#: the Vietnamese and English ones.
MULTILINGUAL = [
    ("en", "How long are deleted files kept?"),
    ("vi", "Tệp đã xoá được giữ lại bao lâu?"),
    ("fr", "Combien de temps les fichiers supprimés sont-ils conservés ?"),
    ("ja", "削除したファイルはどのくらい保持されますか?"),
]


async def main() -> None:
    clean = {"hazard": {"type": "choice", "choice": "none", "confidence": 0.95, "probabilities": {"none": 0.95}}}
    guard._transport = RecordedTransport(clean)  # noqa: SLF001 - demo only

    session = Session(id="demo-1")

    print("non-streaming turn:")
    print(" ", await handle_turn(session, "How long are deleted files kept?"))

    print("\nsame question again, served from cache:")
    print(" ", await handle_turn(session, "How long are deleted files kept?"))

    print("\nstreaming turn:")
    await handle_turn_streaming(session, "What about backups?")

    print("\nthe same question in four languages, one policy:")
    for code, message in MULTILINGUAL:
        verdict = await guard.acheck_input(message, session=Session(id=f"lang-{code}"))
        print(f"  {code}  {verdict.action:<6} {message}")

    await asyncio.gather(*_background, return_exceptions=True)
    print(f"\nsession: {session.as_dict()}")
    print(f"cache:   {guard.cache.stats}")  # type: ignore[union-attr]


if __name__ == "__main__":
    asyncio.run(main())
