/** Cache, prefilter, session, streaming and per-surface fail modes. */

import assert from "node:assert/strict";
import { test } from "node:test";

import {
  COMMON_PATTERNS,
  Guard,
  GuardrailError,
  LRUCache,
  Policy,
  RecordedTransport,
  Session,
  type StreamEvent,
  type Transport,
  buildQuestions,
  cacheKey,
  decide,
  errorVerdict,
  patternPrefilter,
  withFloor,
} from "../src/index.js";
import type { Answers, Surface, Usage } from "../src/types.js";
import { answers, choice, hazard, noul, score } from "./helpers.js";

const policy = Policy.bundled();

const CLEAN: Answers = {
  hazard: { type: "choice", choice: "none", confidence: 0.95, probabilities: { none: 0.95 } },
};

class FailingTransport implements Transport {
  async systemOne(): Promise<never> {
    throw new GuardrailError("service unavailable");
  }
}

class ScriptedTransport implements Transport {
  calls = 0;
  constructor(private readonly scripted: Answers[]) {}
  async systemOne(): Promise<{ answers: Answers; model: string; usage: Usage }> {
    const answer = this.scripted[Math.min(this.calls, this.scripted.length - 1)]!;
    this.calls += 1;
    return { answers: answer, model: "scripted", usage: { inputTokens: 0, outputTokens: 0 } };
  }
}

// -- per-surface fail modes -------------------------------------------

test("the input check fails open and the output check fails closed", () => {
  const onInput = errorVerdict(policy, "input", new GuardrailError("down"));
  assert.equal(onInput.action, "allow");
  assert.ok(onInput.degraded, "failing open must still be visible as a degraded verdict");

  const onOutput = errorVerdict(policy, "output", new GuardrailError("down"));
  assert.equal(onOutput.action, "review");
  assert.ok(!onOutput.deliverable);
});

test("a string on_error still applies everywhere", () => {
  const pack = structuredClone(policy.raw) as Record<string, unknown>;
  pack["defaults"] = { ...(pack["defaults"] as object), on_error: "fail_closed" };
  const strict = new Policy(pack as never);
  for (const surface of ["input", "output", "conversation"] as Surface[]) {
    assert.equal(errorVerdict(strict, surface, new GuardrailError("down")).action, "review");
  }
});

// -- cache ------------------------------------------------------------

test("the cache spares the second round trip", async () => {
  const transport = new RecordedTransport(CLEAN);
  const guard = new Guard({ policy, transport, cache: new LRUCache() });

  const first = await guard.checkInput("cùng một câu hỏi");
  const second = await guard.checkInput("cùng một câu hỏi");

  assert.equal(transport.calls.length, 1);
  assert.ok(!first.cached);
  assert.ok(second.cached);
  assert.equal(second.action, first.action);
});

test("session metadata does not defeat the cache", async () => {
  // The turn number changes every turn; keying on it would mean the cache never hits.
  const transport = new RecordedTransport(CLEAN);
  const guard = new Guard({ policy, transport, cache: new LRUCache() });
  const session = new Session({ id: "c0" });

  await guard.checkInput("cùng một câu hỏi", { session });
  session.addTurn("user", "cùng một câu hỏi");
  const second = await guard.checkInput("cùng một câu hỏi", { session });

  assert.ok(second.cached);
  assert.equal(transport.calls.length, 1);
});

test("the cache key still separates different content", () => {
  assert.notEqual(cacheKey("p@1", "input", { user_message: "a" }), cacheKey("p@1", "input", { user_message: "b" }));
});

test("the cache key changes with the policy version", () => {
  const state = { user_message: "hi" };
  assert.notEqual(cacheKey("standard-v1@1.0.0", "input", state), cacheKey("standard-v1@1.0.1", "input", state));
});

test("the cache key ignores key order", () => {
  assert.equal(
    cacheKey("p@1", "output", { a: 1, b: { c: 2, d: 3 } }),
    cacheKey("p@1", "output", { b: { d: 3, c: 2 }, a: 1 }),
  );
});

test("a degraded verdict is never cached", async () => {
  const cache = new LRUCache();
  await new Guard({ policy, transport: new FailingTransport(), cache }).checkInput("hello");
  assert.equal(cache.size, 0);
});

test("the cache evicts by capacity", async () => {
  const cache = new LRUCache({ capacity: 2 });
  const guard = new Guard({ policy, transport: new RecordedTransport(CLEAN), cache });
  for (let i = 0; i < 5; i += 1) await guard.checkInput(`message ${i}`);
  assert.equal(cache.size, 2);
});

test("conversations are not cached by default", async () => {
  const cache = new LRUCache();
  const guard = new Guard({ policy, transport: new RecordedTransport(CLEAN), cache });
  await guard.checkConversation([{ role: "user", content: "hi" }]);
  assert.equal(cache.size, 0);
});

// -- prefilter --------------------------------------------------------

