# Changelog

All notable changes to this project are recorded here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- `vietnam-compliance-v1`, a policy pack for AI services in Viet Nam covering the content
  requirements of the Law on Artificial Intelligence (Luật Trí tuệ nhân tạo) and the Law on
  Cybersecurity (Luật An ninh mạng), with a labelled set for calibration.
- `Responder`, which picks the pack's prewritten reply for a verdict in Vietnamese, English or
  Chinese instead of letting the model write it.
- `scripts/build-packs.py`, which builds derived packs from overlays in `policies/overlays/`.
- A step-by-step compliance guide for AI services in Viet Nam, in Vietnamese, English and Chinese:
  `docs/vietnam-compliance*.md`.

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
- MIT licensed.
