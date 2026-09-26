# Changelog

All notable changes to this project are recorded here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Python 1.1.0] - 2026-09-26

### Changed

- Multi-turn: a turn is held only for what it or its reply does. The input check never reads the
  history; in a watched session the reply is also read in context, and those findings count only
  when the reply completes an earlier harmful request. `multiturn="floor"` keeps the earlier floor.
  The same engine ships in TypeScript and in the Go module, with the same decisions.
- Withheld turns are kept as `[earlier message omitted]` (`Session.record`) and left out of
  `Session.model_history()`; the session's risk is no longer sent to Jev.
- `standard-v1`: sentinel corroboration, a confidence gate that respects benign intent, a cap for
  conversations that are not escalating, `spc` judged per reply, and `ncr` / `iwp` descriptions
  that exclude victims and questions about the law.

### Added

- `review_handling="audit"` (`reviewHandling: "audit"` in TypeScript) for realtime chat: only
  `block` stops content, and every verdict carries an `audit` level.
- `examples/multiturn-live.jsonl` (223 conversations) and `examples/multiturn-contamination.jsonl`
  (26 scenarios).

## [Python 1.0.1] - 2026-09-23

### Changed

- The licence is now CC BY-NC 4.0 (Creative Commons Attribution-NonCommercial 4.0 International).

## [1.0.0]

First public release.

### Added

- Three checks: `check_input`, `check_output` and `check_conversation`, in Python and TypeScript,
  sharing one policy file.
- `policies/standard-v1.json`: 18 hazard categories drawn from MLCommons AILuminate v1.1, Meta
  Llama Guard S1-S14 and the OWASP Top 10 for LLM Applications 2025, plus 8 context signals and 10
  rules connecting them.
- Separate `action` and `route` axes, so how strongly to react and how to handle it are decided
  independently.
- Sentinel questions for the six categories where a miss is unacceptable, asked in the same
  request as the main choice.
- A confidence gate: a low-confidence answer escalates to `review` rather than resolving as safe,
  and never downgrades a block.
- Deployment plumbing: verdict cache, deterministic prefilter, per-surface fail modes, a session
  carrying risk between turns over a ten-turn transcript window, an observer hook, and a streaming
  mode that releases text one chunk behind its check.
- `Session.as_state()` and `Session.from_state()`, so a session can be persisted between requests
  and shared across workers. The verdict log is not part of that state: it is in-process
  observability, and a restored session should not claim verdicts it never emitted.
- Offline tuning: `RecordingTransport` keeps what a live run saw, and `scripts/sweep.py` replays
  it against modified thresholds without calling the API.
- 58 labelled cases in `examples/`, in Vietnamese, English, French and Japanese: 36 input, 15
  output, 7 conversations.
- Runnable examples: one guarded turn in both languages, the same turn behind FastAPI with SSE and
  a session store, and an overlay that adds a compliance domain of your own.
- A CLI whose exit code carries the verdict, with `4` reserved for a degraded one, because on the
  input surface a degraded verdict's action is `allow` and reporting that as success would tell a
  pipeline the content passed a check that never ran.
- Documentation in English, Vietnamese, French and Japanese.
