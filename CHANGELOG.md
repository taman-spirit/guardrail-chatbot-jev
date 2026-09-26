# Changelog

All notable changes to this project are recorded here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Go 1.2.4, Python 1.1.4] - 2026-09-26

### Changed

- Go, Python and TypeScript: `Session.ModelHistory()` tells the chat model about each withheld turn
  instead of dropping it. A withheld user message becomes `WithheldNote`, naming the categories
  that stopped it and never its text, followed by `DeclinedReply` unless a reply was recorded after
  it; a withheld reply becomes `WithheldReplyNote`. Dropping them left the model to meet "do it" or
  "my first request" with nothing before it, and it guessed. Jev still reads only the neutral
  placeholder.
- `Session.Record` keeps why each turn was withheld, trimmed with the transcript window, and
  `AsState` / `SessionFromState` carry it under `withheld`, in the same shape in every language: a
  session written by Go, Python or TypeScript reads back the same model history in the others.

## [Go 1.2.3, Python 1.1.3] - 2026-09-26

### Fixed

- Go, Python and TypeScript: a request to hurt others is no longer routed to crisis support. The
  crisis route is taken only when self-harm leads the turn: no other finding at a stronger action,
  or at the same action with a higher probability (a tie goes to self-harm). Measured on Jev, the
  self-harm sentinel answers 0.15 to 0.66 on requests to build a bomb or bring down a building, and
  every one of seven such requests was routed to crisis support; now none is, and all seven
  self-harm messages still are. The action is unchanged: the content is still held. The three
  engines agree on 40,000 random answer sets.

## [Go 1.2.2, Python 1.1.2] - 2026-09-26

### Documentation

- README, Definitions: a table of the settings in use and one of every category's thresholds, the
  rules and the single-turn calibration explained in words before the formulas, and each formula
  block followed by links to the code that implements it. In English, Vietnamese, French and Japanese.
- README, Releases: the demo at https://nhatnguyet.org/tro-ly-ai.

## [Go 1.2.1, Python 1.1.1] - 2026-09-26

### Fixed

- Go, Python and TypeScript: a lone self-harm sentinel in the review band is no longer weakened to flag, so it still reaches
  the crisis response: `sentinel_corroboration.weak_except` keeps `ssh` whole. A flag-band one
  still does not replace an ordinary answer.

## [Go 1.2.0, Python 1.1.0] - 2026-09-26

### Changed

- Multi-turn, in Go, Python and TypeScript (the Go engine was first released as `go-multiturn-v1`): a turn is held only for what it or its reply does. The input check never reads the
  history; in a watched session the reply is also read in context, and those findings count only
  when the reply completes an earlier harmful request. `MultiturnFloor` keeps the earlier floor.
- Withheld turns are kept as `[earlier message omitted]` and left out of `Session.ModelHistory()`;
  the session's risk is no longer sent to Jev; a transcript of withheld turns alone is not checked.
- `standard-v1`: sentinel corroboration, a confidence gate that respects benign intent, a cap for
  conversations that are not escalating, specialised advice judged per reply, and `ncr` / `iwp`
  descriptions that exclude victims and questions about the law.

### Added

- `ReviewHandling: ReviewAsAudit` for realtime chat, and an `audit` level on every verdict.
- `examples/multiturn-live.jsonl` (223 conversations) and `examples/multiturn-contamination.jsonl`
  (26 simulated scenarios); live, replay and regression tests in `go/` behind the `live` and
  `replay` build tags.

## [Go 1.0.1] - 2026-09-23

### Added

- A Go module in `go/`, a port of the Python package: the three checks, `CheckTurn`, the cache,
  prefilter, session, streaming over channels, the recording transports, offline tuning and the
  command line. It embeds the same policy pack, sends the same request and reaches the same
  verdict for the same answers; sessions saved by one language restore in the other. No
  third-party dependencies.

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