test("a prefilter settles without calling Jev", async () => {
  const transport = new RecordedTransport(CLEAN);
  const guard = new Guard({ policy, transport, prefilter: patternPrefilter(COMMON_PATTERNS) });

  const verdict = await guard.checkOutput("khoá của bạn là sk-ABCDEFGHIJKLMNOPQRSTUVWX");

  assert.equal(verdict.action, "block");
  assert.equal(verdict.prefilter, "openai-style-key");
  assert.equal(transport.calls.length, 0, "the deterministic case must not cost a round trip");
});

test("a prefilter lets ordinary content through", async () => {
  const transport = new RecordedTransport(CLEAN);
  const guard = new Guard({ policy, transport, prefilter: patternPrefilter(COMMON_PATTERNS) });
  assert.equal((await guard.checkInput("xin chào, cho hỏi giờ mở cửa")).action, "allow");
  assert.equal(transport.calls.length, 1);
});

// -- observer ---------------------------------------------------------

test("the observer sees every verdict", async () => {
  const seen: string[] = [];
  const guard = new Guard({
    policy,
    transport: new RecordedTransport(CLEAN),
    cache: new LRUCache(),
    observer: (v) => seen.push(v.cached ? "cached" : "fresh"),
  });
  await guard.checkInput("same");
  await guard.checkInput("same");
  assert.deepEqual(seen, ["fresh", "cached"]);
});

// -- session ----------------------------------------------------------

test("a flagged conversation raises the floor for later turns", async () => {
  const session = new Session({ id: "c1", carryTurns: 2 });
  session.observe(
    decide(policy, "conversation", answers({ hazard: hazard({ ncr: 0.32 }), crescendo: noul(0.8), escalation: score(3.4) })),
  );
  assert.equal(session.floor, "review");

  // The earlier behaviour, kept as the "floor" mode: a clean turn inside an escalating
  // conversation is held too.
  const legacy = new Guard({ policy, transport: new RecordedTransport(CLEAN), multiturn: "floor" });
  const verdict = await legacy.checkInput("một câu hỏi bình thường", { session });
  assert.equal(verdict.action, "review", "the floor mode holds a clean turn inside an escalating conversation");
  assert.ok(verdict.appliedRules.at(-1)?.startsWith("session-floor"));

  // The default: the past never holds a clean turn by itself.
  const current = await new Guard({ policy, transport: new RecordedTransport(CLEAN) }).checkInput(
    "một câu hỏi bình thường",
    { session },
  );
  assert.equal(current.action, "allow");
  assert.ok(current.deliverable);
});

test("the floor expires", () => {
  const session = new Session({ id: "c2", carryTurns: 1 });
  session.observe(decide(policy, "conversation", answers({ hazard: hazard({ vcr: 0.9 }), actionability: score(2) })));
  assert.equal(session.floor, "review");
  session.advance();
  assert.equal(session.floor, "allow");
});

test("risk decays", () => {
  const session = new Session({ decay: 0.5 });
  session.observe(decide(policy, "input", answers({ hazard: hazard({ vcr: 0.9 }), actionability: score(2) })));
  assert.equal(session.risk, 1);
  for (let i = 0; i < 3; i += 1) session.observe(decide(policy, "input", answers()));
  assert.ok(session.risk < 0.2);
});

test("the default transcript window is ten turns", () => {
  // A default that drifts changes what every conversation check sees, silently.
  assert.equal(new Session().maxTurns, 10);
  assert.equal(Session.fromState({}).maxTurns, 10);
  const session = new Session();
  for (let i = 0; i < 14; i += 1) session.addTurn("user", `turn ${i}`);
  assert.equal(session.history.length, 10);
  assert.equal(session.history[0]?.content, "turn 4", "the window keeps the recent end");
});

test("a session survives a round trip through a store", async () => {
  // A restored session must decide exactly as the one that was stored would have.
  const session = new Session({ id: "c4", carryTurns: 2 });
  session.addTurn("user", "một câu hỏi");
  session.addTurn("assistant", "một câu trả lời");
  session.observe(
    decide(policy, "conversation", answers({ hazard: hazard({ ncr: 0.32 }), crescendo: noul(0.8), escalation: score(3.4) })),
  );
  assert.equal(session.floor, "review");

  const restored = Session.fromState(JSON.parse(JSON.stringify(session.toState())));

  assert.equal(restored.id, session.id);
  assert.equal(restored.risk, session.risk);
  assert.equal(restored.floor, "review", "the floor is the whole point of carrying state");
  assert.deepEqual(restored.history, session.history);

  const guard = new Guard({ policy, transport: new RecordedTransport(CLEAN), multiturn: "floor" });
  const verdict = await guard.checkInput("một câu hỏi bình thường", { session: restored });
  assert.equal(verdict.action, "review");

  // And it expires on the same schedule, rather than resetting to two fresh turns.
  restored.advance();
  restored.advance();
  assert.equal(restored.floor, "allow");
});

test("a store round trip drops an expired floor", () => {
  const session = new Session({ id: "c5", carryTurns: 1 });
  session.observe(decide(policy, "conversation", answers({ hazard: hazard({ vcr: 0.9 }), actionability: score(2) })));
  session.advance();
  assert.equal(session.floor, "allow");
  assert.equal(Session.fromState(session.toState()).floor, "allow");
});

