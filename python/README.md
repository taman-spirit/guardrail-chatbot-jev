# guardrail-chatbot-jev (Python)

Content guardrails for AI chatbots, decided by the Jev decision model.

Full guide: [English](../README.md) · [Tiếng Việt](../README.vi.md) · [Français](../README.fr.md) · [日本語](../README.ja.md)

```bash
pip install guardrail-chatbot-jev

export JEV_API_KEY=sk-...
```

```python
from guardrail_chatbot_jev import Guard

guard = Guard()
verdict = guard.check_input("...")
verdict.action       # 'allow' | 'flag' | 'review' | 'block'
verdict.route        # 'deliver' | 'redact' | 'guide' | 'crisis_support' | 'human_review' | 'safe_response'
verdict.findings     # the hazards that fired, with probabilities and standard references
```

The package has no third-party dependencies: it talks to the endpoint over stdlib HTTP. If your
provider ships its own SDK client, hand it to `SdkTransport` instead.

For a deployment, add the pieces around the checks: a cache, a deterministic prefilter, a session
carrying risk between turns, and the input check running beside the model call rather than before
it. The root guide covers all of them, and `examples/integration.py` is the runnable version.

Offline, for tests and policy tuning:

```python
from guardrail_chatbot_jev import Guard, Policy, RecordedTransport, RecordingTransport, decide

decide(Policy.bundled(), "input", recorded_answers)          # no client at all
Guard(transport=RecordedTransport(recorded_answers))          # exercise the full path
Guard(transport=RecordingTransport(real_transport))           # keep what a real run saw
Guard().preview("input", {"user_message": "..."})             # the request, unsent
```

## License

MIT. See `../LICENSE`.
