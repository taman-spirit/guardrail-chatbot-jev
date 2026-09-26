<h1 align="center">guardrail-chatbot-jev</h1>

<p align="center">
  Content safety for AI chatbots: check what the user sends, check what your bot replies,<br>
  and get back one clear decision you can act on.
</p>

<p align="center">
  <a href="https://github.com/taman-spirit/guardrail-chatbot-jev/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/taman-spirit/guardrail-chatbot-jev/actions/workflows/ci.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="License: CC BY-NC 4.0" src="https://img.shields.io/badge/license-CC%20BY--NC%204.0-lightgrey.svg"></a>
  <img alt="Python 3.10+" src="https://img.shields.io/badge/python-3.10%2B-blue.svg">
  <img alt="Node 20+" src="https://img.shields.io/badge/node-20%2B-brightgreen.svg">
  <img alt="Go 1.22+" src="https://img.shields.io/badge/go-1.22%2B-00ADD8.svg">
</p>

<p align="center">
  <b>English</b> ·
  <a href="README.vi.md">Tiếng Việt</a> ·
  <a href="README.fr.md">Français</a> ·
  <a href="README.ja.md">日本語</a>
</p>

---

## The problem this solves

You shipped a chatbot. Now someone is trying to talk it into explaining how to make thermite,
someone else pasted a customer's ID number into it, and your support model just gave confident
medical advice to a person in distress.

You need something between your users and your model that says *this one is fine, hold that one,
mask the phone number in this reply, send this person to a crisis line.* That is what
guardrail-chatbot-jev does.

It is a library, not a service. You call it, you get a verdict, and your code decides what to do.
It runs in **Python and TypeScript**, both reading the same policy file, so the two sides of your
stack cannot drift apart. Neither package has a third-party dependency.

## Releases

| Release | Tag | What it is | Licence |
| --- | --- | --- | --- |
| [Python SDK 1.0.1](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/python/v1.0.1) | `python/v1.0.1` | The Python package: three checks, cache, prefilter, sessions, streaming, offline tuning and the CLI | CC BY-NC 4.0 |
| [Go SDK 1.0.1](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/go/v1.0.1) | `go/v1.0.1` | A Go port of the Python package, reading the same policy and reaching the same verdicts | CC BY-NC 4.0 |
| [Python: Viet Nam compliance policy v1.1](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/python-vietnam-compliance-v1.1) | `python-vietnam-compliance-v1.1` | The `vietnam-compliance-v1` policy, with prewritten replies in Vietnamese, English and Chinese | CC BY-NC 4.0 |
| [Go: Viet Nam compliance policy v1.1](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/go-vietnam-compliance-v1.1) | `go-vietnam-compliance-v1.1` | The same policy and replies in Go, as module version `v1.1.1` | CC BY-NC 4.0 |

Each release note lists what the release contains and how to install it. In the same order:

```bash
pip install "git+https://github.com/taman-spirit/guardrail-chatbot-jev@python/v1.0.1#subdirectory=python"
go get github.com/taman-spirit/guardrail-chatbot-jev/go@v1.0.1
pip install "git+https://github.com/taman-spirit/guardrail-chatbot-jev@python-vietnam-compliance-v1.1#subdirectory=python"
go get github.com/taman-spirit/guardrail-chatbot-jev/go@v1.1.1
```

