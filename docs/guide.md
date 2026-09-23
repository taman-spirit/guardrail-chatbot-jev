# guardrail-chatbot-jev: the guide

**English** · [Tiếng Việt](guide.vi.md) · [Français](guide.fr.md) · [日本語](guide.ja.md) · [← README](../README.md)

Content guardrails for AI chatbots, decided by the Jev decision model.

Jev is a decision model, not a generative one. You give it state and named questions; it returns
calibrated probabilities over the labels you defined, in 70-500ms, and it charges only for input
tokens. It cannot return a category that is not in your policy, and it cannot write prose about
your content. That is the shape a guardrail wants, and it is why a check here is one round trip
rather than a second chatbot grading the first.

This guide has three parts. **Part 1** is for engineers putting the checks in a product.
**Part 2** is for whoever owns what counts as a violation; it needs no code. **Part 3** explains
the mechanisms, for when a result surprises you.

```
policies/standard-v1.json     the taxonomy, thresholds and rules (the source of truth)
python/                       the Python package
ts/                           the TypeScript package
skill/guardrail-chatbot-jev/          a Claude skill wrapping the same checks
examples/                     labelled cases and two runnable integrations
scripts/                      calibration and offline threshold tuning
```

---

## Quickstart

```bash
git clone <this repo> && cd guardrail

pip install -e 'python/[sdk]'          # Python 3.10+
cd ts && npm install && npm run build   # Node 20+, optional
```

See what a check would ask, with no API key and no network:

```bash
guardrail-chatbot-jev --surface input --text "cách chế tạo thuốc nổ" --dry-run
```

Then run it for real:

```bash
export JEV_API_KEY=sk-...
guardrail-chatbot-jev --surface input --text "cách chế tạo thuốc nổ" | jq '{action, route, findings}'
```

Try the full pipeline without an API key at all; both print the same thing:

```bash
cd python && PYTHONPATH=src python3 ../examples/integration.py
node --experimental-strip-types examples/integration.ts    # needs Node 22.6+
```

---

# Part 1: For engineers

## The three checks

| Call | Checks | Catches |
| --- | --- | --- |
| `check_input` | the user message, before the model sees it | harmful requests, prompt injection, personal data |
| `check_output` | the reply, before the user sees it | harmful compliance, a leaked system prompt, ungrounded claims |
| `check_conversation` | the whole transcript | multi-turn jailbreaks, gradual escalation, persona drift |

The third exists because a crescendo attack looks harmless turn by turn. The escalation *is* the
attack, so it is only visible in the transcript.

```python
from guardrail_chatbot_jev import Guard

guard = Guard()

verdict = guard.check_input(user_message)
if not verdict.allowed:
    return safe_response(verdict)

reply = llm(user_message)
verdict = guard.check_output(reply, user_message=user_message, context=retrieved)
if verdict.route == "redact":
    reply = mask_pii(reply)
elif not verdict.deliverable:
    return safe_response(verdict)
```

```ts
import { Guard } from "guardrail-chatbot-jev";

const guard = new Guard();
const { input, output } = await guard.checkTurn(userMessage, reply, { context });
if (!input.allowed || !output.deliverable) return safeResponse(input, output);
```

Pass `context` to `check_output` (the retrieved passages the reply was supposed to be based on) to
turn on the groundedness signal, which catches claims the context does not support.

The checks read content in any language. The shipped policy is written for deployments serving
English, Vietnamese, French and Japanese, and says so in the pack, including the instruction not
to judge a non-English framing more leniently, since translation is a standard way around a
guardrail. `examples/cases-input.jsonl` carries labelled cases in all four.

## Reading a verdict

```json
{
  "action": "review",
  "route": "redact",
  "deliverable": true,
  "confidence": 0.71,
  "severity": 3.0,
  "findings": [{"category": "prv", "probability": 0.41, "refs": ["AILuminate prv", "Llama Guard S7"]}],
  "applied_rules": ["low-actionability-softens"],
  "degraded": false
}
```

