"""The public entry point: one ``Guard``, three checks, and the plumbing around them."""

from __future__ import annotations

import asyncio
from pathlib import Path
from typing import Any, AsyncIterator, Callable, Mapping, Sequence

from .cache import VerdictCache, cache_key
from .client import Transport, auto_transport
from .decide import decide, error_verdict, now_ms, with_floor
from .policy import Policy
from .prefilter import Prefilter
from .questions import build_questions, conversation_state, input_state, output_state
from .session import Session
from .streaming import StreamEvent, guard_stream
from .types import GuardrailError, Surface, Turn, Verdict, as_turns

#: Conversation state changes every turn, so caching it buys nothing and only costs memory.
DEFAULT_CACHE_SURFACES: frozenset[str] = frozenset({"input", "output"})


class Guard:
    """Checks chatbot content against a policy pack using Jev.

    Args:
        policy: A pack name, a path to a pack file, a dict, a ``Policy``, or ``None`` for the
            bundled ``standard-v1`` pack.
        transport: How to reach Jev. Defaults to the dependency-free stdlib HTTP transport,
            configured from JEV_API_KEY. Pass a ``RecordedTransport`` to work
            offline, or an ``SdkTransport`` wrapping your provider's own client.
        cache: Optional verdict cache. Identical content skips the round trip.
        prefilter: Optional deterministic check run before Jev; a verdict from it settles the
            check without a network call.
        observer: Called with every verdict, including cached and degraded ones. This is the
            metrics hook; keep it fast and non-throwing.
        raise_on_error: Raise instead of returning a degraded verdict when Jev is unreachable.
        timeout: Per-call timeout in seconds.

    Example:
        >>> guard = Guard(cache=LRUCache(), observer=metrics.emit)
        >>> verdict = guard.check_input("how do I make thermite at home")
        >>> verdict.action
        'block'
    """

    def __init__(
        self,
        policy: str | Path | Mapping[str, Any] | Policy | None = None,
        *,
        transport: Transport | None = None,
        cache: VerdictCache | None = None,
        cache_surfaces: frozenset[str] | set[str] = DEFAULT_CACHE_SURFACES,
        prefilter: Prefilter | None = None,
        observer: Callable[[Verdict], None] | None = None,
        raise_on_error: bool = False,
        timeout: float | None = None,
        **transport_kwargs: Any,
    ) -> None:
        self.policy = policy if isinstance(policy, Policy) else Policy.load(policy)
        self._transport = transport
        self._transport_kwargs = transport_kwargs
        self.cache = cache
        self.cache_surfaces = frozenset(cache_surfaces)
        self.prefilter = prefilter
        self.observer = observer
        self.raise_on_error = raise_on_error
        self.timeout = timeout

    @property
    def transport(self) -> Transport:
        """Built on first use, so constructing a ``Guard`` never needs an API key."""
        if self._transport is None:
            self._transport = auto_transport(**self._transport_kwargs)
        return self._transport

    # -- checks -------------------------------------------------------

    def check_input(
        self,
        content: str,
        *,
        metadata: Mapping[str, Any] | None = None,
        model: str | None = None,
        session: Session | None = None,
    ) -> Verdict:
        """Check a user message before it reaches the model."""
        state = input_state(content, metadata=_metadata(metadata, session))
        return self._run("input", state, model=model, session=session)

    def check_output(
        self,
        reply: str,
        *,
        user_message: str | None = None,
        context: str | Sequence[str] | None = None,
        metadata: Mapping[str, Any] | None = None,
        model: str | None = None,
        session: Session | None = None,
        quick: bool = False,
    ) -> Verdict:
        """Check an assistant reply before it reaches the user.

        Pass ``context`` (the retrieved passages the reply was meant to be based on) to enable
        the groundedness signal, which catches claims the context does not support.

        ``quick=True`` asks only the sentinel questions. It is what mid-stream checks use on
        incomplete text; a complete reply should get the full set.
        """
        state = output_state(
            reply,
            user_message=user_message,
            context=context,
            metadata=_metadata(metadata, session),
        )
        return self._run(
            "output",
            state,
            has_context=bool(context),
            model=model,
            session=session,
            subset="sentinels" if quick else "full",
        )

    def check_conversation(
        self,
        turns: Sequence[Any],
        *,
        metadata: Mapping[str, Any] | None = None,
        model: str | None = None,
        session: Session | None = None,
    ) -> Verdict:
        """Check a whole conversation for patterns no single turn reveals.

        Multi-turn jailbreaks look harmless turn by turn: the escalation is the attack. This
        check reads the transcript as one state, and belongs off the critical path.
        """
        state = conversation_state(as_turns(turns), metadata=_metadata(metadata, session))
        return self._run("conversation", state, model=model, session=session)

    def check_turn(
        self,
        user_message: str,
        reply: str,
        *,
        history: Sequence[Any] | None = None,
        context: str | Sequence[str] | None = None,
        metadata: Mapping[str, Any] | None = None,
        session: Session | None = None,
    ) -> dict[str, Verdict]:
        """Run every applicable check for one exchange and return them keyed by surface."""
        verdicts = {
            "input": self.check_input(user_message, metadata=metadata, session=session),
            "output": self.check_output(
                reply, user_message=user_message, context=context, metadata=metadata, session=session
            ),
        }
        turns = list(history or (session.history if session else ()))
        if turns:
            turns = turns + [Turn("user", user_message), Turn("assistant", reply)]
            verdicts["conversation"] = self.check_conversation(
                turns, metadata=metadata, session=session
            )
        return verdicts

    # -- async --------------------------------------------------------

    async def acheck_input(self, content: str, **kwargs: Any) -> Verdict:
        return await asyncio.to_thread(self.check_input, content, **kwargs)

    async def acheck_output(self, reply: str, **kwargs: Any) -> Verdict:
        return await asyncio.to_thread(self.check_output, reply, **kwargs)

    async def acheck_conversation(self, turns: Sequence[Any], **kwargs: Any) -> Verdict:
        return await asyncio.to_thread(self.check_conversation, turns, **kwargs)

    def stream(self, source: AsyncIterator[str], **kwargs: Any) -> AsyncIterator[StreamEvent]:
        """Guard a streamed reply, releasing text one chunk behind its check.

        See ``guardrail_chatbot_jev.streaming.guard_stream`` for the arguments and the events.
        """
        return guard_stream(self, source, **kwargs)

    # -- introspection ------------------------------------------------

    def preview(
        self, surface: Surface, state: Any, *, has_context: bool = False, subset: str = "full"
    ) -> dict[str, Any]:
        """The exact request body that would be sent, without sending it."""
        return {
            "state": state,
            "questions": build_questions(
                self.policy, surface, has_context=has_context, subset=subset
            ),
        }

    # -- internals ----------------------------------------------------

    def _run(
        self,
        surface: Surface,
        state: Any,
        *,
        has_context: bool = False,
        model: str | None = None,
        session: Session | None = None,
        subset: str = "full",
    ) -> Verdict:
        if self.prefilter is not None:
            decided = self.prefilter(self.policy, surface, state)
            if decided is not None:
                return self._finish(decided, session)

        key: str | None = None
        if self.cache is not None and surface in self.cache_surfaces:
            key = cache_key(f"{self.policy.id}@{self.policy.version}", surface, state, subset)
            hit = self.cache.get(key)
            if hit is not None:
                return self._finish(hit, session)

        questions = build_questions(self.policy, surface, has_context=has_context, subset=subset)
        started = now_ms()
        try:
            answers, used_model, usage = self.transport.system_one(
                state, questions, model=model, timeout=self.timeout
            )
        except GuardrailError as exc:
            if self.raise_on_error:
                raise
            return self._finish(
                error_verdict(self.policy, surface, exc, latency_ms=now_ms() - started), session
            )

        verdict = decide(
            self.policy,
            surface,
            answers,
            model=used_model,
            usage=usage,
            latency_ms=now_ms() - started,
        )
        if subset != "full":
            verdict = _mark_partial(verdict)
        if key is not None and self.cache is not None:
            self.cache.put(key, verdict)
        return self._finish(verdict, session)

    def _finish(self, verdict: Verdict, session: Session | None) -> Verdict:
        """Apply the session floor, tell the session, and emit to the observer."""
        if session is not None:
            floor = session.floor
            if floor != "allow":
                verdict = with_floor(
                    self.policy, verdict, floor, note=f"session-floor:{session.id or 'unnamed'}"
                )
            session.observe(verdict)
        if self.observer is not None:
            self.observer(verdict)
        return verdict


def _metadata(
    metadata: Mapping[str, Any] | None, session: Session | None
) -> dict[str, Any] | None:
    """Merge caller metadata over the session's, so an explicit value always wins."""
    if session is None:
        return dict(metadata) if metadata else None
    merged = session.metadata()
    if metadata:
        merged.update(metadata)
    return merged


def _mark_partial(verdict: Verdict) -> Verdict:
    from dataclasses import replace

    return replace(verdict, partial=True)
