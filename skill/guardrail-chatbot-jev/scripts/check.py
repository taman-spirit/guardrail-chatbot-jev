#!/usr/bin/env python3
"""Batch guardrail checks over a JSONL file.

Each input line is a JSON object. The fields read depend on the surface:

    input         {"text": "..."}                     or {"content": "..."}
    output        {"text": "...", "user_message": "...", "context": ["..."]}
    conversation  {"turns": [{"role": "user", "content": "..."}, ...]}

Any other fields are carried through to the result line under "input", so ids and expected
labels survive the round trip.

    python3 check.py --surface input --jsonl cases.jsonl --out results.jsonl
    python3 check.py --surface input --jsonl cases.jsonl --expect expected_action --summary
    python3 check.py --surface input --jsonl cases.jsonl --record answers.jsonl

Pass --record on any real run. It saves Jev's raw answers, which makes every later threshold
question an offline replay (see scripts/sweep.py) instead of another call.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from collections import Counter
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from typing import Any

REPO = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(REPO / "python" / "src"))

from guardrail_chatbot_jev import Guard, Policy, RecordingTransport, Verdict, auto_transport  # noqa: E402


def check_one(guard: Guard, surface: str, case: dict[str, Any]) -> Verdict:
    if surface == "conversation":
        return guard.check_conversation(case["turns"], metadata=case.get("metadata"))
    text = case.get("text") or case.get("content") or ""
    if surface == "output":
        return guard.check_output(
            text,
            user_message=case.get("user_message"),
            context=case.get("context"),
            metadata=case.get("metadata"),
        )
    return guard.check_input(text, metadata=case.get("metadata"))


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--surface", choices=("input", "output", "conversation"), default="input")
    parser.add_argument("--jsonl", required=True, type=Path, help="Input cases, one JSON object per line.")
    parser.add_argument("--out", type=Path, help="Write result lines here; default stdout.")
    parser.add_argument("--record", type=Path, help="Write Jev's raw answers here, for offline replay.")
    parser.add_argument("--policy", help="Pack name or path; defaults to the bundled standard-v1.")
    parser.add_argument("--concurrency", type=int, default=8)
    parser.add_argument("--expect", help="Field holding the expected action, for scoring.")
    parser.add_argument("--summary", action="store_true", help="Print a breakdown to stderr.")
    args = parser.parse_args(argv)

    if not os.environ.get("JEV_API_KEY"):
        print("JEV_API_KEY is not set; every check would fail closed.", file=sys.stderr)
        return 2

    cases = [json.loads(line) for line in args.jsonl.read_text("utf-8").splitlines() if line.strip()]
    policy = Policy.load(args.policy)
    inner = auto_transport()

    # One recorder per case, so answers stay matched to their case even though the pool finishes
    # them out of order.
    sinks: list[list[dict[str, Any]]] = [[] for _ in cases]

    def run(index: int) -> Verdict:
        guard = Guard(policy, transport=RecordingTransport(inner, sinks[index]))
        return check_one(guard, args.surface, cases[index])

    with ThreadPoolExecutor(max_workers=args.concurrency) as pool:
        verdicts = list(pool.map(run, range(len(cases))))

    sink = args.out.open("w", encoding="utf-8") if args.out else sys.stdout
    try:
        for case, verdict in zip(cases, verdicts):
            sink.write(json.dumps({"input": case, "verdict": verdict.as_dict()}, ensure_ascii=False) + "\n")
    finally:
        if args.out:
            sink.close()

    if args.record:
        _write_records(args.record, args.surface, cases, sinks, args.expect or "expected_action")
        print(f"recorded {sum(1 for s in sinks if s)} answer sets to {args.record}", file=sys.stderr)

    if args.summary or args.expect:
        _report(cases, verdicts, args.expect)

    return 1 if any(v.degraded for v in verdicts) else 0


def _write_records(
    path: Path,
    surface: str,
    cases: list[dict[str, Any]],
    sinks: list[list[dict[str, Any]]],
    expect_field: str,
) -> None:
    with path.open("w", encoding="utf-8") as handle:
        for case, sink in zip(cases, sinks):
            if not sink:
                continue  # prefiltered, cached, or the call failed: nothing to replay
            entry = sink[0]
            handle.write(
                json.dumps(
                    {
                        "id": case.get("id", ""),
                        "surface": surface,
                        "input": case,
                        "expected_action": case.get(expect_field),
                        "state": entry["state"],
                        "answers": entry["answers"],
                        "model": entry["model"],
                        "usage": entry["usage"],
                    },
                    ensure_ascii=False,
                )
                + "\n"
            )


def _report(cases: list[dict[str, Any]], verdicts: list[Verdict], expect: str | None) -> None:
    actions = Counter(v.action for v in verdicts)
    print(f"\n{len(verdicts)} cases: " + ", ".join(f"{a}={n}" for a, n in actions.most_common()), file=sys.stderr)

    degraded = sum(v.degraded for v in verdicts)
    if degraded:
        print(f"WARNING: {degraded} verdicts are degraded and say nothing about the content.", file=sys.stderr)

    categories = Counter(c for v in verdicts for c in v.categories)
    if categories:
        print("categories: " + ", ".join(f"{c}={n}" for c, n in categories.most_common(8)), file=sys.stderr)

    if not expect:
        return
    scored = [(c, v) for c, v in zip(cases, verdicts) if expect in c]
    if not scored:
        print(f"no case carries a {expect!r} field", file=sys.stderr)
        return
    hits = sum(1 for c, v in scored if c[expect] == v.action)
    print(f"exact match: {hits}/{len(scored)} ({hits / len(scored):.0%})", file=sys.stderr)
    for case, verdict in scored:
        if case[expect] != verdict.action:
            rules = ", ".join(verdict.applied_rules) or "none"
            print(
                f"  expected {case[expect]:<6} got {verdict.action:<6} "
                f"[{', '.join(verdict.categories) or 'no findings'}] rules: {rules}",
                file=sys.stderr,
            )


if __name__ == "__main__":
    raise SystemExit(main())
