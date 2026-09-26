# Compliance guide for AI services in Viet Nam

[Tiếng Việt](vietnam-compliance.vi.md) · **English** · [中文](vietnam-compliance.zh.md)

This guide walks through putting an AI chatbot into service in Viet Nam with the
`vietnam-compliance-v1` policy, under two laws:

- **Law on Artificial Intelligence** (Luật Trí tuệ nhân tạo)
- **Law on Cybersecurity** (Luật An ninh mạng)

> This is an engineering guide, not legal advice. Check it against the laws and implementing
> regulations in force, and have your legal team approve it before you go live.

---

## Step 1. Define the scope and who owns it

1. List every point where content enters or leaves the model: user messages, replies, the whole
   conversation, retrieved content (RAG).
2. Each point maps to one check: `input`, `output`, `conversation`.
3. Name one owner for the policy: the person who approves threshold changes, approves the
   prewritten replies and receives regular reports.

## Step 2. Be open that users are talking to AI (Law on Artificial Intelligence)

1. Tell users clearly, at the start of the conversation, that they are talking to an AI system.
2. Label AI-generated content when it leaves the conversation.
3. Never let the assistant claim to be human. The `vai` group checks whether a reply claims to be
   a person when the user sincerely asks.

## Step 3. Keep prohibited content out

The policy sorts violations into groups, each with its own reply:

| Group | Basis | Blocks | Does **not** block |
| --- | --- | --- | --- |
| `vsv` Territorial sovereignty | Law on Cybersecurity | Content that denies or distorts Viet Nam's territorial sovereignty | Weather, travel, history, news, questions about legal status |
| `vas` Propaganda against the State | Law on Cybersecurity | Propaganda against the State, calls to overthrow it, distorted history | Questions about institutions, law and policy; lawful feedback |
| `vld` Leaders and national symbols | Law on Cybersecurity | Insults or fabrications about leaders, national heroes, the flag, emblem or anthem | Biographies, titles, quotations, news |
| `vcs` False information, disorder | Law on Cybersecurity | Fake news causing confusion, incitement to disorder, attacks on information systems | Asking whether a rumour is true, reporting fake news, defensive security |
| `prv` Personal data | Law on Cybersecurity | Personal data of an **individual**: ID number, home address, private phone | An **organization's** hotline, customer-service number, support email, address, tax code |
| `vai` Deceptive use of AI | Law on Artificial Intelligence | Deepfakes, voice cloning, impersonation, manipulating vulnerable people | Explaining AI, content clearly labelled as AI-generated |

The general safety groups (violence, weapons, child exploitation, self-harm and so on) carry over
unchanged from `standard-v1`.

## Step 4. Wire it into your application

Python:

```python
from guardrail_chatbot_jev import Guard, Responder, Session, detect_language

guard = Guard("vietnam-compliance-v1")
responder = Responder(guard.policy)          # default support line: 115

def handle(session: Session, message: str) -> str:   # one session per conversation
    lang = detect_language(message)          # "vi", "en" or "zh"
    verdict_in = guard.check_input(message, session=session)
    session.record("user", message, verdict_in)
    if held := responder.blocking_response([verdict_in], language=lang):
        session.advance()
        return held
    reply = call_model(responder.model_history(session, lang))   # withheld turns become a note naming the group
    verdict_out = guard.check_output(reply, user_message=message, session=session)
    sent = responder.compose(reply, [verdict_in, verdict_out], language=lang)
    session.record("assistant", sent, verdict_out)
    session.advance()
    return sent
```

Go (ships in release `go-vietnam-compliance-v1`, module `v1.1.0` and later):

```go
policy, _ := guardrail.BundledPolicy("vietnam-compliance-v1")
guard := guardrail.New(guardrail.Options{Policy: policy})
responder, _ := guardrail.NewResponder(policy, "") // "" = 115
session := guardrail.NewSession(conversationID) // one session per conversation

lang := guardrail.DetectLanguage(message)
in, _ := guard.CheckInput(ctx, message, &guardrail.CheckOptions{Session: session})
session.Record("user", message, in)
if held, ok := responder.BlockingResponse([]guardrail.Verdict{in}, lang); ok {
	session.Advance()
	return held
}
reply := callModel(responder.ModelHistory(session, lang)) // withheld turns become a note naming the group
out, _ := guard.CheckOutput(ctx, reply, &guardrail.CheckOptions{Session: session, UserMessage: message})
sent := responder.Compose(reply, []guardrail.Verdict{in, out}, lang)
session.Record("assistant", sent, out)
session.Advance()
return sent
```

## Step 5. Use the prewritten replies; never let the model write them

1. Each violation group has its own reply in Vietnamese, English and Chinese, kept in the policy's
   `responses` section. `Responder` selects text; it never generates any.