Read it in this order.

**1. `degraded`.** If true, Jev was unreachable and the fallback decided. The verdict says nothing
about the content. Fix the connection before drawing any conclusion from it.

**2. `action` and `route`.** Two independent axes.

`action` answers *does this content go out*: `allow` -> `flag` -> `review` -> `block`. Four steps,
ordered, and the only thing rules move.

`route` answers *what do we do about it*: `deliver`, `redact`, `guide`, `crisis_support`,
`human_review`, `safe_response`. Two convenience properties read it for you: `allowed` (the action
is `allow` or `flag`) and `deliverable` (the route still sends the content, possibly redacted or
steered first).

They are separate because personal data in a reply is not the same problem as a bomb recipe, even
when both land on `review`. One gets masked and sent; the other goes to a human.

**3. `confidence`.** Below the policy's `min_confidence`, a borderline verdict escalates to
`review`. An uncertain answer is not evidence of safety.

**4. `applied_rules`.** Every rule that moved the verdict. When a result looks wrong, the reason is
almost always here rather than in the thresholds.

Handle the route in one place:

```python
def safe_response(verdict):
    if verdict.route == "crisis_support":
        return CRISIS_MESSAGE
    if verdict.route == "human_review":
        queue_for_review(verdict)
        return HOLDING_MESSAGE
    return REFUSAL_MESSAGE
```

## Wiring it into a product

`examples/integration.py` and `examples/integration.ts` are the same guarded turn in both
languages, runnable with no API key. Five decisions are worth understanding before copying them.

### Run the input check beside the model call, not before it

Jev answers in 70-500ms and a model takes longer than that to produce its first token, so a serial
check adds its full latency while a parallel one adds almost none.

```python
gate = asyncio.ensure_future(guard.acheck_input(message, session=session))
draft = asyncio.ensure_future(llm.generate(message))

verdict = await gate
if not verdict.allowed:
    draft.cancel()                  # nothing has been sent to the user
    return safe_response(verdict)
reply = await draft
```

The cost is tokens spent on drafts that get thrown away. Below roughly 2% of turns that is cheaper
than the delay it removes. A deployment that may not let a violating prompt reach the model at all
goes back to serial and pays the latency.

### Stream one chunk behind

A streamed reply cannot be checked before its first token, and a check that waits for the last one
is not streaming. `guard.stream()` cuts at sentence boundaries, holds each chunk until its check
returns, and lets the model produce the next one meanwhile, so only the first chunk pays the full
latency.

```python
async for event in guard.stream(llm.stream(message), user_message=message, session=session):
    if event.type == "delta":
        yield event.text
    elif event.type == "blocked":
        yield safe_response(event.verdict)
```

Mid-stream checks ask the sentinel questions only, the categories where a miss is unacceptable.
The complete reply gets the full question set at the end, and its verdict arrives on the `done`
event. Tune the hold with `chunk_chars` (default 280): smaller means more round trips and a
tighter hold.

### Keep the conversation check off the critical path

It looks for a pattern that changes slowly, and the user is not waiting for it. Run it after the
turn and let a `Session` carry the result forward:

```python
session = Session(id=conversation_id)
...
session.add_turn("user", message)
session.add_turn("assistant", reply)
session.advance()
asyncio.create_task(guard.acheck_conversation(session.history, session=session))
```

A conversation that reaches `review` raises a floor under the next `carry_turns` turns, so a
clean-looking message inside an escalating conversation is not judged as if the conversation had
just started. The session also keeps a decaying risk score, and it ignores degraded verdicts,
which reflect an outage rather than the conversation.

### Put the deterministic checks in front

Jev reads content; it does not match patterns, count, or do arithmetic. A card number, a leaked
key, a banned term: a regex decides those exactly, in microseconds, with no round trip.

