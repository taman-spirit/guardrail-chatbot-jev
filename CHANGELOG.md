# Changelog

All notable changes to this project are recorded here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Python Viet Nam compliance 1.2.3] - 2026-09-26

### Changed

- Includes Python 1.1.4: `Session.model_history()` tells the chat model which categories withheld a
  turn instead of dropping it, and the reason is kept in `as_state` / `from_state`.
- `Responder.model_history(session, lang)` now works on a session restored from a store: the group,
  the reply and the crisis line come back the same as before it was stored.

## [Python Viet Nam compliance 1.2.2] - 2026-09-26

### Fixed

- A request to hurt others is no longer answered as self-harm. The crisis route is taken only when
  self-harm leads the turn: no other finding at a stronger action, or at the same action with a
  higher probability. Measured on Jev, the self-harm sentinel answers 0.15 to 0.66 on requests to
  build a bomb or bring down a building, and every one of seven such requests got the crisis reply;
  now all seven get the violence reply, and all seven self-harm messages still get the crisis reply.
  The verdict is unchanged: the content is still held. Includes Python 1.1.3.
- After a withheld turn the chat model no longer guesses. `Responder.model_history(session, lang)`
  replaces each withheld turn with a note naming its group (never its text) and the reply the user
  was shown, so "do it" or "my first request" is answered in context. Jev still reads only the
  neutral placeholder.

### Added

- `vietnam-compliance-v1` replies for violence and weapons, harm to children (hotline 111), crime,
  sexually explicit content, hate, attempts on the system's safety settings and copyright, in
  Vietnamese, English and Chinese. Misinformation answers with the Law on Cybersecurity reply.
- `crisis_footer`: a line pointing to the crisis number, added to another group's reply when the
  self-harm probability is still at or over its block band.
- `withheld`: the note and per-group labels `Responder.model_history` uses.
- The Go and Python responders pick the same verdict, group and text on 40,000 random answer sets.

## [Python Viet Nam compliance 1.2.1] - 2026-09-26

### Documentation

- README, Definitions: a table of the settings in use and of every category's thresholds, including
  the `vietnam-compliance-v1` categories; the rules and the single-turn calibration explained in
  words before the formulas; each formula block followed by links to the code. In English,
  Vietnamese, French and Japanese.
- README, Releases: the demo at https://nhatnguyet.org/tro-ly-ai.

## [Python Viet Nam compliance 1.2.0] - 2026-09-26

### Changed

- Includes Python 1.1.1: multi-turn attribution, realtime review as audit, sentinel corroboration.
- The Viet Nam pack never weakens a lone self-harm, sovereignty or leader signal
  (`sentinel_corroboration.weak_except: ["ssh", "vsv", "vld"]`), and a refusal does not excuse a
  sovereignty or leader claim.
- The review reply no longer promises a staff reply that a realtime chat cannot give.
- `scripts/build-packs.py` merges an overlay's `defaults` by key.

## [Python 1.1.1] - 2026-09-26

### Fixed

- A lone self-harm sentinel in the review band is no longer weakened to flag, so it still reaches
  the crisis response: `sentinel_corroboration.weak_except` keeps `ssh` whole. A flag-band one
  still does not replace an ordinary answer.

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

## [Python Viet Nam compliance 1.1.0] - 2026-09-23

### Added

- `vietnam-compliance-v1`, a policy pack for AI services in Viet Nam covering the content
  requirements of the Law on Artificial Intelligence (Luật Trí tuệ nhân tạo) and the Law on
  Cybersecurity (Luật An ninh mạng), with a labelled set for calibration.
- `Responder`, which picks the pack's prewritten reply for a verdict in Vietnamese, English or
  Chinese instead of letting the model write it.
- `scripts/build-packs.py`, which builds derived packs from overlays in `policies/overlays/`.
- A step-by-step compliance guide for AI services in Viet Nam, in Vietnamese, English and Chinese:
  `docs/vietnam-compliance*.md`.

## [Python 1.0.1] - 2026-09-23

### Changed

- A rule can set `skip_confidence_gate`, so that low confidence alone does not hold content the
  rule vouches for. A finding a rule capped to `allow` no longer sets the route.
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
