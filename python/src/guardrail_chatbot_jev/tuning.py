"""Offline threshold tuning, replayed against recorded answers.

Calling Jev is the expensive part of calibration: tokens, rate-limit budget, and the labelled set
itself, which is the scarcest input of all. ``decide`` is a pure function, so once the answers for
a case are on disk, every later question about thresholds can be answered without a network call.

The flow is: run the labelled set once with a ``RecordingTransport``, then sweep here.
"""

from __future__ import annotations

import copy
import json
from dataclasses import dataclass, field
from pathlib import Path
from statistics import median
from typing import Any, Iterable, Mapping, Sequence

from .decide import decide
from .policy import Policy
from .questions import HAZARD, NONE_LABEL, SENTINEL_PREFIX
from .types import Action, LADDER, Surface, Verdict, rank

BANDS = ("block", "review", "flag")


@dataclass(frozen=True, slots=True)
class Record:
    """One recorded exchange, plus whatever label the case carried."""

    surface: Surface
    answers: Mapping[str, Mapping[str, Any]]
    expected: Action | None = None
    #: The hazard the case is meant to exercise, when the labelled set says. Separation needs it:
    #: without it there is no way to tell a case this category should have caught from one that
    #: belongs to a different category entirely.
    expected_category: str | None = None
    id: str = ""
    state: Any = None

    @classmethod
    def from_json(cls, row: Mapping[str, Any], *, expect_field: str = "expected_action") -> "Record":
        case = row.get("input") or {}
        expected = row.get(expect_field) or case.get(expect_field)
        category = row.get("expected_category") or case.get("expected_category")
        return cls(
            surface=row.get("surface") or "input",  # type: ignore[arg-type]
            answers=row.get("answers") or {},
            expected=expected,
            expected_category=category,
            id=str(row.get("id") or case.get("id") or ""),
            state=row.get("state"),
        )


def load_records(path: str | Path, *, expect_field: str = "expected_action") -> list[Record]:
    """Read a JSONL file written by ``check.py --record``."""
    rows = [json.loads(line) for line in Path(path).read_text("utf-8").splitlines() if line.strip()]
    return [Record.from_json(row, expect_field=expect_field) for row in rows]


def replay(policy: Policy, records: Sequence[Record]) -> list[Verdict]:
    """Re-decide every record under a policy. No network, no model."""
    return [decide(policy, r.surface, r.answers) for r in records]


@dataclass
class Report:
    """How a policy scores against a labelled set.

    Exact match is the least interesting number here. A guardrail is judged on its two kinds of
    error separately, because they are not interchangeable: letting harmful content through is a
    safety failure, blocking ordinary users is a product failure, and the review queue is what a
    team has to staff.
    """

    total: int = 0
    exact: int = 0
    under: int = 0
    over: int = 0
    critical_misses: list[str] = field(default_factory=list)
    over_blocks: list[str] = field(default_factory=list)
    confusion: dict[str, dict[str, int]] = field(default_factory=dict)
    actions: dict[str, int] = field(default_factory=dict)
    degraded: int = 0

    @property
    def exact_rate(self) -> float:
        return self.exact / self.total if self.total else 0.0

    @property
    def review_rate(self) -> float:
        return self.actions.get("review", 0) / self.total if self.total else 0.0

    @property
    def block_rate(self) -> float:
        return self.actions.get("block", 0) / self.total if self.total else 0.0

    def as_dict(self) -> dict[str, Any]:
        return {
            "total": self.total,
            "exact": self.exact,
            "exact_rate": round(self.exact_rate, 4),
            "under_enforced": self.under,
            "over_enforced": self.over,
            "critical_misses": self.critical_misses,
            "over_blocks": self.over_blocks,
            "review_rate": round(self.review_rate, 4),
            "block_rate": round(self.block_rate, 4),
            "confusion": self.confusion,
            "degraded": self.degraded,
        }


def score(records: Sequence[Record], verdicts: Sequence[Verdict]) -> Report:
    """Compare replayed verdicts against the labels."""
    report = Report()
    for record, verdict in zip(records, verdicts):
        report.total += 1
        report.actions[verdict.action] = report.actions.get(verdict.action, 0) + 1
        if verdict.degraded:
            report.degraded += 1
        if record.expected is None:
            continue
        row = report.confusion.setdefault(record.expected, {})
        row[verdict.action] = row.get(verdict.action, 0) + 1

        got, want = rank(verdict.action), rank(record.expected)
        if got == want:
            report.exact += 1
        elif got < want:
            report.under += 1
            # The label said hold it and the policy would have delivered it: the failure that
            # matters, listed by name rather than counted.
            if record.expected == "block" and verdict.action in ("allow", "flag"):
                report.critical_misses.append(record.id or "?")
        else:
            report.over += 1
            if record.expected == "allow" and verdict.action == "block":
                report.over_blocks.append(record.id or "?")
    return report


