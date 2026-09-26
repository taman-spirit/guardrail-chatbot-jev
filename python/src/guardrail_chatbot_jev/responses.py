"""Prewritten replies, chosen by the policy pack instead of written by the model.

A verdict says whether content goes out. When it does not, something has to go out in its place,
and for some groups the wording is a legal statement that must be exact. Letting the model write it
invites a wrong document number or a softened claim, so the text lives in the pack's ``responses``
section and this module only selects it:

    responder = Responder(guard.policy, crisis_line="...")
    held = responder.blocking_response([input_verdict], language="vi")
    if held:
        return held
    ...
    return responder.compose(reply, [input_verdict, output_verdict], language="vi")

Each violation group has its own reply. A group can also carry an affirmation that ends every path
where it is involved: the reply that replaces the content, a held-for-review message, and a
delivered answer to a question on the subject all end with the same fixed text.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Any, Iterable, Mapping

from .policy import Policy
from .session import WITHHELD_PLACEHOLDER, Session
from .types import Turn, Verdict, rank

_CJK = re.compile(r"[㐀-鿿]")
_VIETNAMESE = re.compile(
    r"[ăâđêôơưàảãáạằẳẵắặầẩẫấậèẻẽéẹềểễếệìỉĩíịòỏõóọồổỗốộờởỡớợùủũúụừửữứựỳỷỹýỵ]", re.IGNORECASE
)


def detect_language(text: str | None, default: str = "vi") -> str:
    """A cheap guess between Vietnamese, English and Chinese, good enough to pick a reply."""
    if not text:
        return default
    if _CJK.search(text):
        return "zh"
    if _VIETNAMESE.search(text):
        return "vi"
    if re.search(r"[A-Za-z]", text):
        return "en"
    return default


@dataclass(frozen=True, slots=True)
class Choice:
    """Why a reply was chosen, for logs and audits."""

    group: str
    text: str
    affirmed: bool


class Responder:
    """Selects the prewritten reply for a set of verdicts.

    Args:
        policy: A pack with a ``responses`` section, such as ``vietnam-compliance-v1``.
        crisis_line: The number the self-harm reply asks the user to call. Defaults to the pack's
            ``crisis_line_default`` (115 in the Viet Nam pack). Replace it only with a line your
            team has verified: a number that has changed or was never right is worse than none.
    """

    def __init__(self, policy: Policy, *, crisis_line: str | None = None) -> None:
        spec = policy.data.get("responses")
        if not isinstance(spec, Mapping):
            raise ValueError(f"policy {policy.id!r} has no responses section")
        self.spec = spec
        self.policy = policy
        self.languages: tuple[str, ...] = tuple(spec.get("languages") or ("vi",))
        self.default_language: str = str(spec.get("default_language") or self.languages[0])
        self.crisis_line = crisis_line or str(spec.get("crisis_line_default") or "")
        self._groups: dict[str, Mapping[str, Any]] = dict(spec.get("groups") or {})
        self._order: list[str] = list(spec.get("order") or self._groups)
        self._by_category: dict[str, str] = {}
        for name in self._order:
            for category in self._groups[name].get("categories") or ():
                self._by_category.setdefault(category, name)
        self._affirmation: Mapping[str, Any] = spec.get("sovereignty_affirmation") or {}
        self._quiet_after = set(self._affirmation.get("not_after_rules") or ())
        self._affirm_groups = {"sovereignty"}
        _check(self)

    # -- public ---------------------------------------------------------

    def affirmation(self, language: str | None = None) -> str:
        """The fixed sovereignty statement, exactly as written in the pack."""
        return self._text(self._affirmation.get("text") or {}, language)

    def blocking_response(self, verdicts: Iterable[Verdict | None], language: str | None = None) -> str | None:
        """The reply to send instead, when any verdict withholds the content; otherwise ``None``."""
        choice = self.choose(verdicts, language)
        return choice.text if choice is not None and choice.group != "affirmation" else None

    def compose(
        self, reply: str, verdicts: Iterable[Verdict | None], language: str | None = None
    ) -> str:
        """What to actually send: the prewritten reply if anything was withheld, else the model's
        reply, followed by the affirmation when the turn touched sovereignty."""
        choice = self.choose(verdicts, language)
        if choice is None:
            return reply
        if choice.group == "affirmation":
            return f"{reply.rstrip()}\n\n{choice.text}"
        return choice.text

    def choose(self, verdicts: Iterable[Verdict | None], language: str | None = None) -> Choice | None:
        """The decision behind ``compose``, with the group it came from."""
        present = [v for v in verdicts if v is not None]
        affirm = any(self._touches_sovereignty(v) for v in present)
        held = [v for v in present if not v.deliverable]

        if not held:
            if affirm:
                return Choice("affirmation", self.affirmation(language), True)
            return None

        # The strongest verdict answers. At equal rank a real verdict beats one from an outage, and
        # the crisis route beats everything, so a person at risk never gets a maintenance notice.
        verdict = max(
            held,
            key=lambda v: (rank(v.action), v.route == "crisis_support", not v.degraded),
        )
        if verdict.degraded:
            group, text = "unavailable", self._text(self.spec.get("unavailable") or {}, language)
        elif verdict.route == "crisis_support":
            group = "self_harm"
            text = self._group_text(group, language)
        else:
            group = self._group_for(held)
            if verdict.route == "human_review":
                # Held for a person, not refused: telling the user they broke the law before
                # anyone has looked would be the wrong message for a borderline case.
                group, text = "review", self._text(self.spec.get("review") or {}, language)
            else:
                text = self._group_text(group, language)
            # Self-harm that reached its block band without leading the turn, beside a request to
            # hurt others, still gets the number to call: the reply names what stopped the content,
            # and the line is there in case the person is also at risk.
            footer = self._crisis_footer(held, language)
            if footer:
                text = f"{text}\n\n{footer}"

        # A group with an affirmation ends every path it is part of, including a held review.
        if affirm and group != "self_harm":
            text = f"{text}\n\n{self.affirmation(language)}"
        return Choice(group, text, affirm and group != "self_harm")

    def model_history(self, session: Session, language: str | None = None) -> tuple[Turn, ...]:
        """The session's transcript for the chat model, with each withheld turn told rather than
        dropped.

        A withheld user message becomes a note naming its group, followed by the reply the user was
        shown; a withheld reply becomes the reply the user was shown. ``Session.model_history`` drops
        withheld turns, so after a refusal the model meets "do it" or "my first request" with nothing
        before it, and guesses. The note names the group, never the text, so the model knows what it
        declined without being able to act on it.
        """
        out: list[Turn] = []
        turns = session.turns
        for i, turn in enumerate(turns):
            if turn.content != WITHHELD_PLACEHOLDER:
                out.append(Turn(role=turn.role, content=turn.content))
                continue
            held = session.held_verdict(i)
            choice = self.choose([held] if held is not None else [], language)
            if choice is None or choice.group == "affirmation":
                # A turn recorded without its verdict, or one that is not withheld after all.
                choice = Choice("general", self._group_text("general", language), False)
            if turn.role != "user":
                out.append(Turn(role=turn.role, content=choice.text))
                continue
            withheld = self.spec.get("withheld") or {}
            label = self._text((withheld.get("labels") or {}).get(choice.group) or {}, language)
            note = self._text(withheld.get("note") or {}, language).replace("{label}", label)
            out.append(Turn(role="user", content=note))
            if i + 1 >= len(turns) or turns[i + 1].role != "assistant":
                out.append(Turn(role="assistant", content=choice.text))
        return tuple(out)

    # -- internals ------------------------------------------------------

    def _crisis_footer(self, held: list[Verdict], language: str | None) -> str:
        """The crisis line to add to a reply that is not the crisis reply.

        Added when a held verdict carries a self-harm probability at or over its own block band. The
        band is read from the probability, not the finding's action: a rule that hardens every
        finding on a request for capability lifts a self-harm answer of 0.15 to block on "how do I
        bring down a bridge", and a crisis line under every refusal would be noise.
        """
        self_harm = set(self._groups["self_harm"].get("categories") or ())
        for verdict in held:
            for finding in verdict.findings:
                if finding.category not in self_harm or rank(finding.action) < rank("flag"):
                    continue
                category = self.policy.categories.get(finding.category)
                bands = category.threshold(verdict.surface) if category is not None else {}
                if "block" in bands and finding.probability >= bands["block"]:
                    footer = self._text(self.spec.get("crisis_footer") or {}, language)
                    return footer.replace("{crisis_line}", self.crisis_line)
        return ""

    def _group_for(self, verdicts: list[Verdict]) -> str:
        """The group of the finding that caused the hold, with ``order`` only breaking ties.

        A fake-news block that also carries a minor flag elsewhere is answered as fake news: the
        reply should name what actually stopped the content.
        """
        findings = [f for v in verdicts for f in v.findings if rank(f.action) >= rank("flag")]
        if findings:
            top = max(rank(f.action) for f in findings)
            fired = {f.category for f in findings if rank(f.action) == top}
            for name in self._order:
                # The crisis reply comes only from the crisis route, which is set when self-harm leads.
                if name == "self_harm":
                    continue
                if fired.intersection(self._groups[name].get("categories") or ()):
                    return name
        for name in self._order:
            if "*" in (self._groups[name].get("categories") or ()):
                return name
        return self._order[-1]

    def _touches_sovereignty(self, verdict: Verdict) -> bool:
        # A finding a neutral-mention rule settled is not a claim, so it does not earn the statement.
        quiet = bool(self._quiet_after.intersection(verdict.applied_rules))
        for finding in () if quiet else verdict.findings:
            if self._by_category.get(finding.category) in self._affirm_groups and rank(finding.action) >= rank("flag"):
                return True
        signal = self._affirmation.get("trigger_signal")
        value = verdict.signals.get(signal) if signal else None
        threshold = float(self._affirmation.get("trigger_value", 0.6))
        return isinstance(value, (int, float)) and not isinstance(value, bool) and value >= threshold

    def _group_text(self, group: str, language: str | None) -> str:
        spec = self._groups[group]
        return self._text(spec.get("text") or {}, language).replace("{crisis_line}", self.crisis_line)

    def _text(self, texts: Mapping[str, str], language: str | None) -> str:
        for lang in (language, self.default_language, *self.languages):
            if lang and texts.get(lang):
                return str(texts[lang])
        return ""


def _check(responder: Responder) -> None:
    """Refuse a pack whose replies would leave a user without an answer in some language."""
    spec = responder.spec
    missing: list[str] = []
    for name in responder._order:
        if name not in responder._groups:
            missing.append(f"group {name!r} is in order but not defined")
            continue
        for lang in responder.languages:
            if not (responder._groups[name].get("text") or {}).get(lang):
                missing.append(f"{name}.{lang}")
    if responder.default_language not in responder.languages:
        missing.append(f"default_language {responder.default_language!r} is not in languages")
    if "self_harm" not in responder._groups:
        # The crisis route answers from this group directly, whether or not it is in order.
        missing.append("group 'self_harm' is not defined")
    if "general" not in responder._groups:
        missing.append("group 'general' is not defined")
    withheld = spec.get("withheld") or {}
    labelled = sorted({*responder._order, "self_harm", "general", "review", "unavailable"})
    for lang in responder.languages:
        if not (spec.get("crisis_footer") or {}).get(lang):
            missing.append(f"crisis_footer.{lang}")
        if not (withheld.get("note") or {}).get(lang):
            missing.append(f"withheld.note.{lang}")
        for name in labelled:
            if not ((withheld.get("labels") or {}).get(name) or {}).get(lang):
                missing.append(f"withheld.labels.{name}.{lang}")
    for key in ("review", "unavailable"):
        for lang in responder.languages:
            if not (spec.get(key) or {}).get(lang):
                missing.append(f"{key}.{lang}")
    for name, group in responder._groups.items():
        if any("{crisis_line}" in text for text in (group.get("text") or {}).values()) and not responder.crisis_line:
            missing.append(f"{name} needs a crisis line: set crisis_line_default or pass crisis_line")
    for lang in responder.languages:
        if responder._affirmation and not (responder._affirmation.get("text") or {}).get(lang):
            missing.append(f"sovereignty_affirmation.{lang}")
    if missing:
        raise ValueError("responses section is incomplete: " + ", ".join(missing))


__all__ = ["Choice", "Responder", "detect_language"]
