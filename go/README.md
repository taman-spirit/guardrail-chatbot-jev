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
| `Responder(policy, crisis_line=...)`, `detect_language` | `NewResponder(policy, crisisLine)`, `DetectLanguage` |

Three differences worth knowing:

- Patterns are case-sensitive unless the expression starts with `(?i)`. Python compiles them
  case-insensitively by default.
- Policy packs are JSON only. Convert a YAML pack before loading it.
- An empty `UserMessage` is left out of the output state, where Python leaves out only `None`.

## Viet Nam

The module embeds `vietnam-compliance-v1`, a policy for AI services in Viet Nam covering the
content requirements of the **Law on Artificial Intelligence** (Luật Trí tuệ nhân tạo) and the
**Law on Cybersecurity** (Luật An ninh mạng). `Responder` picks the policy's prewritten reply for
each violation group in Vietnamese, English or Chinese. The model never writes these replies.

```go
policy, _ := guardrail.BundledPolicy("vietnam-compliance-v1")
guard := guardrail.New(guardrail.Options{Policy: policy})
responder, _ := guardrail.NewResponder(policy, "") // "" = 115

lang := guardrail.DetectLanguage(message)
in, _ := guard.CheckInput(ctx, message, nil)
if held, ok := responder.BlockingResponse([]guardrail.Verdict{in}, lang); ok {
	return held
}
reply := callModel(message)
out, _ := guard.CheckOutput(ctx, reply, &guardrail.CheckOptions{UserMessage: message})
return responder.Compose(reply, []guardrail.Verdict{in, out}, lang)
```

For the same answers, the Go module picks the same reply as the Python `Responder`, word for word.
The step-by-step guide: [Tiếng Việt](../docs/vietnam-compliance.vi.md) ·
[English](../docs/vietnam-compliance.md) · [中文](../docs/vietnam-compliance.zh.md).

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

The flags and exit codes are the same as the Python CLI: `0` allow or flag, `1` redact or guide,
`2` review, `3` block, `4` degraded.

## Tests

```bash
cd go && go test -race ./...
```

No API key and no network: the suite decides against recorded answers.
