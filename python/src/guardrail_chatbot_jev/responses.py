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
from .types import Verdict, rank

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
        crisis_line: A support number your team has verified for the country you serve. Without
            one the self-harm reply points to emergency services only; a number that has changed
            or was never right is worse than none.
    """

    def __init__(self, policy: Policy, *, crisis_line: str | None = None) -> None:
        spec = policy.data.get("responses")
        if not isinstance(spec, Mapping):
            raise ValueError(f"policy {policy.id!r} has no responses section")
        self.spec = spec
        self.languages: tuple[str, ...] = tuple(spec.get("languages") or ("vi",))
        self.default_language: str = str(spec.get("default_language") or self.languages[0])
        self.crisis_line = crisis_line
        self._groups: dict[str, Mapping[str, Any]] = dict(spec.get("groups") or {})
        self._order: list[str] = list(spec.get("order") or self._groups)
        self._by_category: dict[str, str] = {}
        for name in self._order:
            for category in self._groups[name].get("categories") or ():
                self._by_category.setdefault(category, name)
        self._affirmation: Mapping[str, Any] = spec.get("sovereignty_affirmation") or {}
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

        verdict = max(held, key=lambda v: rank(v.action))
        if verdict.degraded:
            group, text = "unavailable", self._text(self.spec.get("unavailable") or {}, language)
        elif verdict.route == "crisis_support":
            group = "self_harm"
            text = self._group_text(group, language)
        else:
            group = self._group_for(held)
            if verdict.route == "human_review" and group != "self_harm":
                # Held for a person, not refused: telling the user they broke the law before
                # anyone has looked would be the wrong message for a borderline case.
                group, text = "review", self._text(self.spec.get("review") or {}, language)
            else:
                text = self._group_text(group, language)

        # A group with an affirmation ends every path it is part of, including a held review.
        if affirm and group != "self_harm":
            text = f"{text}\n\n{self.affirmation(language)}"
        return Choice(group, text, affirm and group != "self_harm")

    # -- internals ------------------------------------------------------

    def _group_for(self, verdicts: list[Verdict]) -> str:
        fired = {f.category for v in verdicts for f in v.findings if rank(f.action) >= rank("flag")}
        for name in self._order:
            categories = self._groups[name].get("categories") or ()
            if "*" in categories or fired.intersection(categories):
                return name
        return self._order[-1]

    def _touches_sovereignty(self, verdict: Verdict) -> bool:
        for finding in verdict.findings:
            if self._by_category.get(finding.category) in self._affirm_groups and rank(finding.action) >= rank("flag"):
                return True
        signal = self._affirmation.get("trigger_signal")
        value = verdict.signals.get(signal) if signal else None
        threshold = float(self._affirmation.get("trigger_value", 0.6))
        return isinstance(value, (int, float)) and value >= threshold

    def _group_text(self, group: str, language: str | None) -> str:
        spec = self._groups[group]
        text = self._text(spec.get("text") or {}, language)
        if "{crisis_line}" in text:
            line = ""
            if self.crisis_line:
                line = self._text(spec.get("crisis_line") or {}, language).format(number=self.crisis_line)
            text = text.replace("{crisis_line}", line)
        return text

    def _text(self, texts: Mapping[str, str], language: str | None) -> str:
        lang = language if language in texts else self.default_language
        return str(texts.get(lang) or next(iter(texts.values()), ""))


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
    for key in ("review", "unavailable"):
        for lang in responder.languages:
            if not (spec.get(key) or {}).get(lang):
                missing.append(f"{key}.{lang}")
    for lang in responder.languages:
        if responder._affirmation and not (responder._affirmation.get("text") or {}).get(lang):
            missing.append(f"sovereignty_affirmation.{lang}")
    if missing:
        raise ValueError("responses section is incomplete: " + ", ".join(missing))


__all__ = ["Choice", "Responder", "detect_language"]
