"""Command line entry point: ``guardrail-chatbot-jev``."""

from __future__ import annotations

import argparse
import json
import sys
from typing import Any

from .guard import Guard
from .questions import conversation_state, input_state, output_state
from .types import as_turns

_EXIT = {"allow": 0, "flag": 0, "redact": 1, "guide": 1, "review": 2, "block": 3}

#: A degraded verdict says nothing about the content: Jev was unreachable, so no check happened.
#: It gets its own code because the exit status is the only thing a shell script reads, and on the
#: input surface the shipped policy fails open, which would otherwise report success. A pipeline
#: written as `guardrail-chatbot-jev ... && send` must not send content that was never checked.
_EXIT_DEGRADED = 4


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="guardrail-chatbot-jev",
        description="Check AI chatbot content against a Jev-backed policy pack.",
        epilog=(
            "Exit codes: 0 allow or flag, 1 redact or guide, 2 review, 3 block, "
            "4 degraded (Jev unreachable, so nothing was actually checked)."
        ),
    )
    parser.add_argument("--surface", choices=("input", "output", "conversation"), default="input")
    parser.add_argument("--text", help="Content to check; omit to read stdin.")
    parser.add_argument("--user-message", help="The user message an assistant reply answered.")
    parser.add_argument("--context", action="append", default=[], help="Reference passage; repeatable.")
    parser.add_argument("--policy", help="Pack name or path; defaults to the bundled standard-v1.")
    parser.add_argument("--model", help="Jev model override.")
    parser.add_argument("--timeout", type=float, default=None)
    parser.add_argument("--dry-run", action="store_true", help="Print the request body and exit; no API key needed.")
    parser.add_argument("--quiet", action="store_true", help="Print only the action.")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    text = args.text if args.text is not None else sys.stdin.read()
    guard = Guard(args.policy, timeout=args.timeout)

    if args.surface == "conversation":
        payload: Any = json.loads(text)
        state = conversation_state(as_turns(payload))
    elif args.surface == "output":
        state = output_state(text, user_message=args.user_message, context=args.context or None)
    else:
        state = input_state(text)

    if args.dry_run:
        print(json.dumps(guard.preview(args.surface, state, has_context=bool(args.context)), indent=2, ensure_ascii=False))
        return 0

    if args.surface == "conversation":
        verdict = guard.check_conversation(json.loads(text), model=args.model)
    elif args.surface == "output":
        verdict = guard.check_output(
            text, user_message=args.user_message, context=args.context or None, model=args.model
        )
    else:
        verdict = guard.check_input(text, model=args.model)

    print(verdict.action if args.quiet else json.dumps(verdict.as_dict(), indent=2, ensure_ascii=False))
    if verdict.degraded:
        return _EXIT_DEGRADED
    return _EXIT.get(verdict.action, 1)


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