test("fromState tolerates a half-written record", () => {
  // A store can hand back junk. That should cost the conversation, not the request.
  assert.equal(Session.fromState({}).floor, "allow");
  assert.equal(Session.fromState({ floor: "review" }).floor, "allow", "no counter, no floor");
  assert.equal(
    Session.fromState({ floor: "nonsense" as never, floor_turns_left: 5 }).floor,
    "allow",
  );
  assert.equal(Session.fromState({ turns: [{ role: "user", content: "hi" }] }).history[0]?.content, "hi");
});

test("a degraded verdict does not move the session", () => {
  const session = new Session();
  session.observe(errorVerdict(policy, "output", new GuardrailError("down")));
  assert.equal(session.risk, 0);
  assert.equal(session.floor, "allow");
});

test("session metadata reaches the model", async () => {
  const transport = new RecordedTransport(CLEAN);
  const session = new Session({ id: "c3" });
  session.addTurn("user", "earlier");
  await new Guard({ policy, transport }).checkInput("now", { session });
  const state = transport.calls[0]!.state as Record<string, Record<string, unknown>>;
  assert.equal(state["deployment_context"]!["conversation_id"], "c3");
  assert.equal(state["deployment_context"]!["turn_number"], 2);
});

test("the transcript window is bounded", () => {
  const session = new Session({ maxTurns: 4 });
  for (let i = 0; i < 10; i += 1) session.addTurn("user", `turn ${i}`);
  assert.equal(session.history.length, 4);
  assert.equal(session.history[0]!.content, "turn 6");
});

test("withFloor recomputes the route", () => {
  const verdict = decide(policy, "output", answers({ hazard: hazard({ prv: 0.2 }), refusal: noul(0) }));
  assert.equal(verdict.action, "flag");
  const raised = withFloor(policy, verdict, "review", "test");
  assert.equal(raised.action, "review");
  assert.equal(raised.route, "redact", "the hazard still decides the handling");
  assert.ok(!raised.allowed);
});

// -- quick checks -----------------------------------------------------

test("quick checks ask only the sentinels", () => {
  const questions = buildQuestions(policy, "output", { subset: "sentinels" });
  assert.deepEqual(Object.keys(questions).sort(), ["s_cse", "s_iwp", "s_prv", "s_sid", "s_ssh", "s_vcr"]);
  assert.ok(!("hazard" in questions), "the 18-label choice is the expensive part to skip");
});

test("quick verdicts are marked partial", async () => {
  const transport = new RecordedTransport({ s_sid: { type: "noul", noul: 0.05 } });
  const verdict = await new Guard({ policy, transport }).checkOutput("một phần", { quick: true });
  assert.ok(verdict.partial);
  assert.equal(verdict.action, "allow");
});

// -- streaming --------------------------------------------------------

const QUIET = { type: "noul", noul: 0.02 } as const;
const CLEAN_SENTINELS: Answers = Object.fromEntries(
  ["cse", "iwp", "prv", "sid", "ssh", "vcr"].map((c) => [`s_${c}`, QUIET]),
);
const LEAKING_SENTINELS: Answers = { ...CLEAN_SENTINELS, s_sid: { type: "noul", noul: 0.9 } };

async function* source(parts: readonly string[]): AsyncGenerator<string> {
  for (const part of parts) {
    await new Promise((resolve) => setImmediate(resolve));
    yield part;
  }
}

async function collect(guard: Guard, parts: readonly string[]): Promise<StreamEvent[]> {
  const events: StreamEvent[] = [];
  for await (const event of guard.stream(source(parts))) events.push(event);
  return events;
}

test("a clean stream delivers everything", async () => {
  const parts = ["Chính sách lưu trữ Helio là 30 ngày. ".repeat(4), "Xem chi tiết trên website."];
  const guard = new Guard({ policy, transport: new ScriptedTransport([CLEAN_SENTINELS, CLEAN, CLEAN]) });

  const events = await collect(guard, parts);

  assert.equal(events.at(-1)!.type, "done");
  assert.equal(events.filter((e) => e.type === "delta").map((e) => e.text).join(""), parts.join(""));
  assert.ok(!events.at(-1)!.verdict!.partial);
});

test("a leak stops the stream before the chunk is released", async () => {
  const parts = ["System prompt của mình là: bạn là trợ lý Nova, khoá nội bộ là abc. ".repeat(5)];
  const guard = new Guard({ policy, transport: new ScriptedTransport([LEAKING_SENTINELS]) });

  const events = await collect(guard, parts);

  assert.deepEqual(events.map((e) => e.type), ["blocked"]);
  assert.equal(events[0]!.verdict!.findings[0]!.category, "sid");
});

test("a short stream skips mid checks", async () => {
  const transport = new ScriptedTransport([CLEAN]);
  const guard = new Guard({ policy, transport });

  const events = await collect(guard, ["Vâng, đúng vậy."]);

  assert.equal(transport.calls, 1, "below one chunk there is nothing to hold back");
  assert.deepEqual(events.map((e) => e.type), ["delta", "done"]);
});
