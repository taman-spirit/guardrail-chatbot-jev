"""Carrying risk across turns.

A multi-turn attack is made of turns that are each defensible on their own. Judging every turn
from a standing start is what makes that work. A session keeps the transcript, accumulates a
decaying risk score, and raises a floor under the next few turns when it has already seen
something, so a conversation that has been escalating is not read as if it had just begun.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Mapping, Sequence

from .types import LADDER, Action, Turn, Verdict, as_turns, rank, stronger

#: Stands in the transcript for a message the guardrail withheld. The attempt stays visible to the
#: conversation check, but its text does not: neither Jev nor the model reads a blocked request again.
#: The wording is neutral on purpose: measured against Jev, "[message withheld by the safety check]"
#: still drew sentinel answers of 0.08 to 0.15 after a harmless question; this one drew 0.04 or less.
WITHHELD_PLACEHOLDER = "[earlier message omitted]"

#: How much each action contributes to the session's risk score.
ACTION_RISK: Mapping[Action, float] = {"allow": 0.0, "flag": 0.25, "review": 0.6, "block": 1.0}


@dataclass
class Session:
    """Per-conversation state. Not thread-safe; use one per conversation.

    Args:
        id: The conversation identifier, carried into verdict metadata.
        decay: How much of the previous risk survives each turn. 0.5 means a single flagged turn
            stops mattering after three or four clean ones.
        carry_turns: How many following turns a raised floor applies to.
        max_turns: Transcript window kept for conversation checks. The escalation pattern a
            conversation check looks for lives in the recent turns, so a short window finds it
            just as well and costs a fraction of the input tokens. Jev's state limit is 32k
            tokens, which leaves room for roughly 40 turns of ordinary chat if you want a longer
            memory; raise it when your conversations genuinely build over more than ten turns.
    """

    id: str = ""
    decay: float = 0.5
    carry_turns: int = 2
    max_turns: int = 10
    turns: list[Turn] = field(default_factory=list)
    risk: float = 0.0
    verdicts: list[Verdict] = field(default_factory=list)
    _floor: Action = "allow"
    _floor_left: int = 0

    # -- transcript ---------------------------------------------------

    def add_turn(self, role: str, content: str) -> None:
        self.turns.append(Turn(role=role, content=content))  # type: ignore[arg-type]
        if len(self.turns) > self.max_turns:
            del self.turns[: len(self.turns) - self.max_turns]

    def extend(self, turns: Sequence[Any]) -> None:
        for turn in as_turns(turns):
            self.add_turn(turn.role, turn.content)

    @property
    def history(self) -> tuple[Turn, ...]:
        return tuple(self.turns)

    def record(self, role: str, content: str, verdict: Verdict) -> None:
        """Append a turn given the verdict on it; a withheld turn is kept as the placeholder."""
        self.add_turn(role, content if verdict.deliverable else WITHHELD_PLACEHOLDER)

    def model_history(self) -> tuple[Turn, ...]:
        """The transcript for the chat model: withheld turns are left out entirely."""
        return tuple(t for t in self.turns if t.content != WITHHELD_PLACEHOLDER)

    def watching(self, threshold: float = 0.2) -> bool:
        """Whether replies should also be read in context.

        On while a conversation-level review or a block is inside ``carry_turns`` completed turns,
        while the risk has not decayed below ``threshold``, or while a withheld turn is still in the
        window. It never holds anything by itself; it only decides whether a second read is paid for.
        """
        if self._floor_left > 0 or self.risk >= threshold:
            return True
        return any(t.content == WITHHELD_PLACEHOLDER for t in self.turns)

    # -- risk ---------------------------------------------------------

    @property
    def floor(self) -> Action:
        """The minimum action the next check will resolve to."""
        return self._floor if self._floor_left > 0 else "allow"

    def observe(self, verdict: Verdict) -> None:
        """Fold a verdict into the session's state.

        A degraded verdict is ignored: it reflects an outage, not the conversation.
        """
        if verdict.degraded:
            return
        self.verdicts.append(verdict)
        self.risk = max(self.risk * self.decay, ACTION_RISK.get(verdict.action, 0.0))

        # A conversation-level finding is the one that justifies holding the next turns to a
        # higher standard, because it is about the pattern rather than a single message.
        if verdict.surface == "conversation" and rank(verdict.action) >= rank("review"):
            self._raise("review")
        elif verdict.action == "block":
            self._raise("flag")

    def advance(self) -> None:
        """Call once per completed turn, to let a raised floor expire."""
        if self._floor_left > 0:
            self._floor_left -= 1
            if self._floor_left == 0:
                self._floor = "allow"

    def _raise(self, floor: Action) -> None:
        self._floor = stronger(self._floor if self._floor_left > 0 else "allow", floor)  # type: ignore[assignment]
        self._floor_left = self.carry_turns

    # -- reporting ----------------------------------------------------

    def metadata(self) -> dict[str, Any]:
        """Deployment context worth putting in front of Jev on later turns."""
        # The risk score is deliberately not here: in front of Jev it invites judging the current
        # message by the conversation's past, which is the contamination the session exists to avoid.
        return {
            "conversation_id": self.id,
            "turn_number": len(self.turns) + 1,
        }

    def as_state(self) -> dict[str, Any]:
        """Everything a store has to carry for a conversation to survive the process.

        The verdict log is deliberately not in here. It is in-process observability, and a
        restored session should not claim to have emitted verdicts this process never saw. What
        is here is exactly what changes a later decision: the transcript, the risk, and the floor.

        The keys are the same in both languages, so a session written by one can be read by the
        other.
        """
        return {
            "id": self.id,
            "decay": self.decay,
            "carry_turns": self.carry_turns,
            "max_turns": self.max_turns,
            "turns": [t.as_dict() for t in self.turns],
            "risk": self.risk,
            "floor": self._floor,
            "floor_turns_left": self._floor_left,
        }

    @classmethod
    def from_state(cls, state: Mapping[str, Any]) -> "Session":
        """Rebuild a session from ``as_state``.

        Tolerant of a missing or malformed field, because this state comes back from a store and
        a half-written record should cost one conversation's memory, not the request.
        """
        session = cls(
            id=str(state.get("id", "")),
            decay=float(state.get("decay", 0.5)),
            carry_turns=int(state.get("carry_turns", 2)),
            max_turns=int(state.get("max_turns", 10)),
            risk=float(state.get("risk", 0.0)),
        )
        session.extend(state.get("turns") or ())
        floor = state.get("floor", "allow")
        left = int(state.get("floor_turns_left", 0) or 0)
        # A floor that outlived its counter, or a value no longer in the ladder, is no floor.
        if floor in LADDER and floor != "allow" and left > 0:
            session._floor = floor  # type: ignore[assignment]
            session._floor_left = left
        return session

    def as_dict(self) -> dict[str, Any]:
        return {
            "id": self.id,
            "turns": len(self.turns),
            "risk": round(self.risk, 3),
            "floor": self.floor,
            "floor_turns_left": self._floor_left,
            "checks": len(self.verdicts),
        }
