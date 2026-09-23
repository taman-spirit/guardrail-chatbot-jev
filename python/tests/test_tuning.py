"""Offline tuning: replay, scoring, overrides, sweeps and separation."""

from __future__ import annotations

import json

import pytest
from conftest import answers, hazard, noul, score as score_answer

from guardrail_chatbot_jev import Guard, Policy, RecordedTransport, RecordingTransport
from guardrail_chatbot_jev.tuning import (
    Record,
    grid,
    load_records,
    override,
    probability_of,
    replay,
    score,
    separation,
    sweep,
)


def record(case_id: str, expected: str, **parts) -> Record:  # noqa: ANN003
    return Record(surface="input", answers=answers(**parts), expected=expected, id=case_id)  # type: ignore[arg-type]


@pytest.fixture()
def labelled() -> list[Record]:
    """A small set with one of each outcome, including a miss and an over-block."""
    return [
        record("safe-1", "allow"),
        record("safe-2", "allow", hazard=hazard({"ncr": 0.02})),
        record("over-1", "allow", hazard=hazard({"hte": 0.62}), actionability=score_answer(2.0)),
        record("miss-1", "block", hazard=hazard({"ncr": 0.2})),
        record("hit-1", "block", hazard=hazard({"iwp": 0.6}), s_iwp=noul(0.85), actionability=score_answer(2.0)),
        record("flag-1", "flag", hazard=hazard({"vcr": 0.16})),
    ]


# -- recording ---------------------------------------------------------


def test_recording_transport_keeps_the_exchange(policy: Policy) -> None:
    sink: list[dict] = []
    inner = RecordedTransport({"hazard": {"type": "choice", "choice": "none", "confidence": 0.9, "probabilities": {"none": 0.9}}})
    Guard(policy, transport=RecordingTransport(inner, sink)).check_input("xin chào")

    assert len(sink) == 1
    assert sink[0]["state"]["user_message"] == "xin chào"
    assert "hazard" in sink[0]["answers"]
    assert sink[0]["usage"] == {"input_tokens": 0, "output_tokens": 0}


def test_a_recording_replays_to_the_same_verdict(policy: Policy) -> None:
    """The point of recording: the replay must agree with the live run it came from."""
    raw = {"hazard": {"type": "choice", "choice": "ncr", "confidence": 0.88, "probabilities": {"ncr": 0.55, "none": 0.45}}}
    sink: list[dict] = []
    live = Guard(policy, transport=RecordingTransport(RecordedTransport(raw), sink)).check_input("...")

    replayed = replay(policy, [Record(surface="input", answers=sink[0]["answers"])])[0]

    assert replayed.action == live.action
    assert replayed.route == live.route
    assert replayed.categories == live.categories


def test_records_load_from_jsonl(tmp_path) -> None:  # noqa: ANN001
    path = tmp_path / "answers.jsonl"
    path.write_text(
        json.dumps(
            {
                "id": "c1",
                "surface": "output",
                "expected_action": "review",
                "answers": {"hazard": {"type": "choice", "choice": "prv", "confidence": 0.9, "probabilities": {"prv": 0.4}}},
            }
        )
        + "\n",
        encoding="utf-8",
    )
    loaded = load_records(path)
    assert loaded[0].id == "c1"
    assert loaded[0].surface == "output"
    assert loaded[0].expected == "review"


# -- scoring -----------------------------------------------------------


def test_score_separates_the_two_kinds_of_error(policy: Policy, labelled: list[Record]) -> None:
    report = score(labelled, replay(policy, labelled))

    assert report.total == 6
    assert "miss-1" in report.critical_misses, "labelled block but would be delivered"
    assert "over-1" in report.over_blocks, "labelled allow but would be blocked"
    assert report.under >= 1 and report.over >= 1
    assert 0.0 <= report.review_rate <= 1.0


def test_confusion_matrix_is_keyed_by_label(policy: Policy, labelled: list[Record]) -> None:
    report = score(labelled, replay(policy, labelled))
    assert set(report.confusion) <= {"allow", "flag", "review", "block"}
    assert sum(sum(row.values()) for row in report.confusion.values()) == 6


def test_unlabelled_records_are_counted_but_not_scored(policy: Policy) -> None:
    report = score([Record("input", answers())], replay(policy, [Record("input", answers())]))
    assert report.total == 1
    assert report.exact == 0
    assert report.confusion == {}


# -- overrides ---------------------------------------------------------


def test_override_changes_the_outcome(policy: Policy) -> None:
    records = [record("m", "block", hazard=hazard({"ncr": 0.2}), actionability=score_answer(2.0))]
    assert replay(policy, records)[0].action != "block"

    loosened = override(policy, {"ncr.input.block": 0.15})
    assert replay(loosened, records)[0].action == "block"