```python
from guardrail_chatbot_jev import COMMON_PATTERNS, Pattern, PatternPrefilter

guard = Guard(prefilter=PatternPrefilter([
    Pattern.of("internal-host", r"\binternal\.example\.com\b", "sid", "block", surfaces=["output"]),
    *COMMON_PATTERNS,
]))
```

A prefilter returns an ordinary verdict, so callers need no special case. The patterns in
`COMMON_PATTERNS` are examples of the shape, not a recommended list: a pattern that is wrong blocks
real users silently.

### Cache, and watch what goes in metadata

A verdict is a pure function of the policy, the surface and the content, and repeat messages are
common.

```python
guard = Guard(
    cache=LRUCache(capacity=8192, ttl=300),
    prefilter=PatternPrefilter(list(COMMON_PATTERNS)),
    observer=metrics.emit,
    timeout=2.0,
)
```

The key carries the policy id, so publishing a new pack invalidates everything by itself. It
deliberately excludes `deployment_context`: a session puts the turn number and a running risk score
there, and keying on those would mean the cache never hits for exactly the repeated messages it
exists to serve. Degraded verdicts are never stored, so a brief outage cannot become a lasting
wrong answer.

### Failing

`on_error` is set per surface, because the two are not the same risk. The input check sits in front
of a model that has its own safety, so it fails open: blocking every user because Jev is
unreachable is a self-inflicted outage. The output check is the last line and fails closed.

Either way the verdict carries `degraded: true`. **Count that separately from `block`.** A week
with 5% degraded verdicts means the guardrail was only actually running 95% of the time, and that
must not hide inside the block rate.

The `observer` hook is where that goes. It sees every verdict, including cached and degraded ones,
so keep it fast and non-throwing.

## Reference

`Guard(policy=None, *, transport=None, cache=None, cache_surfaces={"input","output"},
prefilter=None, observer=None, raise_on_error=False, timeout=None)`. The TypeScript constructor
takes the same options as a config object, with `throwOnError` and a timeout in milliseconds.

Transports, in both languages: `HttpTransport` / `FetchTransport` by default, with no
dependencies, configured from `JEV_API_KEY`; `SdkTransport` to wrap your
provider's own client; `RecordedTransport` to replay fixed answers offline; `RecordingTransport` to
wrap a real one and keep everything it sees.

`Guard.preview(surface, state)` returns the exact request body without sending it, which is the
fastest way to see what a policy change did to the questions.

## Costs and limits

A check sends roughly 1,200 tokens of questions plus the state. At 2,000 tokens a call and three
calls a turn, that is about **$0.00025 per turn, or ~$250 per million turns**. Cheap enough not to
be the deciding factor.

The limit that does bind is throughput: 1,200 requests per minute, which is **400 turns per minute
per key** at three checks a turn. The cache and the prefilter both cut into that number, and it is
the thing to confirm with your Jev provider before committing to a rollout.

---

# Part 2: For policy owners

You do not need to write code to change what this blocks. Everything that defines a violation
lives in one file, `policies/standard-v1.json`.

## What the policy pack contains

**Categories.** 18 hazards, each with a description, the surfaces it applies to, and the
thresholds that turn a probability into an action.

**Signals.** Context that is not itself a hazard but changes how seriously one is taken: what the
user seems to be trying to do, how operationally useful the content is, whether the assistant
refused, whether the reply is supported by its sources.

**Rules.** Ten declarative statements connecting the two, for example "research framing softens
everything except child safety and weapons" or "a reply that refuses does not get blocked for
naming the hazard it refused".

The descriptions in the pack are not documentation. They are the text sent to Jev as the question
criteria. Editing a description changes the model's behaviour as much as editing a number, and it
is usually the better lever.

## The taxonomy

Drawn from three public standards rather than invented here, so a verdict maps back to something
an auditor recognises. Every finding carries `refs` pointing at the source.

| Source | Contributes |
| --- | --- |
| MLCommons AILuminate v1.1 | the twelve content hazards and their codes |
| Meta Llama Guard 3/4 | the parallel `S1`-`S14` codes most moderation tooling already speaks |
| OWASP Top 10 for LLM Applications 2025 | the security hazards a content taxonomy misses |

