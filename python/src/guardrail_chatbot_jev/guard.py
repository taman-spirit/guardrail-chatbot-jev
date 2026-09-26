"""The public entry point: one ``Guard``, three checks, and the plumbing around them."""

from __future__ import annotations

import asyncio
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass, replace
from pathlib import Path
from typing import Any, AsyncIterator, Callable, Mapping, Sequence

from .cache import VerdictCache, cache_key
from .client import Transport, auto_transport
from .decide import _route, decide, error_verdict, now_ms, with_floor
from .policy import Policy
from .prefilter import Prefilter
from .questions import (
    CONTEXT_COMPLETES,
    CONTEXT_DISENGAGES,
    build_questions,
    context_questions,
    conversation_state,
    input_state,
    output_in_context_state,
    output_state,
)
from .session import WITHHELD_PLACEHOLDER, Session
from .streaming import StreamEvent, guard_stream
from .types import GuardrailError, Surface, Turn, Verdict, as_turns, rank, stronger

#: Conversation state changes every turn, so caching it buys nothing and only costs memory.
DEFAULT_CACHE_SURFACES: frozenset[str] = frozenset({"input", "output"})


@dataclass(frozen=True, slots=True)
class ContextCheck:
    """Tunes the in-context output check that ``multiturn="attribute"`` uses.

    ``watch_risk`` is the session risk from which replies are also read in context; ``attribution``
    is how sure Jev must be that the reply completes an earlier harmful request before the in-context
    findings count.
    """

    always: bool = False
    never: bool = False
    watch_risk: float = 0.2
    attribution: float = 0.5


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
        multiturn: str = "attribute",
        context_check: ContextCheck | None = None,
        review_handling: str = "hold",
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
        if multiturn not in ("attribute", "floor"):
            raise ValueError("multiturn must be 'attribute' or 'floor'")
        if review_handling not in ("hold", "audit"):
            raise ValueError("review_handling must be 'hold' or 'audit'")
        #: "attribute" holds a turn only for what it or its reply does; "floor" is the earlier
        #: behaviour, which raised every later turn to a floor after a conversation-level review.
        self.multiturn = multiturn
        self.context_check = context_check or ContextCheck()
        #: "audit" is for realtime chat: a review verdict delivers and is queued for a person.
        self.review_handling = review_handling
        self._pool: ThreadPoolExecutor | None = None

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
        history: Sequence[Any] | None = None,
    ) -> Verdict:
        """Check an assistant reply before it reaches the user.

        Pass ``context`` (the retrieved passages the reply was meant to be based on) to enable
        the groundedness signal, which catches claims the context does not support.

        ``quick=True`` asks only the sentinel questions. It is what mid-stream checks use on
        incomplete text; a complete reply should get the full set.

        In a watched session the complete reply is also read against the earlier turns (``history``,
        or the session's), in a second request sent alongside the first. Those findings count only
        when the reply itself completes an earlier harmful request; see ``multiturn``.
        """
        meta = _metadata(metadata, session)
        state = output_state(reply, user_message=user_message, context=context, metadata=meta)
        subset = "sentinels" if quick else "full"
        earlier = as_turns(history) if history else (session.history if session is not None else ())
        if quick or not self._context_check_applies(session, earlier):
            return self._run("output", state, has_context=bool(context), model=model, session=session, subset=subset)

        # Both requests go out together, so the in-context read adds no latency, and the standalone
        # check never sees the history: its answer stays uncontaminated by what came before.
        if self._pool is None:
            self._pool = ThreadPoolExecutor(max_workers=4, thread_name_prefix="guardrail-context")
        in_context = self._pool.submit(self._check_in_context, reply, user_message, earlier, meta, model)
        verdict = self._evaluate("output", state, has_context=bool(context), model=model, subset=subset)
        return self._finish(self._attribute(verdict, in_context.result()), session)

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

        A transcript with nothing in it but withheld turns is not sent: there is no content to read,
        and Jev, asked to judge only omissions, answers from what they might have been.
        """
        parsed = as_turns(turns)
        if all(t.content == WITHHELD_PLACEHOLDER for t in parsed):
            nothing = Verdict(
                action="allow", surface="conversation", route="deliver", confidence=1.0,
                applied_rules=("nothing-to-read",), policy_id=f"{self.policy.id}@{self.policy.version}",
            )
            return self._finish(nothing, session)
        state = conversation_state(parsed, metadata=_metadata(metadata, session))
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
        verdict = self._evaluate(surface, state, has_context=has_context, model=model, subset=subset)
        return self._finish(verdict, session)

    def _evaluate(
        self,
        surface: Surface,
        state: Any,
        *,
        has_context: bool = False,
        model: str | None = None,
        subset: str = "full",
    ) -> Verdict:
        """Reach a verdict for one state without touching the session or the observer."""
        if self.prefilter is not None:
            decided = self.prefilter(self.policy, surface, state)
            if decided is not None:
                return decided

        key: str | None = None
        if self.cache is not None and surface in self.cache_surfaces:
            key = cache_key(f"{self.policy.id}@{self.policy.version}", surface, state, subset)
            hit = self.cache.get(key)
            if hit is not None:
                return hit

        questions = build_questions(self.policy, surface, has_context=has_context, subset=subset)
        started = now_ms()
        try:
            answers, used_model, usage = self.transport.system_one(
                state, questions, model=model, timeout=self.timeout
            )
        except GuardrailError as exc:
            if self.raise_on_error:
                raise
            return error_verdict(self.policy, surface, exc, latency_ms=now_ms() - started)

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
        return verdict

    def _audit(self, verdict: Verdict) -> Verdict:
        """Mark what a person should look at later; under review_handling="audit", let a review
        through. A degraded verdict keeps its route: when Jev could not be reached nothing was
        checked, so a fail-closed surface still holds."""
        route = verdict.route
        if self.review_handling == "audit" and route == "human_review" and not verdict.degraded:
            route = "deliver_and_audit"
        return replace(verdict, audit=_audit_level(verdict), route=route)

    # -- multi-turn -----------------------------------------------------

    def _context_check_applies(self, session: Session | None, earlier: Sequence[Turn]) -> bool:
        c = self.context_check
        if c.never or self.multiturn == "floor" or not earlier:
            return False
        if c.always:
            return True
        return session is not None and session.watching(c.watch_risk if c.watch_risk > 0 else 0.2)

    def _check_in_context(
        self, reply: str, user_message: str | None, earlier: Sequence[Turn], metadata: Any, model: str | None
    ) -> tuple[Verdict | None, float, float, Exception | None]:
        try:
            answers, used_model, usage = self.transport.system_one(
                output_in_context_state(reply, user_message=user_message, earlier=earlier, metadata=metadata),
                context_questions(self.policy),
                model=model,
                timeout=self.timeout,
            )
        except GuardrailError as exc:
            return None, 0.0, 0.0, exc
        verdict = decide(self.policy, "output", answers, model=used_model, usage=usage)
        completes = float((answers.get(CONTEXT_COMPLETES) or {}).get("noul", 0.0) or 0.0)
        disengages = float((answers.get(CONTEXT_DISENGAGES) or {}).get("noul", 0.0) or 0.0)
        return verdict, completes, disengages, None

    def _attribute(self, v: Verdict, result: tuple[Verdict | None, float, float, Exception | None]) -> Verdict:
        """Merge the in-context findings into the standalone verdict, but only when the reply itself
        completes an earlier harmful request. Otherwise they are recorded and change nothing."""
        ctx_verdict, completes, disengages, err = result
        read: dict[str, Any] = {"ran": True, "attributed": False, "completes": completes, "disengages": disengages, "categories": []}
        if err is not None or ctx_verdict is None:
            # The standalone check still stands; an outage of the second read is not evidence.
            read["error"] = str(err)
            return replace(v, context=read, applied_rules=v.applied_rules + ("context-unavailable",))
        fired = [f for f in ctx_verdict.findings if rank(f.action) >= rank("flag")]
        read["categories"] = [f.category for f in fired]
        threshold = self.context_check.attribution if self.context_check.attribution > 0 else 0.5
        read["attributed"] = bool(fired) and completes >= threshold and completes >= disengages
        if not fired:
            return replace(v, context=read)
        if not read["attributed"]:
            return replace(v, context=read, applied_rules=v.applied_rules + ("context-not-attributed",))

        findings = list(v.findings)
        for f in fired:
            f = replace(f, source="context:" + f.source, notes=f.notes + ("completes an earlier request",))
            same = next((i for i, x in enumerate(findings) if x.category == f.category), None)
            if same is not None:
                if rank(f.action) > rank(findings[same].action):
                    findings[same] = f
                continue
            findings.append(f)
        findings.sort(key=lambda f: (-rank(f.action), -f.probability))
        action = v.action
        for f in findings:
            action = stronger(action, f.action)
        return replace(
            v,
            findings=tuple(findings),
            action=action,
            route=_route(self.policy, findings, action),
            severity=max(v.severity, ctx_verdict.severity),
            applied_rules=v.applied_rules + ("context-attributed",),
            context=read,
        )

    def _finish(self, verdict: Verdict, session: Session | None) -> Verdict:
        """Mark the audit level, apply the session floor ("floor" mode only), tell the session, and
        emit to the observer."""
        verdict = self._audit(verdict)
        if session is not None:
            floor = session.floor
            if floor != "allow" and self.multiturn == "floor":
                verdict = with_floor(
                    self.policy, verdict, floor, note=f"session-floor:{session.id or 'unnamed'}"
                )
            session.observe(verdict)
        if self.observer is not None:
            self.observer(verdict)
        return verdict


def _audit_level(verdict: Verdict) -> str | None:
    if rank(verdict.action) >= rank("review"):
        return "priority"
    return "sample" if verdict.action == "flag" else None


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