# -- threshold surgery -------------------------------------------------


def override(policy: Policy, changes: Mapping[str, float]) -> Policy:
    """A copy of a policy with thresholds replaced.

    Keys are ``category.surface.band``, for example ``prv.output.review``. Use ``default`` as the
    surface to change the fallback band.
    """
    data = copy.deepcopy(policy.data)
    for axis, value in changes.items():
        category, surface, band = _parse_axis(axis)
        categories = data.get("categories") or {}
        if category not in categories:
            raise KeyError(f"unknown category {category!r} in {axis!r}")
        thresholds = categories[category].setdefault("thresholds", {})
        bands = dict(thresholds.get(surface) or thresholds.get("default") or {})
        if not bands:
            raise KeyError(f"category {category!r} has no thresholds to override")
        bands[band] = value
        thresholds[surface] = _ordered(bands)
    return Policy(data)


def sweep(
    policy: Policy,
    records: Sequence[Record],
    axis: str,
    values: Iterable[float],
) -> list[tuple[float, Report]]:
    """Score the set once per candidate value of one threshold."""
    out: list[tuple[float, Report]] = []
    for value in values:
        candidate = override(policy, {axis: value})
        out.append((value, score(records, replay(candidate, records))))
    return out


def separation(
    policy: Policy, records: Sequence[Record], category: str, surface: Surface
) -> dict[str, Any]:
    """How far apart a category's probabilities are between the cases it should and should not catch.

    This is the question a threshold actually answers. Where the two groups overlap, no number
    separates them and the fix is the category's description, which is the text Jev reads.

    The split is by ``expected_category`` when the labelled set carries one. Falling back to the
    action label is much weaker: it counts every non-allow case as one this category should have
    caught, so a self-harm case would be held against the fraud category and everything would look
    like an overlap. The result says which basis was used.
    """
    relevant = [r for r in records if r.surface == surface and r.expected is not None]
    basis = "category" if any(r.expected_category for r in relevant) else "action"

    should: list[float] = []
    should_not: list[float] = []
    for record in relevant:
        probability = probability_of(record, category)
        if basis == "category":
            fires = record.expected_category == category
        else:
            fires = rank(record.expected) >= rank("review")  # type: ignore[arg-type]
        (should if fires else should_not).append(probability)

    bands = policy.categories[category].threshold(surface) if category in policy.categories else {}
    return {
        "category": category,
        "surface": surface,
        "basis": basis,
        "thresholds": dict(bands),
        "should_fire": _describe(should),
        "should_not_fire": _describe(should_not),
        "separated": bool(should and should_not and min(should) > max(should_not)),
    }


def probability_of(record: Record, category: str) -> float:
    """The probability a replay would use for a category: the higher of choice and sentinel."""
    hazard = record.answers.get(HAZARD) or {}
    value = float((hazard.get("probabilities") or {}).get(category, 0.0)) if category != NONE_LABEL else 0.0
    sentinel = record.answers.get(SENTINEL_PREFIX + category)
    if sentinel:
        value = max(value, float(sentinel.get("noul", 0.0)))
    return value


def grid(start: float, stop: float, step: float) -> list[float]:
    """Inclusive float range, rounded so the values print cleanly."""
    if step <= 0:
        raise ValueError("step must be positive")
    values: list[float] = []
    current = start
    while current <= stop + 1e-9:
        values.append(round(current, 4))
        current += step
    return values


# -- internals ---------------------------------------------------------


def _parse_axis(axis: str) -> tuple[str, str, str]:
    parts = axis.split(".")
    if len(parts) != 3 or parts[2] not in BANDS:
        raise ValueError(f"axis must be category.surface.band with band in {BANDS}, got {axis!r}")
    return parts[0], parts[1], parts[2]


def _ordered(bands: Mapping[str, float]) -> dict[str, float]:
    """Keep block >= review >= flag, so an override cannot produce a policy that will not load."""
    block = float(bands["block"])
    review = min(float(bands["review"]), block)
    flag = min(float(bands["flag"]), review)
    return {"block": block, "review": review, "flag": flag}


def _describe(values: Sequence[float]) -> dict[str, Any]:
    if not values:
        return {"n": 0}
    ordered = sorted(values)
    return {
        "n": len(ordered),
        "min": round(ordered[0], 4),
        "p25": round(ordered[len(ordered) // 4], 4),
        "median": round(median(ordered), 4),
        "p75": round(ordered[(3 * len(ordered)) // 4], 4),
        "max": round(ordered[-1], 4),
    }


__all__ = [
    "BANDS",
    "Record",
    "Report",
    "grid",
    "load_records",
    "override",
    "probability_of",
    "replay",
    "score",
    "separation",
    "sweep",
]
