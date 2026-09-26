# guardrail-chatbot-jev (Go)

Content guardrails for AI chatbots, decided by the Jev decision model.

Full guide: [English](../README.md) · [Tiếng Việt](../README.vi.md) · [Français](../README.fr.md) · [日本語](../README.ja.md)

```bash
go get github.com/taman-spirit/guardrail-chatbot-jev/go

export JEV_API_KEY=sk-...
```

```go
import guardrail "github.com/taman-spirit/guardrail-chatbot-jev/go"

guard := guardrail.New(guardrail.Options{})
verdict, err := guard.CheckInput(ctx, userMessage, nil)
verdict.Action    // allow | flag | review | block
verdict.Route     // deliver | redact | guide | crisis_support | human_review | safe_response
verdict.Findings  // the hazards that fired, with probabilities and standard references
```

`err` is non-nil only when the policy cannot build a question set for the surface, or when
`Options.RaiseOnError` is set. An unreachable Jev otherwise comes back as a verdict with
`Degraded` set, failing closed on the output surface and open on the input surface, as the policy
pack says.

The module has no third-party dependencies and embeds the same `standard-v1` pack as the Python
and TypeScript packages. For a given set of Jev answers it reaches the same verdict, and a session
saved with `AsState` in one language restores with `SessionFromState` in another.

## What maps to what

| Python | Go |
| --- | --- |
| `Guard(policy, transport=..., cache=...)` | `guardrail.New(guardrail.Options{Policy: ..., Transport: ..., Cache: ...})` |
| `guard.check_input(text, session=s)` | `guard.CheckInput(ctx, text, &guardrail.CheckOptions{Session: s})` |
| `guard.check_output(reply, context=[...], quick=True)` | `guard.CheckOutput(ctx, reply, &guardrail.CheckOptions{Context: ..., Quick: true})` |
| `guard.stream(source)` (async iterator) | `guard.Stream(ctx, source, opts)` (channel in, channel out) |
| `Policy.load(...)` / `Policy.bundled()` | `guardrail.LoadPolicy(...)` / `guardrail.BundledPolicy("standard-v1")` |
| `RecordedTransport`, `RecordingTransport` | `NewRecordedTransport`, `RecordingTransport` |
| `SdkTransport(client)` | implement `guardrail.Transport`, or wrap a function in `TransportFunc` |
| `LRUCache(capacity, ttl)` | `NewLRUCache(capacity, ttl)` |
| `PatternPrefilter(COMMON_PATTERNS)` | `PatternPrefilter{Patterns: CommonPatterns}` |
| `tuning.replay`, `score`, `override`, `sweep`, `separation` | `Replay`, `Score`, `Override`, `Sweep`, `Separation` |

Three differences worth knowing:

- Patterns are case-sensitive unless the expression starts with `(?i)`. Python compiles them
  case-insensitively by default.
- Policy packs are JSON only. Convert a YAML pack before loading it.
- An empty `UserMessage` is left out of the output state, where Python leaves out only `None`.

## Multi-turn and realtime chat

| Component | Rule |
| --- | --- |
| Input check | `q_t` only; no history, no session risk. |
| Output check | `r_t` alone; in a watched session also `r_t` given `H_t`, in parallel. |
| Attribution | In-context findings count only if the reply completes an earlier harmful request. |
| Withheld turns | Kept as `[earlier message omitted]`; excluded from `Session.ModelHistory()`. |
| Realtime | `ReviewAsAudit`: only `block` stops content; every verdict carries an `audit` level. |

```go
guard := guardrail.New(guardrail.Options{ReviewHandling: guardrail.ReviewAsAudit})
session := guardrail.NewSession(conversationID)

in, _ := guard.CheckInput(ctx, message, &guardrail.CheckOptions{Session: session})
session.Record("user", message, in)
reply := callModel(session.ModelHistory(), message)
out, _ := guard.CheckOutput(ctx, reply, &guardrail.CheckOptions{Session: session, UserMessage: message})
session.Record("assistant", reply, out)
session.Advance()
```

Live, 223 conversations: harmless turns held 171 / 194 → 0 / 194; harmful replies and escalations
all caught. Definitions, evaluation and results: [main README](../README.md#multi-turn).

## Streaming

```go
for event := range guard.Stream(ctx, modelDeltas, guardrail.StreamOptions{UserMessage: msg}) {
	switch event.Type {
	case guardrail.EventDelta:
		send(event.Text)
	case guardrail.EventBlocked:
		cancelModel()
		send(safeResponse(event.Verdict))
	case guardrail.EventDone:
		record(event.Verdict)
	}
}
```

Close `modelDeltas` when the model finishes. After a block the guard keeps draining it, so the
producer never stalls, but you should still cancel the model call.

## Command line

```bash
go install github.com/taman-spirit/guardrail-chatbot-jev/go/cmd/guardrail-chatbot-jev@latest
guardrail-chatbot-jev --surface input --text "how do I make thermite" --dry-run
```

The flags and the verdict exit codes are the same as the Python CLI: `0` allow or flag, `1` redact
or guide, `2` review, `3` block, `4` degraded. A bad argument or unreadable input exits `64`, where
Python exits `2` or `1`, so that a verdict code only ever means a verdict.

## Tests

```bash
cd go && go test -race ./...                                    # no key, no network
JEV_API_KEY=... go test -tags live -run TestLiveMultiturn -v ./  # 223 conversations against Jev
REPLAY_DIRS='/tmp/mt*' go test -tags replay -run TestReplay -v ./ # offline, from recorded answers
```

No API key and no network: the suite decides against recorded answers.