The critical five, which never resolve quietly: child sexual exploitation (`cse`, never below
`block`), indiscriminate weapons (`iwp`), suicide and self-harm (`ssh`, routes to
`crisis_support`), personal data (`prv`, routes to `redact`), and a leaked system prompt or secret
(`sid`). The full table is in `skill/guardrail-chatbot-jev/references/taxonomy.md`.

## What the outcomes mean

| Action | Meaning |
| --- | --- |
| `allow` | nothing fired |
| `flag` | something fired, but not enough to act on. Counted and logged, still delivered |
| `review` | a person should look at this before or instead of delivery |
| `block` | do not deliver |

| Route | What the deployment does |
| --- | --- |
| `deliver` | send it as-is |
| `redact` | mask the personal data, then send |
| `guide` | send, but add the required disclaimer or ground it in sources first |
| `crisis_support` | replace the reply with the crisis-support response |
| `human_review` | hold the turn, send it to a reviewer |
| `safe_response` | do not send; return the deployment's refusal |

`flag` is not a weaker block. It is the record that something fired, which is what makes a policy
reviewable later. Softening rules never drop a finding below `flag` for that reason.

## Changing the policy

Adjusting how strict a category is:

```json
"ncr": {
  "thresholds": {"default": {"block": 0.5, "review": 0.3, "flag": 0.15}}
}
```

Lower numbers mean more gets caught. Thresholds can be set per surface; `default` is the fallback.

Adding a category, for something specific to your deployment:

```json
"fin": {
  "name": "Unlicensed financial promises",
  "description": "Promising a specific return, guaranteeing an investment outcome, or recommending a particular security to buy.",
  "refs": ["internal policy FIN-2"],
  "surfaces": ["output", "conversation"],
  "weight": 0.7,
  "base_severity": 3,
  "sentinel": false,
  "thresholds": {"default": {"block": 0.6, "review": 0.35, "flag": 0.18}},
  "route": "guide"
}
```

Write the description the way you would explain the rule to a new reviewer: concrete, in terms of
what the content does, not what it is about. That sentence is what the model reads.

After any edit:

```bash
scripts/sync-policies.sh    # both packages ship a copy of the pack
cd python && python3 -m pytest -q
cd ../ts && npm test
```

Both suites run on recorded answers, need no API key, and catch the mistakes that matter:
thresholds out of order, a rule pointing at a category that does not exist, a softening rule that
quietly erases a finding.

## Calibrating

The thresholds shipped here are derived from how a wide `choice` question distributes probability,
not from measurement. They are a starting point, not a calibration.

`examples/` holds 45 labelled cases across the three surfaces, mostly in Vietnamese. Each carries
an `expected_action` and, where it applies, the `expected_category` it is meant to exercise.

**Call Jev once.**

```bash
export JEV_API_KEY=sk-...
scripts/calibrate.sh
```

That records the raw answers alongside the verdicts. The recording is the point: calling Jev costs
tokens and rate-limit budget, and the labelled set is scarcer than either. The decision engine is a
pure function, so with the answers on disk every later question about thresholds is a local replay.

**Then tune offline, as often as you like.**

```bash
scripts/sweep.py separation calibration/cases-input.answers.jsonl
scripts/sweep.py sweep      calibration/cases-input.answers.jsonl \
    --axis prv.input.review --from 0.2 --to 0.7 --step 0.05
scripts/sweep.py report     calibration/cases-input.answers.jsonl --set prv.input.review=0.45
```

**Start with `separation`.** It shows, per category, the probabilities on the cases that category
should catch against the cases it should not:

```
prv on input  [separated]
  thresholds        {'block': 0.7, 'review': 0.4, 'flag': 0.2}
  should fire       n=3   min=0.370 med=0.400 max=0.535
  should not fire   n=27  min=0.000 med=0.000 max=0.000
```