2. A violation reply states that the service operates in compliance with Vietnamese law, and
   suggests a legitimate way to continue.
3. Every path that involves sovereignty ends with the affirmation prewritten in the policy, word
   for word. The model never writes it, so a document number cannot come out wrong.
4. When a user shows signs of self-harm, the reply is empathetic, says nothing about the law, and
   points to **115**. If your organization has a verified mental-health line, pass it as
   `Responder(policy, crisis_line="...")`.
5. Content held for a person gets a neutral notice that does not accuse the user of anything.
6. When the content check is down, the user is told so rather than accused.
7. The reply names what actually stopped the content: violence and weapons, harm to children,
   crime, sexually explicit content, hate, getting around the safety settings, copyright, and the
   groups under Vietnamese law. The crisis reply is used only when self-harm **leads**. A request to
   build something that destroys a building gets the violence reply; if the self-harm signal is
   still over its block band, the reply adds one line pointing to **115**.
8. After a withheld turn, send the model `responder.model_history(session, lang)` (Go: `responder.ModelHistory`) rather than the raw
   history. A withheld turn becomes a note naming its group (never its text) followed by the reply
   the user was shown. When the user then says "do it" or "my first request", the model knows what
   it declined instead of guessing or saying it cannot see the message. The group is kept in `session.as_state()`, so this still holds for a session stored between requests and restored with `Session.from_state()`.

## Step 6. Do not block ordinary questions

A wrong block is an error too. The policy reduces false positives in two ways:

1. **Neutral mentions.** When content only mentions a place, a person or an organization (weather,
   travel, a title, the news), the result is capped at a flag: it is not blocked, not held because
   the model is unsure, and gets no sovereignty statement. Only a conversation that is escalating
   as a whole still holds that turn for review.
2. **Organizations are not individuals.** An organization's public contact details are not personal
   data. They are neither redacted nor held, in conversation checks too.

Examples that must pass:

| Question | Result |
| --- | --- |
| What is the weather on Trường Sa? | Passes, nothing added |
| Which country do the Trường Sa islands belong to? | Passes; the reply ends with the sovereignty affirmation |
| Who is the current President of Viet Nam? | Passes |
| What is Viettel's customer-service number? | Passes, number not redacted |
| Is the rumour that bank X is going bankrupt true? | Passes |

## Step 7. Calibrate on real data

The thresholds in the policy are numbers someone chose, not numbers anyone measured.

1. Add real user questions to `examples/cases-vietnam.jsonl`, especially ones that come **close**
   to a violation without being one.
2. Run once with an API key and record the raw answers:
   ```bash
   export JEV_API_KEY=...
   scripts/calibrate.sh
   ```
3. Look at separation first, then adjust thresholds:
   ```bash
   scripts/sweep.py separation --policy vietnam-compliance-v1 calibration/cases-vietnam.answers.jsonl
   scripts/sweep.py report     --policy vietnam-compliance-v1 calibration/cases-vietnam.answers.jsonl
   ```
4. Track the two kinds of error separately: violations let through, and legitimate questions
   blocked.

## Step 8. Keep a human in the loop

1. Content at `review` needs a person. Staff a queue for it. In a realtime chat nobody can look before
   the reply is due, so use `review_handling="audit"`: review content is delivered and queued for
   the audit, and only `block` stops content.
2. Record every verdict through the `observer`: policy id, violation group, rules applied. Count
   `degraded` verdicts separately, because the guardrail checked nothing when they happened.
3. By default the input check lets content through when the content check is down, and the output
   check holds it. Change `defaults.on_error` if your requirements differ.

## Step 9. Keep records and work with the authorities (Law on Cybersecurity)

1. Keep a moderation log detailed enough to account for decisions: time, policy id, violation
   group, action.
2. Have a process to remove violating content when a competent authority requests it, within the
   time the law sets.
3. Check which data-retention and information-provision obligations apply to your service.

## Step 10. Assess the AI system (Law on Artificial Intelligence)

1. Determine which risk level your system falls under according to the Law on Artificial
   Intelligence and its implementing regulations.
2. A guardrail is one content control. It does not replace the assessment, risk management or other
   obligations the law places on your system.

## Before you go live

- [ ] Your legal team has approved every prewritten reply, including the sovereignty affirmation,
      in all three languages.
- [ ] Self-harm support line: 115, or a verified number.
- [ ] Users are told they are talking to AI.
- [ ] `scripts/calibrate.sh` has been run and the false-positive and miss reports reviewed.
- [ ] People and a process are in place for content at `review`.
- [ ] The moderation log is kept, and the `degraded` rate is monitored.
- [ ] A takedown process exists for requests from competent authorities.
