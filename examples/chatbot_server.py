"""A guarded chatbot behind HTTP: FastAPI, Claude, and guardrail-chatbot-jev.

`integration.py` shows the guarded turn on its own. This is the same turn wired into a real
server, which adds the three things a deployment actually has to solve: sessions that outlive a
single function call, streaming to a browser, and what to serve when a check says no.

Run it with no credentials at all. The model is stubbed and verdicts come from a recorded
transport, so every path below still executes:

    pip install -e './python[server]'
    uvicorn examples.chatbot_server:app --port 8000

    curl -s localhost:8000/chat -H 'content-type: application/json' \
         -d '{"conversation_id": "c1", "message": "How long are deleted files kept?"}'

    curl -N localhost:8000/chat/stream -H 'content-type: application/json' \
         -d '{"conversation_id": "c1", "message": "What about backups?"}'

Content is judged by meaning, so any language reaches the same policy. The system prompt asks the
model to answer in the language it was written to:

    curl -s localhost:8000/chat -H 'content-type: application/json' \
         -d '{"conversation_id": "c2", "message": "Tệp đã xoá được giữ lại bao lâu?"}'

    curl -s localhost:8000/chat -H 'content-type: application/json' \
         -d '{"conversation_id": "c3", "message": "削除したファイルはどのくらい保持されますか?"}'

Give it credentials and the same code talks to both services instead:

    export ANTHROPIC_API_KEY=sk-ant-...
    export JEV_API_KEY=sk-...

Set REDIS_URL (and `pip install redis`) to move sessions out of the process, which is what lets
it run on more than one worker without forgetting a conversation halfway through:

    export REDIS_URL=redis://localhost:6379/0

`GET /healthz` reports which of the two it is using, because a demo that silently stubs the model
is worse than no demo.
"""

from __future__ import annotations

import asyncio
import json
import os
from typing import Any, AsyncIterator, Dict, Optional, Set

from fastapi import FastAPI
from fastapi.responses import StreamingResponse
from pydantic import BaseModel, Field

from examples.session_store import build_store
from guardrail_chatbot_jev import (
    COMMON_PATTERNS,
    Guard,
    LRUCache,
    PatternPrefilter,
    RecordedTransport,
    Session,
    Verdict,
)

# --- the guard ----------------------------------------------------------------
# One Guard per process. The cache, prefilter and observer are process-wide; sessions are not.

LIVE_JEV = bool(os.environ.get("JEV_API_KEY"))

#: A recorded "nothing fired" answer, so the server runs without a Jev endpoint. Delete this and
#: the transport argument below when you have credentials.
CLEAN_ANSWER = {
    "hazard": {"type": "choice", "choice": "none", "confidence": 0.95, "probabilities": {"none": 0.95}}
}


def emit_metric(verdict: Verdict) -> None:
    """Every verdict, including cached and degraded ones.

    Count `degraded` separately from `block`. A week with 5% degraded verdicts means the guardrail
    was only actually running 95% of the time, and that must not hide inside the block rate.
    """
    print(
        f"[metric] {verdict.surface:<12} {verdict.action:<6} route={verdict.route:<14}"
        f" {verdict.latency_ms:6.1f}ms"
        f" {'cached ' if verdict.cached else ''}{'DEGRADED' if verdict.degraded else ''}",
        flush=True,  # stdout is block-buffered behind a server; without this metrics arrive late
    )


guard = Guard(
    cache=LRUCache(capacity=8192, ttl=300),
    prefilter=PatternPrefilter(list(COMMON_PATTERNS)),
    observer=emit_metric,
    # A guardrail that hangs is a guardrail that takes the chatbot down with it. On the input
    # surface the shipped policy fails open, so this timeout degrades rather than blocks.
    timeout=2.0,
    transport=None if LIVE_JEV else RecordedTransport(CLEAN_ANSWER),
)

# --- the model ----------------------------------------------------------------

MODEL = "claude-opus-5"
SYSTEM = (
    "You are the support assistant for Helio, a team file storage service. Answer in the language "
    "the user writes in. Be brief and concrete. If you do not know a policy detail, say so rather "
    "than guessing at it."
)

LIVE_MODEL = bool(os.environ.get("ANTHROPIC_API_KEY"))
_claude: Optional[Any] = None


def claude() -> Any:
    """Built on first use, so importing this module never needs a key."""
    global _claude
    if _claude is None:
        from anthropic import AsyncAnthropic

        _claude = AsyncAnthropic()
    return _claude