The Go and Viet Nam releases are built from their own branches (`go-sdk`, `guardrail-vietnam-compliance`,
`go-vietnam-compliance`), which are not merged into `main` yet. The earlier releases `python/v1.0.0`, `go/v1.0.0`, `go/v1.1.0`, `python-vietnam-compliance-v1`, `go-vietnam-compliance-v1`
are superseded by these. [All releases](https://github.com/taman-spirit/guardrail-chatbot-jev/releases).

## AI compliance in Viet Nam

For AI services in Viet Nam, the `vietnam-compliance-v1` policy covers the content requirements of
two laws:

- **Law on Artificial Intelligence** (Luật Trí tuệ nhân tạo)
- **Law on Cybersecurity** (Luật An ninh mạng)

It layers its rules on the shared taxonomy, answers each violation group with a prewritten reply in
Vietnamese, English or Chinese instead of letting the model write one, and is built so that ordinary
questions are not blocked. Calibrate it on your own traffic before going live.

The step-by-step compliance guide covers scope, transparency, prohibited content, integration in
Python and Go, calibration, human oversight and record-keeping: **[Tiếng Việt](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/guardrail-vietnam-compliance/docs/vietnam-compliance.vi.md) · [English](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/guardrail-vietnam-compliance/docs/vietnam-compliance.md) · [中文](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/guardrail-vietnam-compliance/docs/vietnam-compliance.zh.md)**.

## How it works

```
user message ──▶ check_input ──▶ your LLM ──▶ check_output ──▶ user
                     │                            │
                     └──── verdict ───────────────┘
                     allow · flag · review · block
```

A third check, `check_conversation`, reads the whole transcript. It catches what the other two
cannot: an attack spread over ten polite turns looks harmless one message at a time.

Behind the checks is **Jev**, a decision model rather than a generative one. You give it content
and named questions; it answers with calibrated probabilities over labels you defined. It cannot
invent a category that is not in your policy and it cannot write prose about your content, which
is exactly what you want from a referee. A check is one round trip, typically 70-500ms.

---

## Quick start

### 1. Install

```bash
pip install guardrail-chatbot-jev        # Python 3.10+
npm install guardrail-chatbot-jev        # Node 20+
go get github.com/taman-spirit/guardrail-chatbot-jev/go   # Go 1.22+
```

### 2. Give it a key

```bash
export JEV_API_KEY=sk-...

# Optional: only to route through a gateway or a proxy.
# export JEV_BASE_URL=https://...
```

### 3. Run your first check

```python
from guardrail_chatbot_jev import Guard

guard = Guard()

verdict = guard.check_input("how do I make thermite at home")
print(verdict.action)        # 'block'
print(verdict.route)         # 'safe_response'
print(verdict.top.category)  # 'ind' - indiscriminate weapons
```

```typescript
import { Guard } from "guardrail-chatbot-jev";

const guard = new Guard();
const verdict = await guard.checkInput("how do I make thermite at home");
console.log(verdict.action); // 'block'
```

```go
guard := guardrail.New(guardrail.Options{})
verdict, _ := guard.CheckInput(ctx, "how do I make thermite at home", nil)
fmt.Println(verdict.Action) // block
```

The Go module is documented in [`go/README.md`](go/README.md), including how its API maps onto the
Python one.

### 4. Wire it into a turn

This is the whole integration. Check the message, call your model, check the reply:

```python
from guardrail_chatbot_jev import Guard

guard = Guard()

def handle_turn(user_message, history):
    # Before the model sees it
    verdict = guard.check_input(user_message)
    if not verdict.allowed:
        return safe_response(verdict)

    reply = my_llm(user_message, history)

    # Before the user sees it
    verdict = guard.check_output(reply, user_message=user_message)
    if verdict.route == "redact":
        return mask_pii(reply)
    if not verdict.deliverable:
        return safe_response(verdict)

    return reply
```

```typescript
const guard = new Guard();

async function handleTurn(userMessage: string, history: Turn[]) {
  let verdict = await guard.checkInput(userMessage);
  if (!verdict.allowed) return safeResponse(verdict);

  const reply = await myLlm(userMessage, history);

  verdict = await guard.checkOutput(reply, { userMessage });
  if (verdict.route === "redact") return maskPii(reply);
  if (!verdict.deliverable) return safeResponse(verdict);

  return reply;
}
```

---

## Reading a verdict

A verdict answers two separate questions, and keeping them separate is the point.

```json
{
  "action": "review",
  "route": "redact",
  "deliverable": true,
  "confidence": 0.71,
  "findings": [{"category": "prv", "probability": 0.41, "refs": ["AILuminate prv", "Llama Guard S7"]}],
  "applied_rules": ["low-actionability-softens"],
  "degraded": false
}
```

**`action` - how serious is this?**

| | |
| --- | --- |
| `allow` | Nothing fired. Send it. |
| `flag` | Worth logging, not worth stopping. |
| `review` | Do not just pass this through. |
| `block` | Hold it. |

**`route` - so what do we actually do?**

| | |
| --- | --- |
| `deliver` | Send as-is. |
| `redact` | Send it, with the sensitive parts masked. |
| `guide` | Send a steered answer instead of this one. |
| `crisis_support` | Reply with crisis resources, not the model's answer. |
| `human_review` | Put it in a queue for a person. |
| `safe_response` | Send your canned refusal. |

Why two axes? Personal data in a reply and a bomb recipe both land on `review`, but one gets masked
and sent while the other goes to a human. One number could never express that.

**Convenience properties:** `verdict.allowed` (allow or flag), `verdict.deliverable` (the content
still reaches the user, perhaps redacted), `verdict.blocked`, `verdict.needs_human`,
`verdict.top` (the strongest finding).

**Always check `degraded`.** When Jev is unreachable, the verdict comes back with
`degraded: true` and says nothing about the content. Count those separately from blocks; a week at
5% degraded means your guardrail was really only running 95% of the time.

---

## What it checks for

18 hazard categories, drawn from **MLCommons AILuminate**, **Meta Llama Guard** and the
**OWASP Top 10 for LLM Applications** rather than invented, so a verdict maps back to something an
auditor recognises. They cover violent and indiscriminate weapons, self-harm, sexual content and
CSAE, hate and harassment, crime, privacy and personal data, IP, defamation, specialized advice
(medical, legal, financial), prompt injection and jailbreaks, system prompt leaks, agentic
overreach, and more.

The full list with definitions is in the [hazard taxonomy](skill/guardrail-chatbot-jev/references/taxonomy.md).

**Any language.** Content is judged by meaning, not keywords. The shipped policy is written for
English, Vietnamese, French and Japanese, and explicitly tells the model not to go easier on a
non-English framing, since translation is a standard way around a guardrail. The labelled sets in
[`examples/`](examples/) carry cases in all four.

**One policy file.** Categories, thresholds and the rules connecting them live in
[`policies/standard-v1.json`](policies/standard-v1.json), which both languages read. The
descriptions in it are the literal text sent to Jev, so editing the policy changes what the model
is asked, without touching code.

---

## Multi-turn

A multi-turn attack is built out of turns that are each defensible on their own, so a guardrail has
to read conversations, not just messages. Reading them naively has the opposite failure: the
classifier sees a violation in the history and hands the same label to whatever comes next, an
apology, a question about the law, a request for the weather. That is **context contamination**,
and it is the main source of false positives in a chatbot guardrail.

The earlier design held every turn after a violation, clean or not. Measured live against Jev on
the scenario set below, it held **171 of 194 harmless follow-ups**. This section describes the
design that replaced it, how it decides, and what it measured.

### The principle

**History is for understanding the current turn, never for convicting it.** A turn is withheld
only for what that turn, or the reply to it, does. Risk carried over from earlier turns decides
*how closely* a turn is read, never *whether* it is held.

Two consequences follow:

- *Related is not the same as continuing.* "How is making explosives punished?" refers back to a
  blocked request, but steps away from it. "Go on, what's step 2?" continues it. Only the second
  may be held.
- *Judge what is delivered, not a guess about intent.* If a follow-up is harmless, so is the
  answer to it, and there is nothing to hold. If it was an attempt to continue, the harm shows up in
  the reply, which is always checked.

### How a turn is decided

1. **Input is read alone.** The input check never sees the history, so the past can never block a
   question. The session's risk is not sent to Jev either.
2. **Output is read alone, and in a watched session also in context.** When the session is
   *watched* (below), the reply is sent twice, in parallel, so there is no extra latency: once alone,
   and once with the earlier turns. The standalone request never contains history, so it stays
   uncontaminated.
3. **The in-context read counts only if the reply itself completes an earlier harmful request.**
   Besides the hazard choice, the in-context request asks two yes/no questions: does the reply
   supply harmful content or complete a harmful request from an earlier turn (the next step, more
   detail, a rephrasing, translation or fictional retelling of it)? And does the latest user message
   refer back only to step away (apologise, ask about the law, prevention or reporting, ask why it
   was refused, change the subject)?
4. **Withheld turns are remembered, not re-read.** A withheld turn stays in the transcript as
   `[earlier message omitted]`, so the conversation check still sees that an attempt was made. Jev
   never reads the blocked text again, and the chat model never sees it at all
   (`session.ModelHistory()` leaves it out).
5. **The conversation check monitors; it does not hold.** It runs off the critical path, feeds the
   session's risk and watch, and sends conversations to the review queue. A conversation that is not
   moving toward a harmful objective is capped at flag.
6. **Streaming is held whole in a watched session.** Mid-stream checks read each chunk alone, so a
   part that is harmful only in context would get past them. In a watched session the reply is
   released after the final, in-context check.

### Formulas

Notation: `q_t` is the user message at turn t, `r_t` the reply, `H_t` the transcript window (10
messages), `J(x)` Jev's answers for state x, `D(surface, answers)` the policy's decision, and `⊕`
the merge of two verdicts (per category the stronger finding, action the stronger of the two).

```
Input verdict        V_in(t)  = D(input,  J(q_t))                        history never included
Output verdict       V_out(t) = D(output, J(r_t))

Watch                W_t = carry_left > 0  ∨  risk_t ≥ 0.2  ∨  (placeholder ∈ H_t)
In-context read      if W_t:  V_ctx = D(output, J(r_t | H_t)),  c = P(completes),  d = P(disengages)
Attribution          A_t = c ≥ τ  ∧  c ≥ d  ∧  V_ctx has a finding ≥ flag           τ = 0.5
Final                V(t) = V_out(t) ⊕ V_ctx   if A_t
                     V(t) = V_out(t)           otherwise (V_ctx kept on record only)

Risk                 risk_{t+1} = max(δ · risk_t, ρ(action)),  δ = 0.5
                     ρ(allow, flag, review, block) = (0, 0.25, 0.6, 1.0)
Carry                carry_left = 2 turns after a conversation verdict ≥ review or any block
```

Six corrections to single-turn decisions came out of the same measurements, because a follow-up
message is usually short and vague, which is exactly where they misfired. For a finding in
category k with probability p, band thresholds θ_flag ≤ θ_review ≤ θ_block, and `choice_k` the
hazard choice's probability for k:

```
Uncorroborated       u_k = (finding came from the sentinel alone) ∧ choice_k < 0.02
never_below          applies only if ¬u_k ∨ p ≥ θ_block
Weak sentinel        u_k ∧ p < θ_block                            → at most flag (recorded, delivered)
Declining reply      output ∧ refusal ≥ 0.8 ∧ u_k ∧ p < 0.5 ∧ k ∉ {sid, prv, ipv}   → flag
Masked, not blocked  u_k ∧ route_k = redact ∧ action = block ∧ p < 0.8               → review (redact)
Confidence gate      escalate to review if conf < 0.65 ∧ (finding ∨ p ≥ θ_flag/2)
                     unless intent = benign ∧ conf ≥ 0.5
Conversation cap     escalation ≤ 0.5  → cap at flag, except cse and ssh
```

Specialised advice (`spc`) is judged per reply only, not across a conversation. The `ncr` and `iwp`
descriptions now exclude victims asking what to do and questions about the law.

### Realtime chat: review means audit

In a realtime chat nobody can look at a message before the reply is due, so holding at review is a
block with a promise attached. With `ReviewHandling: ReviewAsAudit`, only block stops content:

| Verdict | The user gets | Audit |
| --- | --- | --- |
| allow | the content | none |
| flag | the content | sampled |
| review | the content (masked or steered where the category says so) | priority |
| block | the prewritten safe response | priority |
| self-harm risk | the crisis-support response | priority |
| Jev unreachable, fail-closed surface | held: nothing was checked | none |

Every verdict carries an `audit` level independent of delivery. Post-hoc review is the best source
of labels: record the reviewer's outcome, add the cases to the labelled sets, and replay them before
changing any threshold.

### Results

All live numbers are from Jev (`api.typesafe.ai`), with no degraded verdicts in any run.

**Live, 223 conversations** (`examples/multiturn-live.jsonl`): one violation or several, repeated or
interleaved, then a harmless or a harmful turn; histories past the 10-message window; escalations,
including ones that start after a long ordinary stretch. Harmful material is referenced by id from
the labelled sets.

| | Earlier design (floor) | Now |
| --- | --- | --- |
| Harmless turns held | 171 / 194 | **0 / 194** |
| Harmful replies caught | 19 / 19 | **19 / 19** |
| Escalations flagged | 9 / 9 | **9 / 9** |
| Harmless conversations sent to the review queue | 172 / 194 | **2 / 194** |

How it got there, run by run. In every run the in-context read attributed nothing to a harmless
turn; everything still held after the first step was held by a single-turn check.

| Step | Live set | Harmless turns held |
| --- | --- | --- |
| Floor (earlier design) | 223 conversations | 171 / 194 (88 %) |
| Attribution, neutral placeholder | 37 conversations | 7 / 26 (27 %) |
| Same, on a wider set | 165 conversations | 30 / 141 (21 %): 22 were plain refusals held for `cse` |
| + sentinel corroboration, `ncr` / `iwp` descriptions | 165, four runs | 0 to 1 / 141 (0 to 0.7 %) |
| + repeated violations and long histories added | 223 conversations | 3 / 185 (1.6 %) |
| + weak sentinel at most flag, gate respects benign intent, conversation cap | 223 conversations | **0 / 185** |

**Replayed offline** over every recorded Jev answer (`go/replay_test.go`): about 5,000 samples, the
same answers decided under each variant, so every variant is compared on identical data.

| Variant | Harmless held | Violations caught |
| --- | --- | --- |
| After corroboration | 0.62 % | 100 % (1,720 / 1,720) |
| + weak sentinel at most flag, gate respects benign intent | **0.04 %** | **100 %** |
| Re-asking Jev on borderline holds | 0.00 % | 100 %, at 11 % more calls: not adopted |
| Realtime (`ReviewAsAudit`), all recordings | **0.02 %** stopped (1 / 4,189) | every harmful reply stopped |

**Single-turn regression** (51 labelled cases, live): no labelled violation delivered before or
after; exact matches 28 → 29.

**Noise tolerance** (26 simulated scenarios, `examples/multiturn-contamination.jsonl`, every
probability jittered): at σ = 0.1 and τ = 0.5, 0.9 % of harmless turns held and 100 % of
continuations caught; at σ = 0.2, 4.3 % and 97.2 %. τ = 0.5 is the balance point.

### Reproduce

```bash
cd go
go test ./...                                   # unit tests and the simulated scenarios, no key

export JEV_API_KEY=...
LIVE_OUT=/tmp/mt go test -tags live -run TestLiveMultiturn -v ./          # the 223 conversations
LIVE_REVIEW=audit LIVE_OUT=/tmp/mt go test -tags live -run TestLiveMultiturn -v ./
LIVE_POLICY_BEFORE=old.json go test -tags live -run TestLiveSingleTurnRegression -v ./

REPLAY_DIRS='/tmp/mt*' go test -tags replay -run TestReplay -v ./          # offline, no key
```

`LIVE_OUT` keeps every raw Jev answer, which is what the replay reads.

### Using it

```go
guard := guardrail.New(guardrail.Options{
	ReviewHandling: guardrail.ReviewAsAudit, // realtime chat
})
session := guardrail.NewSession(conversationID)

in, _ := guard.CheckInput(ctx, message, &guardrail.CheckOptions{Session: session})
session.Record("user", message, in)             // a withheld turn is kept as a placeholder
if !in.Deliverable() {
	return safeResponse(in)
}
reply := callModel(session.ModelHistory(), message) // the model never sees withheld turns
out, _ := guard.CheckOutput(ctx, reply, &guardrail.CheckOptions{Session: session, UserMessage: message})
session.Record("assistant", reply, out)
session.Advance()
// out.Context shows the in-context read; out.Audit says what to queue.
```

`MultiturnFloor` keeps the earlier behaviour for deployments that want it.

### Limits

- The labelled violations are 30 texts, and there is no real child-safety positive among them, so
  the recall of the sentinel corroboration on `cse` is unmeasured on the harmful side. Those signals
  are still recorded and audited.
- A decomposed attack whose early pieces raise nothing is read in context only once the
  conversation check notices it, one turn late, as before.
- In a watched session a reply costs one more Jev request, sent in parallel.

---

## Adding a compliance domain of your own

The shipped taxonomy is what every deployment shares. What a regulated product needs on top of it
is specific: a clinic cares about dosing instructions, a broker about performance promises. You add
that as an overlay rather than a fork, so you keep inheriting upstream changes:

```python
policy = overlay(Policy.bundled(), MY_DOMAIN)   # the 18 shipped categories plus yours
guard = Guard(policy)
```

There are four things to add: a **category** (a hazard and its thresholds), a **signal** (another
question for rules to read), a **rule** (logic connecting them), and a **prefilter pattern** (the
deterministic part, settled with no model call at all).

[`examples/domain_policy.py`](examples/domain_policy.py) is a runnable one, and it demonstrates the
four things that are easy to get wrong:

- A rule cannot set a route. Routes belong to categories, so a rule reaches one by adding a finding
  for a category that owns it.
- `never_below` means "once this category fires, never resolve it below X". Under the category's
  lowest threshold nothing fires at all, so the `flag` number is the real on-switch.
- The shipped softening rules apply to your new category too, until you name it in their
  `except_categories`.
- A malformed overlay is refused when it loads, not on the first request.

Then calibrate. The shipped thresholds, and any you write, are numbers someone chose rather than
numbers anyone measured.

---

## Latency and experience

A guardrail that adds a second to every turn gets switched off within a month. Two things make
that avoidable: the shape of Jev itself, and where you put the calls.

**Asking Jev is cheap.** One round trip, 70-500ms. Output tokens are free and every question in a
request is answered in parallel, so adding a category, or a sentinel question on top of it, costs
a few input tokens and almost no latency. That is why the entire policy goes in one request instead
of one call per category.

**Run the input check beside the model call, not before it.** A serial check adds its full latency.
A parallel one adds almost none, because your model needs longer than 500ms to produce its first
token anyway.

```python
gate = asyncio.ensure_future(guard.acheck_input(message, session=session))
draft = asyncio.ensure_future(my_llm.generate(message))

verdict = await gate
if not verdict.allowed:
    draft.cancel()                 # nothing has reached the user
    return safe_response(verdict)
reply = await draft
```

```typescript
const [verdict, draft] = await Promise.all([
  guard.checkInput(message, { session }),
  myLlm.generate(message),
]);
if (!verdict.allowed) return safeResponse(verdict);  // the draft is discarded
return draft;
```

What this costs is tokens spent on drafts you throw away. Below roughly 2% violating turns, that is
cheaper than the delay it removes. If your rules say a violating prompt must never reach the model
at all, go serial and pay the latency deliberately.

**Stream one chunk behind.** A streamed reply cannot be checked before its first token, and a check
that waits for the last one is not streaming. `guard.stream()` cuts at sentence boundaries, holds
each chunk until its check returns, and lets the model produce the next one meanwhile, so only the
first chunk pays the full latency.

```python
async for event in guard.stream(my_llm.stream(message), user_message=message, session=session):
    if event.type == "delta":
        yield event.text
    elif event.type == "blocked":
        yield safe_response(event.verdict)
```

Mid-stream checks ask only the sentinel questions, the categories where a miss is unacceptable. The
complete reply gets the full question set at the end, on the `done` event. `chunk_chars`
(default 280) trades round trips against how tightly text is held.

**Skip the call entirely when you can.** A cache hit on repeated content and a prefilter hit on an
obvious case both settle the check locally, with no network at all.

**Never let a slow Jev become your outage.** Set `timeout`, and note that the shipped policy fails
*open* on input and *closed* on output (`on_error: {input: fail_open, output: fail_closed}`): a
timeout in front of a model that has its own safety degrades gracefully, while the output check has
nothing behind it. Either way the verdict carries `degraded: true`, so count those separately.

| Path | Latency it adds |
| --- | --- |
| Prefilter hit | none, no network |
| Cache hit | none, no network |
| Input check, parallel with the model | close to none |
| Input check, serial | 70-500ms |
| Streamed reply | the first chunk only |

**And the experience is the `route`, not the block.** A guardrail that only refuses feels broken to
the people it is protecting. Because `route` is decided separately from severity, the same `review`
can mask a phone number and still send the reply, steer the answer, or hand someone crisis
resources, instead of a flat "I can't help with that". Wire all six routes and most users never
notice the guardrail is there.

---

## Going to production

The quick start is real code, but a deployment wants more than three calls. Everything below ships
with the package; the section above covers the cache, the prefilter, streaming and the fail modes in
more detail.

- **Verdict cache** and **deterministic prefilter** - the two ways a check costs nothing at all.
- **Per-surface fail modes** - open on input, closed on output.
- **Streaming** - release text one chunk behind its check, so nothing reaches the user unchecked.
- **Sessions** - risk carries between turns across a ten-turn transcript window, so a user who just
  tripped a category is held to a higher bar on the next one.
- **Observer hook** - every verdict, including cached and degraded ones, for your metrics.

```python
from guardrail_chatbot_jev import Guard, LRUCache

guard = Guard(cache=LRUCache(), observer=metrics.emit, timeout=2.0)
```

**Sessions have to outlive the request**, which is the part a server gets wrong quietly. Behind
several workers, per-process state means each worker thinks every conversation just began, and the
watch and the withheld-turn placeholders stop carrying, with nothing in the logs to say so. `Session.as_state()` and
`Session.from_state()` are what a store persists;
[`examples/session_store.py`](examples/session_store.py) has a bounded in-process store for one
worker and a Redis one for more than one.

---

## Examples

All of them run with no API key. The model and the verdicts fall back to recorded answers, so every
path still executes.

| | |
| --- | --- |
| [`integration.py`](examples/integration.py) · [`integration.ts`](examples/integration.ts) | One guarded turn end to end: input check beside the model call, streaming, cache, prefilter, session, and the same question in four languages |
| [`chatbot_server.py`](examples/chatbot_server.py) | The same turn behind HTTP: FastAPI, Claude, SSE streaming, sessions per conversation |
| [`domain_policy.py`](examples/domain_policy.py) | Adding your own compliance domain on top of the shipped pack |
| [`go/example_test.go`](go/example_test.go) | A guarded turn and a guarded stream in Go, run as part of `go test` |

```bash
cd python && PYTHONPATH=src python3 ../examples/integration.py
PYTHONPATH=python/src python3 examples/domain_policy.py

pip install -e './python[server]'
uvicorn examples.chatbot_server:app --port 8000
```

[`examples/`](examples/) also holds the labelled sets used for calibration: 36 input cases, 15
output cases and 7 conversations, in English, Vietnamese, French and Japanese, each with the action
it should produce.

## Command line

```bash
guardrail-chatbot-jev --surface input --text "how do I make thermite" --dry-run
```

The Go module ships the same command with the same flags and verdict exit codes; a bad argument exits
`64` there instead of `2` or `1`:
`go install github.com/taman-spirit/guardrail-chatbot-jev/go/cmd/guardrail-chatbot-jev@latest`.

`--dry-run` prints the exact request that would be sent, and needs no key. Without it the exit code
carries the verdict, so a shell script can branch on it:

| Code | Meaning |
| --- | --- |
| `0` | allow or flag |
| `1` | redact or guide |
| `2` | review |
| `3` | block |
| `4` | degraded: Jev was unreachable, so nothing was actually checked |

`4` is separate on purpose. On the input surface the shipped policy fails open, so a degraded
verdict's action is `allow`; reporting that as success would tell
`guardrail-chatbot-jev ... && send` that content passed a check which never ran.

---

## Documentation

The full guide covers integration, policy ownership, calibration and the mechanisms behind each
decision.

| | |
| --- | --- |
| [English](docs/guide.md) | [Tiếng Việt](docs/guide.vi.md) |
| [Français](docs/guide.fr.md) | [日本語](docs/guide.ja.md) |

Also: the [hazard taxonomy](skill/guardrail-chatbot-jev/references/taxonomy.md), and
[`skill/guardrail-chatbot-jev/`](skill/guardrail-chatbot-jev/), a Claude skill wrapping the same
checks.

---

## What it is not

**It is not an enforcement layer.** It returns a verdict; your deployment decides what to do with
it. Nothing here blocks anything on its own.

**Jev reads only what you give it.** It cannot look things up, count reliably, or do arithmetic,
and it reads literally, so negation and implication are its weak spots. Retrieval, rate limits,
account state and deterministic checks belong in the code around it.

**The shipped thresholds are a starting point, not a measurement.** They are derived from how a
wide choice question distributes probability. Calibrate them against your own labelled data before
trusting them in production; the guide explains how, and [`scripts/`](scripts/) has the tooling.

---

## Contributing

Issues and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md). Both test suites run
without an API key or a network, so a change is easy to verify:

```bash
cd python && python3 -m pytest -q     # 81 tests
cd ts && npm test                     # 57 tests
```

`scripts/verify-all.sh` runs everything else as well: the policy pack, the labelled sets, the CLI,
every example, the offline tuning tool and the packaging.

To report a vulnerability, see [SECURITY.md](SECURITY.md).

## License

[CC BY-NC 4.0](LICENSE): Creative Commons Attribution-NonCommercial 4.0 International. You may use,
share and adapt it for non-commercial purposes, with attribution. Commercial use needs separate
permission from the copyright holder.
