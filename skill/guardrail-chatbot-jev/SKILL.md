---
name: guardrail-chatbot-jev
description: Check AI chatbot content for safety and compliance violations using the Jev decision model. Use when auditing chatbot transcripts or prompts, testing a guardrail policy, tuning thresholds, triaging a flagged conversation, or building guardrail checks into an application. Covers user input, assistant output, and multi-turn conversations against a taxonomy aligned with MLCommons AILuminate, Llama Guard and OWASP LLM Top 10.
---

# Jev Guardrail

Judge chatbot content against a policy pack and return a typed verdict: an **action**
(`allow` / `flag` / `review` / `block`) and a **route** (`deliver` / `redact` / `guide` /
`crisis_support` / `human_review` / `safe_response`), with the hazard findings behind it.

Jev is a decision model, not a generative one. It returns calibrated probabilities over labels you
define, in 70-500ms, and cannot invent a category that is not in the policy. That is what makes it
a guardrail rather than a second chatbot grading the first.

## Before anything else

Check what the task needs:

| The user wants | Do this |
| --- | --- |
| A verdict on some content | Run `scripts/check.py` (below) |
| To understand why something was blocked | Run the check, then read `verdict.findings` and `applied_rules` |
| To change what counts as a violation | Edit `policies/standard-v1.json`, then re-run the test suite |
| To add the checks to their code | `examples/integration.py` or `.ts`, and the README's wiring section; do not re-implement |
| To put it behind an HTTP API | `examples/chatbot_server.py`: FastAPI, SSE streaming, sessions per conversation |
| To know what a category means | `references/taxonomy.md` |

A real check needs an API key in `JEV_API_KEY` and outbound HTTPS to the Jev endpoint. Managed
and sandboxed environments often deny that host at the egress proxy;
when they do, every verdict returns `"degraded": true` and says nothing about the content, so
check that flag before reporting any result. Without a key or a route, `--dry-run` still shows the
exact request that would be sent, which is enough to review or tune the questions.

## Running a check

```bash
export JEV_API_KEY=sk-...
pip install -e python/                       # once

# a user message, before it reaches the model
guardrail-chatbot-jev --surface input --text "..."

# an assistant reply, before it reaches the user
guardrail-chatbot-jev --surface output --text "..." \
  --user-message "..." --context "retrieved passage"

# a whole conversation, as JSON [{"role":"user","content":"..."}, ...]
guardrail-chatbot-jev --surface conversation --text "$(cat transcript.json)"

# see the request without sending it, no key needed
guardrail-chatbot-jev --surface input --text "..." --dry-run
```

Exit codes: `0` allow or flag, `1` redact or guide route, `2` review, `3` block. Output is JSON on
stdout, so pipe it to `jq`.

For a batch of content, use `scripts/check.py` with a JSONL file rather than one call per line:

```bash
python3 scripts/check.py --surface input --jsonl cases.jsonl --out results.jsonl
```

To measure the policy against the labelled sets in `examples/`, run `scripts/calibrate.sh`. It
covers all three surfaces and, importantly, records Jev's raw answers, so every later threshold
question is an offline replay through `scripts/sweep.py` rather than another call.

Content in any language is fine. The shipped policy is written for English, Vietnamese, French and
Japanese, and instructs the model not to judge a non-English framing more leniently, since
translation is a standard way around a guardrail.

## Reading a verdict

```json
{
  "action": "review",
  "route": "redact",
  "severity": 3.0,
  "confidence": 0.71,
  "findings": [{"category": "prv", "probability": 0.41, "action": "review", "refs": ["AILuminate prv", "Llama Guard S7"]}],
  "applied_rules": ["low-actionability-softens"],
  "degraded": false
}
```

Read it in this order:

1. **`degraded: true`** means Jev was unreachable and the fallback decided. The verdict says
   nothing about the content. Fix the connection before drawing any conclusion from it.
2. **`action`** is the decision; **`route`** is the handling. They move independently, so a
   `review` can still be deliverable after redaction.
3. **`confidence`** below the policy's `min_confidence` is why a borderline case became `review`.
   A low-confidence answer is not evidence of safety.
4. **`applied_rules`** names every rule that moved the verdict. If a result looks wrong, the
   explanation is almost always here, not in the thresholds.

## Tuning a policy

Thresholds are per category and per surface, and the descriptions in the pack are the text Jev
actually reads. Changing a description changes the model's behaviour as much as changing a number.

If a calibration recording exists, answer threshold questions with it before changing anything:

```bash
scripts/sweep.py separation calibration/cases-input.answers.jsonl   # can a threshold help at all?
scripts/sweep.py sweep      calibration/cases-input.answers.jsonl --axis ncr.input.block \
    --from 0.2 --to 0.8 --step 0.05
scripts/sweep.py report     calibration/cases-input.answers.jsonl --set ncr.input.block=0.45
```

`separation` comes first. Where the cases a category should catch and the cases it should not
produce overlapping probabilities, no number separates them and the fix is the description.

After any edit to `policies/*.json`:

```bash
scripts/sync-policies.sh        # both packages ship a copy
cd python && python3 -m pytest -q
cd ../ts && npm test
```

The suites run entirely on recorded answers, so they need no API key and catch the mistakes that
matter: thresholds out of order, a rule pointing at a category that does not exist, a softening
rule that quietly erases a finding.

Two things to keep in mind when changing numbers:

- A `choice` question spreads probability across every label, so a hazard competing with 17 others
  rarely exceeds 0.5. Thresholds are calibrated for that, which is why they look low. Categories
  where a miss is unacceptable also get a `sentinel` yes/no question that does not have to compete.
- Lowering a threshold raises recall and the review queue together. Ask what the deployment can
  actually staff before lowering one.

## Integration questions

If the question is about putting this in a product rather than running a check, the answers are in
the README under **Wiring it into a product**, and the runnable versions are
`examples/integration.py` and `examples/integration.ts`. The short form:

- The input check runs beside the model call, not before it, so its latency hides behind the
  model's first token.
- Streamed replies are released one chunk behind their check; mid-stream checks ask sentinels only.
- The conversation check stays off the critical path and feeds a `Session`, which raises a floor
  under the following turns.
- Deterministic matches (a leaked key, a card number) belong in a `PatternPrefilter` ahead of Jev.
- `on_error` is per surface: input fails open, output fails closed, and both mark the verdict
  `degraded`. Count degraded verdicts separately from blocks.

## What this does not do

Jev reads the content it is given and nothing else. It cannot look anything up, count reliably, or
do arithmetic, and it reads literally, so negation and implication are its weak spots. Retrieval,
rate limits, account state and deterministic checks (a blocklist, a regex for card numbers) belong
in the application code around it, not in a question.

It also does not enforce anything. It returns a verdict; the deployment decides what to do with it.