def _request(session: Session, user_message: str) -> Dict[str, Any]:
    """The request body both the buffered and the streaming path send.

    `effort: low` because a support reply is not a reasoning problem, and effort is the first
    latency lever: it trades thoroughness for tokens and time within one model. Raise it for
    routes where the answer is actually hard.

    `fallbacks` is the server-side refusal fallback. Claude's own safety classifiers can decline a
    request, and without this the turn simply stops; with it the API re-runs the same request on a
    fallback model inside the same call and the user gets an answer.
    """
    return {
        "model": MODEL,
        "max_tokens": 2048,
        "system": SYSTEM,
        "output_config": {"effort": "low"},
        "betas": ["server-side-fallback-2026-07-01"],
        "fallbacks": "default",
        "messages": [{"role": t.role, "content": t.content} for t in session.history]
        + [{"role": "user", "content": user_message}],
    }


async def generate(session: Session, user_message: str) -> str:
    if not LIVE_MODEL:
        await asyncio.sleep(0.4)  # stand in for the model's latency, which is the point below
        return STUB_REPLY
    response = await claude().beta.messages.create(**_request(session, user_message))
    if response.stop_reason == "refusal":
        # Claude declined and so did every fallback. Not a guardrail verdict, but the same
        # outcome for the user, so it goes through the same door.
        return ""
    return "".join(block.text for block in response.content if block.type == "text")


async def generate_stream(session: Session, user_message: str) -> AsyncIterator[str]:
    if not LIVE_MODEL:
        for part in STUB_STREAM:
            await asyncio.sleep(0.15)
            yield part
        return
    # A refusal before any output is rescued by `fallbacks` inside the same call. One that lands
    # mid-stream just ends the stream early; the text already yielded was checked and stands, and
    # the `done` verdict still describes what the user actually received.
    async with claude().beta.messages.stream(**_request(session, user_message)) as stream:
        async for text in stream.text_stream:
            yield text


STUB_REPLY = "On the Team plan, deleted files stay in the trash for 30 days and any workspace admin can restore them."

#: Long enough to cross the 280-character chunk boundary more than once, so the streaming path
#: below really does release text in pieces instead of arriving as one block.
STUB_STREAM = [
    "On the Team plan, deleted files stay in the trash for 30 days, and any workspace admin can restore them from there. ",
    "After 30 days they are purged from primary storage, and from backups within a further 60 days. ",
    "Retention is configurable on Enterprise, where an admin can set anything from 7 to 365 days. ",
    "Would you like me to check which plan your workspace is on?",
]

# --- sessions -----------------------------------------------------------------
# A Session carries risk between turns, which only works if it outlives the request. The store
# owns where it lives and the lock that keeps two turns of one conversation from interleaving;
# see session_store.py for why both of those are decisions and not details. Set REDIS_URL and the
# same server runs behind `--workers 4` without silently forgetting who it is talking to.

store = build_store(os.environ.get("REDIS_URL"))


#: Strong references to fire-and-forget checks, so the event loop cannot drop them mid-flight.
_background: Set["asyncio.Task[None]"] = set()


async def _check_conversation(conversation_id: str) -> None:
    """Run the conversation check in its own transaction.

    It has to re-open the session rather than borrow the one from the turn. This runs after that
    transaction has closed and saved, so a floor raised on a borrowed object would be raised on a
    copy nobody reads again. Re-opening also means it takes the lock, which is what keeps it from
    racing the next turn.
    """
    async with store.transaction(conversation_id) as session:
        if not session.history:
            return
        await guard.acheck_conversation(session.history, session=session)


def check_conversation_later(conversation_id: str) -> None:
    """Off the critical path: the escalation it looks for builds over turns, nobody is waiting.

    The cost is that its floor lands on the next turn rather than this one, which is inherent:
    the pattern is not visible until the turn it completes exists.
    """
    task = asyncio.create_task(_check_conversation(conversation_id))
    _background.add(task)
    task.add_done_callback(_background.discard)


# --- what the user sees when a check says no ----------------------------------

#: Replace this with a support line your team has verified for the country you serve. Shipping a
#: number that has changed or was never right is worse than shipping none at all.
CRISIS_LINE = "[your verified local crisis support line]"