def test_override_keeps_the_bands_ordered(policy: Policy) -> None:
    """An override that would invert the bands must still produce a loadable policy."""
    tightened = override(policy, {"ncr.input.block": 0.1})
    bands = tightened.categories["ncr"].threshold("input")
    assert bands["block"] >= bands["review"] >= bands["flag"]


def test_override_leaves_the_original_alone(policy: Policy) -> None:
    before = dict(policy.categories["ncr"].threshold("input"))
    override(policy, {"ncr.input.block": 0.11})
    assert policy.categories["ncr"].threshold("input") == before


def test_a_bad_axis_is_rejected(policy: Policy) -> None:
    with pytest.raises(ValueError, match="category.surface.band"):
        override(policy, {"ncr.input": 0.5})
    with pytest.raises(KeyError):
        override(policy, {"nope.input.block": 0.5})


# -- sweep and separation ----------------------------------------------


def test_sweep_is_monotonic_in_the_right_direction(policy: Policy, labelled: list[Record]) -> None:
    """Lowering a block threshold can only block more, never less."""
    rows = sweep(policy, labelled, "ncr.input.block", grid(0.1, 0.6, 0.1))
    blocked = [report.actions.get("block", 0) for _, report in rows]
    assert blocked == sorted(blocked), "raising the threshold must not increase blocks"


def categorised(case_id: str, expected: str, category: str | None, **parts) -> Record:  # noqa: ANN003
    return Record(
        surface="input",
        answers=answers(**parts),
        expected=expected,  # type: ignore[arg-type]
        expected_category=category,
        id=case_id,
    )


def test_separation_spots_an_overlap(policy: Policy) -> None:
    overlapping = [
        categorised("a", "block", "ncr", hazard=hazard({"ncr": 0.3})),
        categorised("b", "allow", None, hazard=hazard({"ncr": 0.35})),
    ]
    result = separation(policy, overlapping, "ncr", "input")
    assert result["basis"] == "category"
    assert result["separated"] is False
    assert result["should_fire"]["n"] == 1
    assert result["should_not_fire"]["n"] == 1


def test_separation_spots_a_clean_split(policy: Policy) -> None:
    clean = [
        categorised("a", "block", "ncr", hazard=hazard({"ncr": 0.8})),
        categorised("b", "allow", None, hazard=hazard({"ncr": 0.02})),
    ]
    assert separation(policy, clean, "ncr", "input")["separated"] is True


def test_separation_counts_only_the_cases_a_category_owns(policy: Policy) -> None:
    """A self-harm case must not be held against the fraud category.

    Splitting by the action label alone does exactly that, and makes every category look like an
    overlap no matter how well it actually separates.
    """
    records = [
        categorised("fraud", "block", "ncr", hazard=hazard({"ncr": 0.6})),
        categorised("harm", "block", "ssh", hazard=hazard({"ssh": 0.6})),
        categorised("safe", "allow", None),
    ]
    by_category = separation(policy, records, "ncr", "input")
    assert by_category["should_fire"]["n"] == 1
    assert by_category["separated"] is True

    unlabelled = [Record("input", r.answers, r.expected, None, r.id) for r in records]
    by_action = separation(policy, unlabelled, "ncr", "input")
    assert by_action["basis"] == "action"
    assert by_action["should_fire"]["n"] == 2, "the self-harm case is wrongly counted as ncr's"
    assert by_action["separated"] is False


def test_records_carry_the_expected_category(tmp_path) -> None:  # noqa: ANN001
    path = tmp_path / "a.jsonl"
    path.write_text(
        json.dumps({"id": "x", "surface": "input", "expected_action": "block", "expected_category": "iwp", "answers": {}})
        + "\n",
        encoding="utf-8",
    )
    assert load_records(path)[0].expected_category == "iwp"


def test_the_labelled_sets_use_known_categories(policy: Policy) -> None:
    """A typo in expected_category would silently make a category look like it never fires."""
    import pathlib as _pathlib

    root = _pathlib.Path(__file__).resolve().parents[2] / "examples"
    # A labelled set for a derived pack is judged against that pack's categories.
    packs = {"cases-vietnam.jsonl": Policy.bundled("vietnam-compliance-v1")}
    for path in sorted(root.glob("*.jsonl")):
        known = packs.get(path.name, policy).categories
        for line in path.read_text("utf-8").splitlines():
            if not line.strip():
                continue
            case = json.loads(line)
            category = case.get("expected_category")
            if category is not None:
                assert category in known, f"{path.name}: {case['id']} -> {category!r}"


def test_probability_takes_the_higher_of_choice_and_sentinel() -> None:
    r = Record("input", answers(hazard=hazard({"cse": 0.1}), s_cse=noul(0.7)))
    assert probability_of(r, "cse") == pytest.approx(0.7)
    assert probability_of(r, "ncr") == 0.0


def test_grid_is_inclusive() -> None:
    assert grid(0.1, 0.3, 0.1) == [0.1, 0.2, 0.3]
    with pytest.raises(ValueError):
        grid(0.1, 0.3, 0)
