#!/usr/bin/env python3
"""Tune thresholds against recorded answers, without calling Jev.

Record a run once:

    python3 skill/guardrail-chatbot-jev/scripts/check.py --surface input \
        --jsonl examples/cases-input.jsonl --record calibration/input.answers.jsonl

Then ask as many questions as you like, offline:

    scripts/sweep.py report     calibration/input.answers.jsonl
    scripts/sweep.py separation calibration/input.answers.jsonl --category prv
    scripts/sweep.py sweep      calibration/input.answers.jsonl --axis prv.input.review \
        --from 0.2 --to 0.7 --step 0.05
    scripts/sweep.py report     calibration/input.answers.jsonl --set prv.input.review=0.45

Start with `separation`. A threshold can only help when the cases that should fire and the ones
that should not produce different probabilities; when they overlap, the fix is the category's
description, which is the text Jev reads, not its number.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any

REPO = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(REPO / "python" / "src"))

from guardrail_chatbot_jev import Policy  # noqa: E402
from guardrail_chatbot_jev.types import LADDER  # noqa: E402
from guardrail_chatbot_jev.tuning import (  # noqa: E402
    Record,
    Report,
    grid,
    load_records,
    override,
    probability_of,
    replay,
    score,
    separation,
    sweep,
)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument("command", choices=("report", "sweep", "separation"))
    parser.add_argument("records", type=Path, nargs="+", help="JSONL files from check.py --record")
    parser.add_argument("--policy", help="Pack name or path; defaults to the bundled standard-v1.")
    parser.add_argument(
        "--set",
        action="append",
        default=[],
        metavar="category.surface.band=VALUE",
        help="Override a threshold before scoring; repeatable.",
    )
    parser.add_argument("--axis", help="sweep: the threshold to vary, as category.surface.band")
    parser.add_argument("--from", dest="start", type=float, default=0.05)
    parser.add_argument("--to", dest="stop", type=float, default=0.95)
    parser.add_argument("--step", type=float, default=0.05)
    parser.add_argument("--category", help="separation: which category to look at")
    parser.add_argument("--surface", default=None, help="separation: which surface; default: from the records")
    parser.add_argument("--json", action="store_true", help="Machine-readable output on stdout.")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)

    records: list[Record] = []
    for path in args.records:
        records.extend(load_records(path))
    if not records:
        print("no records; run check.py --record first", file=sys.stderr)
        return 2

    policy = Policy.load(args.policy)
    if args.set:
        policy = override(policy, dict(_parse_set(item) for item in args.set))

    if args.command == "report":
        report = score(records, replay(policy, records))
        _emit(report.as_dict(), args.json, lambda: _print_report(report, records, policy))
        return 0 if not report.critical_misses else 1

    if args.command == "sweep":
        if not args.axis:
            print("sweep needs --axis category.surface.band", file=sys.stderr)
            return 2
        rows = sweep(policy, records, args.axis, grid(args.start, args.stop, args.step))
        payload = [{"value": v, **r.as_dict()} for v, r in rows]
        _emit(payload, args.json, lambda: _print_sweep(args.axis, rows))
        return 0

    surface = args.surface or records[0].surface
    categories = [args.category] if args.category else _categories_seen(records, policy)
    payload = [separation(policy, records, c, surface) for c in categories if c in policy.categories]
    _emit(payload, args.json, lambda: _print_separation(payload))
    return 0


# -- printing ----------------------------------------------------------


def _print_report(report: Report, records: list[Record], policy: Policy) -> None:
    print(f"policy      {policy.id}@{policy.version}")
    print(f"cases       {report.total} ({sum(1 for r in records if r.expected)} labelled)")
    print(f"exact       {report.exact}/{report.total}  ({report.exact_rate:.0%})")
    print(f"under       {report.under:<4} too permissive against the label")
    print(f"over        {report.over:<4} too restrictive against the label")
    print(f"review rate {report.review_rate:.0%}   <- the queue a team has to staff")
    print(f"block rate  {report.block_rate:.0%}")
    if report.degraded:
        print(f"DEGRADED    {report.degraded} records carry no verdict at all")

    if report.critical_misses:
        print("\ncritical misses (labelled block, would be delivered):")
        for case_id in report.critical_misses:
            print(f"  {case_id}")
    if report.over_blocks:
        print("\nover-blocks (labelled allow, would be blocked):")
        for case_id in report.over_blocks:
            print(f"  {case_id}")

    if report.confusion:
        # Both axes in ladder order, so the diagonal reads as agreement and everything below it
        # is the permissive half.
        order = [a for a in LADDER if a in report.confusion or any(a in r for r in report.confusion.values())]
        print("\nexpected \\ got")
        print("            " + "".join(f"{a:>8}" for a in order))
        for expected in order:
            row = report.confusion.get(expected)
            if row is None:
                continue
            print(f"  {expected:<10}" + "".join(f"{row.get(a, 0):>8}" for a in order))


def _print_sweep(axis: str, rows: list[tuple[float, Report]]) -> None:
    print(f"sweeping {axis}\n")
    print(f"{'value':>7}  {'exact':>7}  {'under':>6}  {'over':>5}  {'crit':>5}  {'review':>7}  {'block':>6}")
    for value, report in rows:
        print(
            f"{value:>7.2f}  {report.exact_rate:>6.0%}  {report.under:>6}  {report.over:>5}  "
            f"{len(report.critical_misses):>5}  {report.review_rate:>6.0%}  {report.block_rate:>5.0%}"
        )
    print("\nLowering a threshold raises recall and the review queue together.")
    print("Pick by what the queue can carry, not by the exact-match column.")


def _print_separation(rows: list[dict[str, Any]]) -> None:
    if rows and rows[0]["basis"] == "action":
        print(
            "No case carries expected_category, so this splits by the action label instead.\n"
            "That counts every non-allow case against every category, which makes overlap the\n"
            "default answer. Add expected_category to the labelled set for a usable reading.\n"
        )
    for row in rows:
        should, should_not = row["should_fire"], row["should_not_fire"]
        if not should["n"]:
            continue
        mark = "separated" if row["separated"] else "OVERLAP"
        print(f"\n{row['category']} on {row['surface']}  [{mark}]")
        print(f"  thresholds        {row['thresholds']}")
        print(f"  should fire       {_fmt(should)}")
        print(f"  should not fire   {_fmt(should_not)}")
        if not row["separated"] and should_not["n"]:
            gap = should["min"] - should_not["max"]
            print(f"  overlap of {abs(gap):.3f}; no threshold separates these.")
            print("  the fix is the category description, which is the text Jev reads")


def _fmt(stats: dict[str, Any]) -> str:
    if not stats["n"]:
        return "no cases"
    return (
        f"n={stats['n']:<3} min={stats['min']:.3f} p25={stats['p25']:.3f} "
        f"med={stats['median']:.3f} p75={stats['p75']:.3f} max={stats['max']:.3f}"
    )


def _categories_seen(records: list[Record], policy: Policy) -> list[str]:
    """Categories that ever showed a non-trivial probability, most active first."""
    peak: dict[str, float] = {}
    for record in records:
        for category in policy.categories:
            value = probability_of(record, category)
            if value > peak.get(category, 0.0):
                peak[category] = value
    return [c for c, v in sorted(peak.items(), key=lambda kv: -kv[1]) if v >= 0.02]


def _emit(payload: Any, as_json: bool, printer: Any) -> None:
    if as_json:
        print(json.dumps(payload, ensure_ascii=False, indent=2))
    else:
        printer()


def _parse_set(item: str) -> tuple[str, float]:
    axis, _, value = item.partition("=")
    if not value:
        raise SystemExit(f"--set needs category.surface.band=VALUE, got {item!r}")
    return axis, float(value)


if __name__ == "__main__":
    raise SystemExit(main())