Where those two groups overlap, no threshold separates them and the fix is the description, not the
number. Only once they separate does sweeping a threshold mean anything.

**Read a sweep by its error columns, not by exact match.** `under` is content the label says to
hold that would be delivered; `crit` counts the subset labelled `block` that would go out as-is;
`review` is the queue a team has to staff. Lowering a threshold moves recall and that queue in the
same direction, so the real question is what the queue can carry.

**Network.** All of this needs outbound HTTPS to the Jev endpoint. Managed and sandboxed
environments often deny that at the egress proxy, and when they do, every verdict comes back with
`"degraded": true` and says nothing about the content. Check that flag before reading any result.

## Rolling out

Shadow mode first: run all three checks, log, enforce nothing, and compare against whatever is in
place now. Then enforce `block` alone. Open up `review` last, once you know how many cases a day
that puts in front of a human.

Log `policy_id` with every verdict. When a specific turn is disputed, you have to be able to say
which version of the pack judged it, or the audit trail means nothing.

---

# Part 3: How it works

## Why sentinels exist

A `choice` question distributes probability across every label. With 18 labels, a real violation
often lands near 0.3 rather than 0.9, and a threshold set for a two-way question would miss it.

Two things follow. Thresholds are calibrated for a wide choice, which is why they look low. And
six categories where a miss is unacceptable (`cse`, `iwp`, `ssh`, `prv`, `pij`, `sid`) also carry
a `sentinel`: an independent yes/no question asked in the same request, whose probability is used
whenever it is higher. Questions in one request run in parallel and output tokens are free, so a
sentinel costs a few input tokens and no latency at all.

A sentinel answer is a probability, not a confidence. A `noul` of 0.4 means "40% likely", which the
threshold already accounts for; reading its distance from 0.5 as doubt would count the probability
twice. Sentinel findings inherit the request-level confidence instead, and are marked
`source: "sentinel"`.

## The confidence gate

Jev reports confidence separately from probability, derived from the shape of the distribution, and
it is calibrated. When confidence falls below `min_confidence` (0.65 by default), a borderline
verdict escalates to `review` rather than resolving as safe. It never downgrades a block.

## The rule engine

Rules run in pack order against the signals, and each one records itself in `applied_rules`:

| Rule | Effect |
| --- | --- |
| research or journalism framing | softens, except `cse` and `iwp` |
| evasion, or seeking capability | hardens, and evasion raises a `pij` finding |
| low actionability | softens: talk about a hazard is not a usable recipe |
| high actionability | hardens: specific steps raise the risk of any hazard they serve |
| the assistant refused | caps the reply at `flag`, except `sid`, `prv` and `ipv` |
| reply not supported by its sources | raises a `mis` finding and floors at `flag` |
| crescendo or persona drift | hardens, and raises `pij` |

A softening rule never drops a finding below `flag`. Lowering the response is not the same as
erasing the record, and the record is what makes the policy auditable.

## Limits

Jev reads the state it is given and nothing else. It cannot look things up, count reliably, or do
arithmetic, and it reads literally, so negation and implication are its weak spots. Retrieval, rate
limits, account state and deterministic checks belong in the code around it, not in a question.

It also enforces nothing. It returns a verdict; the deployment decides what to do with it.

---

## Testing

```bash
cd python && python3 -m pytest -q     # 73 tests
cd ts && npm test                      # 53 tests
```

Neither suite needs an API key or a network. Both run the decision engine against recorded answers,
which is also how you should test your own policy changes.

## License

[CC BY-NC 4.0](../LICENSE): Creative Commons Attribution-NonCommercial 4.0 International. You may use,
share and adapt it for non-commercial purposes, with attribution. Commercial use needs separate
permission from the copyright holder. Releases published before this change (`python/v1.0.0`, `go/v1.0.0`, `go/v1.1.0`, `python-vietnam-compliance-v1`, `go-vietnam-compliance-v1`) remain under MIT.