def safe_response(verdict: Optional[Verdict]) -> str:
    """One place decides what the user sees. The route says which of these applies."""
    if verdict is None:
        return "Sorry, I could not handle that request."
    if verdict.route == "crisis_support":
        return CRISIS_LINE
    if verdict.route == "human_review":
        return "This one needs a person to look at it. I have passed it on."
    if verdict.route == "guide":
        return "I can only help with our own products and services."
    return "Sorry, I cannot help with that."


def mask_personal_data(reply: str) -> str:
    return reply  # your masker here


def verdict_json(verdict: Optional[Verdict]) -> Dict[str, Any]:
    """What a client may see. Findings are internal: they describe how to attack the guardrail."""
    if verdict is None:
        return {}
    return {
        "action": verdict.action,
        "route": verdict.route,
        "surface": verdict.surface,
        "degraded": verdict.degraded,
    }


# --- the API ------------------------------------------------------------------

app = FastAPI(title="guarded chatbot")


class ChatRequest(BaseModel):
    conversation_id: str = Field(min_length=1, max_length=128)
    message: str = Field(min_length=1, max_length=8000)


@app.get("/healthz")
async def healthz() -> Dict[str, Any]:
    return {
        "jev": "live" if LIVE_JEV else "recorded (set JEV_API_KEY)",
        "model": MODEL if LIVE_MODEL else "stubbed (set ANTHROPIC_API_KEY)",
        "sessions": await store.stats(),
        "cache": guard.cache.stats if guard.cache is not None else None,
    }


@app.post("/chat")
async def chat(request: ChatRequest) -> Dict[str, Any]:
    """The buffered turn. The input check runs *beside* the model call, not before it."""
    async with store.transaction(request.conversation_id) as session:
        gate = asyncio.ensure_future(guard.acheck_input(request.message, session=session))
        draft = asyncio.ensure_future(generate(session, request.message))

        verdict = await gate
        if not verdict.allowed:
            # The tokens already spent on `draft` are the price of hiding the check's latency
            # behind the model's. Below roughly 2% of turns, that is cheaper than the delay it
            # removes. If a violating prompt must never reach the model, await the gate first.
            draft.cancel()
            await asyncio.gather(draft, return_exceptions=True)
            return {"reply": safe_response(verdict), "verdict": verdict_json(verdict)}

        reply = await draft
        if not reply:  # the model itself declined
            return {"reply": safe_response(None), "verdict": {"action": "block", "route": "safe_response"}}

        out = await guard.acheck_output(reply, user_message=request.message, session=session)
        if not out.deliverable:
            return {"reply": safe_response(out), "verdict": verdict_json(out)}
        if out.route == "redact":
            reply = mask_personal_data(reply)

        session.add_turn("user", request.message)
        session.add_turn("assistant", reply)
        session.advance()
        check_conversation_later(request.conversation_id)

        return {"reply": reply, "verdict": verdict_json(out)}


@app.post("/chat/stream")
async def chat_stream(request: ChatRequest) -> StreamingResponse:
    """The streamed turn, as server-sent events.

    Nothing is written to the response until its check has returned, so a blocked chunk is never
    a retraction of text the user already read.
    """

    async def events() -> AsyncIterator[str]:
        async with store.transaction(request.conversation_id) as session:
            verdict = await guard.acheck_input(request.message, session=session)
            if not verdict.allowed:
                yield sse({"type": "blocked", "text": safe_response(verdict), "verdict": verdict_json(verdict)})
                return

            async for event in guard.stream(
                generate_stream(session, request.message),
                user_message=request.message,
                session=session,
            ):
                if event.type == "delta":
                    yield sse({"type": "delta", "text": event.text})
                elif event.type == "blocked":
                    # Everything already yielded was cleared; this stops the rest.
                    yield sse(
                        {
                            "type": "blocked",
                            "text": safe_response(event.verdict),
                            "verdict": verdict_json(event.verdict),
                        }
                    )
                    return
                elif event.type == "done":
                    # `done` carries the complete reply, already checked as a whole.
                    session.add_turn("user", request.message)
                    session.add_turn("assistant", event.text)
                    session.advance()
                    check_conversation_later(request.conversation_id)
                    yield sse({"type": "done", "verdict": verdict_json(event.verdict)})

    return StreamingResponse(
        events(),
        media_type="text/event-stream",
        # Without this an nginx or CDN hop buffers the whole reply and the streaming is decorative.
        headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"},
    )


def sse(payload: Dict[str, Any]) -> str:
    return f"data: {json.dumps(payload, ensure_ascii=False)}\n\n"
