"""Guarding a streamed reply.

A streamed reply cannot be checked before its first token, and a check that waits for the last one
gives up streaming altogether. The middle path here is to release the text one chunk behind: chunk
*k* is held until its check returns, while the model is already producing chunk *k+1*. Only the
first chunk pays the full latency.

Mid-stream checks ask the sentinel questions only: the categories where a miss is unacceptable,
which are also the ones judgeable from partial text. The categories that need a whole reply to
assess would only produce noise on a fragment, and skipping the 18-label choice is what keeps a
per-chunk check cheap. The complete reply gets the full question set at the end.
"""

from __future__ import annotations

import asyncio
import re
from dataclasses import dataclass
from typing import Any, AsyncIterator, Literal, Sequence

from .types import Verdict

#: Prefer to cut after sentence-ending punctuation, including the Vietnamese and CJK forms.
_BOUNDARY = re.compile(r"(?<=[.!?;:\n。！？])\s")

EventType = Literal["delta", "blocked", "done"]


@dataclass(frozen=True, slots=True)
class StreamEvent:
    """One thing that happened while the reply was being guarded.

    ``delta`` carries text cleared for delivery. ``blocked`` means the stream stopped and nothing
    further should be sent. ``done`` carries the verdict on the complete reply.
    """

    type: EventType
    text: str = ""
    verdict: Verdict | None = None


async def guard_stream(
    guard: Any,
    source: AsyncIterator[str],
    *,
    user_message: str | None = None,
    context: str | Sequence[str] | None = None,
    session: Any | None = None,
    chunk_chars: int = 280,
    metadata: Any | None = None,
) -> AsyncIterator[StreamEvent]:
    """Wrap a token stream, releasing text one chunk behind its safety check.

    Args:
        guard: A ``Guard``.
        source: The model's stream of text deltas.
        chunk_chars: Minimum characters before looking for a sentence boundary to cut at.
            Smaller means more round trips and a tighter hold; larger means fewer, coarser checks.

    Yields:
        ``StreamEvent``s. Stop consuming on ``blocked``; ``done`` is always last unless blocked.
    """
    # Mid-stream checks read each chunk on its own. In a watched session a part that is harmful only in
    # the light of earlier turns would get past them, so the reply is held whole for the final check,
    # which reads it in context.
    context_check = getattr(guard, "context_check", None)
    if (
        getattr(guard, "multiturn", "floor") == "attribute"
        and context_check is not None
        and not context_check.never
        and session is not None
        and (context_check.always or session.watching(context_check.watch_risk or 0.2))
    ):
        chunk_chars = 2**31 - 1

    queue: asyncio.Queue[str | None] = asyncio.Queue(maxsize=64)
    failure: list[BaseException] = []

    async def produce() -> None:
        try:
            async for delta in source:
                await queue.put(delta)
        except BaseException as exc:  # noqa: BLE001 - re-raised on the consumer side
            failure.append(exc)
        finally:
            await queue.put(None)

    producer = asyncio.create_task(produce())
    delivered = ""
    pending = ""
    blocked = False

    try:
        while True:
            delta = await queue.get()
            if delta is None:
                break
            pending += delta
            chunk = _take_chunk(pending, chunk_chars)
            if chunk is None:
                continue
            pending = pending[len(chunk) :]

            verdict = await guard.acheck_output(
                delivered + chunk,
                user_message=user_message,
                context=context,
                metadata=metadata,
                quick=True,
                session=session,
            )
            if not verdict.deliverable:
                blocked = True
                yield StreamEvent("blocked", verdict=verdict)
                return
            delivered += chunk
            yield StreamEvent("delta", text=chunk)

        if failure:
            raise failure[0]

        final_text = delivered + pending
        verdict = await guard.acheck_output(
            final_text,
            user_message=user_message,
            context=context,
            metadata=metadata,
            session=session,
        )
        if not verdict.deliverable:
            blocked = True
            yield StreamEvent("blocked", verdict=verdict)
            return
        if pending:
            yield StreamEvent("delta", text=pending)
        yield StreamEvent("done", text=final_text, verdict=verdict)
    finally:
        if blocked or not producer.done():
            producer.cancel()
        await asyncio.gather(producer, return_exceptions=True)


def _take_chunk(pending: str, chunk_chars: int) -> str | None:
    """The next releasable chunk, cut at a sentence boundary once past ``chunk_chars``."""
    if len(pending) < chunk_chars:
        return None
    match = _BOUNDARY.search(pending, chunk_chars)
    if match:
        return pending[: match.start() + 1]
    # No boundary in sight and the buffer is getting long: cut anyway rather than hold the
    # stream hostage to a model that is writing one very long sentence.
    if len(pending) >= chunk_chars * 3:
        return pending[:chunk_chars]
    return None
