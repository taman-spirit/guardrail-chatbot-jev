/** The decision engine, exercised against recorded Jev answers. */

import assert from "node:assert/strict";
import { test } from "node:test";

import { Guard, Policy, RecordedTransport, buildQuestions, decide, errorVerdict } from "../src/index.js";
import { HAZARD, NONE_LABEL } from "../src/questions.js";
import { answers, choice, hazard, noul, score } from "./helpers.js";

const policy = Policy.bundled();

test("a clean message is allowed", () => {
  const verdict = decide(policy, "input", answers());
  assert.equal(verdict.action, "allow");
  assert.equal(verdict.route, "deliver");
  assert.equal(verdict.findings.length, 0);
  assert.ok(verdict.allowed);
});

test("a sentinel overrides a diluted choice", () => {
  const verdict = decide(policy, "input", answers({ hazard: hazard({ cse: 0.09 }), s_cse: noul(0.82) }));
  assert.equal(verdict.action, "block");
  assert.equal(verdict.findings[0]?.category, "cse");
  assert.equal(verdict.findings[0]?.probability, 0.82);
});

test("never_below forces block on a weak signal", () => {
  const verdict = decide(policy, "input", answers({ hazard: hazard({ cse: 0.04 }) }));
  assert.equal(verdict.action, "block");
  assert.ok(verdict.findings[0]?.notes.includes("never below block"));
});

test("academic intent softens, but never for weapons", () => {
  const weapons = decide(
    policy,
    "input",
    answers({ hazard: hazard({ iwp: 0.5 }), s_iwp: noul(0.5), intent: choice("academic"), actionability: score(2) }),
  );
  assert.equal(weapons.action, "block");

  const fraud = decide(
    policy,
    "input",
    answers({ hazard: hazard({ ncr: 0.55 }), intent: choice("academic"), actionability: score(2) }),
  );
  assert.equal(fraud.action, "review");
  assert.ok(fraud.appliedRules.includes("academic-intent-softens"));
});

test("actionability moves the verdict both ways", () => {
  const base = { hazard: hazard({ vcr: 0.3 }), severity: score(2), intent: choice("seeking_information") };
  const talk = decide(policy, "input", answers({ ...base, actionability: score(0.5) }));
  assert.equal(talk.action, "flag");
  const recipe = decide(policy, "input", answers({ ...base, actionability: score(3) }));
  assert.equal(recipe.action, "block");
});

test("evasion raises a prompt-injection finding", () => {
  const verdict = decide(policy, "input", answers({ hazard: hazard({ ncr: 0.2 }), intent: choice("evasion") }));
  assert.ok(verdict.findings.some((f) => f.category === "pij"));
});

test("self-harm routes to crisis support", () => {
  const verdict = decide(policy, "input", answers({ hazard: hazard({ ssh: 0.2 }), s_ssh: noul(0.66) }));
  assert.equal(verdict.action, "block");
  assert.equal(verdict.route, "crisis_support");
  assert.equal(verdict.deliverable, false);
});

test("personal data in a reply is redacted, not withheld", () => {
  const verdict = decide(
    policy,
    "output",
    answers({ hazard: hazard({ prv: 0.3 }), s_prv: noul(0.35), refusal: noul(0.02) }),
  );
  assert.equal(verdict.action, "review");
  assert.equal(verdict.route, "redact");
  assert.ok(verdict.deliverable);
});

test("a refusal is not punished for naming the hazard", () => {
  const verdict = decide(
    policy,
    "output",
    answers({ hazard: hazard({ vcr: 0.62 }), refusal: noul(0.95), actionability: score(0) }),
  );
  assert.equal(verdict.action, "flag");
  assert.ok(verdict.appliedRules.includes("refusal-caps-output-risk"));
});

test("a refusal does not excuse a leaked system prompt", () => {
  const verdict = decide(
    policy,
    "output",
    answers({ hazard: hazard({ sid: 0.5 }), s_sid: noul(0.7), refusal: noul(0.95) }),
  );
  assert.equal(verdict.action, "block");
});

test("an ungrounded reply raises misinformation", () => {
  const verdict = decide(policy, "output", answers({ groundedness: score(0.5), refusal: noul(0) }));
  assert.equal(verdict.action, "flag");
  assert.ok(verdict.findings.some((f) => f.category === "mis"));
});

test("a grounded reply stays clean", () => {
  assert.equal(decide(policy, "output", answers({ groundedness: score(3), refusal: noul(0) })).action, "allow");
});

test("a crescendo escalates a conversation", () => {
  const verdict = decide(
    policy,
    "conversation",
    answers({ hazard: hazard({ ncr: 0.32 }), crescendo: noul(0.8), escalation: score(3.4) }),
  );
  assert.ok(["review", "block"].includes(verdict.action));
  assert.ok(verdict.findings.some((f) => f.category === "pij"));
});

test("low confidence escalates to review", () => {
  const verdict = decide(policy, "input", answers({ hazard: hazard({ ncr: 0.32 }, 0.2) }));
  assert.equal(verdict.action, "review");
  assert.ok(verdict.appliedRules.includes("confidence-gate"));
});

test("low confidence with nothing nearby stays allowed", () => {
  assert.equal(decide(policy, "input", answers({ hazard: hazard({ ncr: 0.01 }, 0.2) })).action, "allow");
});

test("low confidence never downgrades a block", () => {
  const verdict = decide(
    policy,
    "input",
    answers({ hazard: hazard({ vcr: 0.9 }, 0.1), actionability: score(2) }),
  );
  assert.equal(verdict.action, "block");
  assert.equal(verdict.route, "safe_response");
});

test("output-only categories are not scored on a user message", () => {
  const verdict = decide(policy, "input", answers({ hazard: hazard({ mis: 0.9 }) }));
  assert.ok(!verdict.findings.some((f) => f.category === "mis"));
});

test("a softening rule never erases a finding", () => {
  const verdict = decide(
    policy,
    "input",
    answers({ hazard: hazard({ ncr: 0.16 }), actionability: score(0), intent: choice("academic") }),
  );
  assert.equal(verdict.action, "flag");
  assert.ok(verdict.findings.some((f) => f.category === "ncr"));
});

test("action and route are separate axes", () => {
  const verdict = decide(
    policy,
    "output",
    answers({ hazard: hazard({ spc: 0.55 }), refusal: noul(0), actionability: score(2) }),
  );
  assert.equal(verdict.action, "review");
  assert.equal(verdict.route, "guide");
  assert.ok(verdict.deliverable);
});

test("an unreachable model fails closed on output", () => {
  // The output check is the last line, so an unreachable Jev holds the reply.
  const verdict = errorVerdict(policy, "output", new Error("boom"));
  assert.equal(verdict.action, "review");
  assert.equal(verdict.route, "human_review");
  assert.ok(!verdict.deliverable, "a degraded verdict must not read as permission to send");
  assert.ok(verdict.degraded);
  assert.ok(!verdict.allowed);
});

test("every surface builds a valid question set", () => {
  for (const surface of ["input", "output", "conversation"] as const) {
    const questions = buildQuestions(policy, surface);
    const hazardQuestion = questions[HAZARD];
    assert.ok(hazardQuestion && hazardQuestion.type === "choice");
    assert.ok(NONE_LABEL in hazardQuestion.criteria);
    assert.ok(Object.keys(hazardQuestion.criteria).length <= 256);
    for (const [name, question] of Object.entries(questions)) {
      assert.ok(question.instructions, name);
      if (question.type === "score") {
        assert.ok(question.criteria.length >= 2 && question.criteria.length <= 10, name);
      }
    }
  }
});

test("groundedness appears only with context", () => {
  assert.ok(!("groundedness" in buildQuestions(policy, "output")));
  assert.ok("groundedness" in buildQuestions(policy, "output", { hasContext: true }));
});

test("bad thresholds are rejected", () => {
  assert.throws(
    () =>
      new Policy({
        id: "broken",
        version: "0",
        signals: {},
        categories: { x: { name: "x", description: "", thresholds: { default: { block: 0.1, review: 0.5, flag: 0.2 } } } },
      }),
    /not ordered/,
  );
});

test("a guard sends one request per check", async () => {
  const transport = new RecordedTransport({
    hazard: { type: "choice", choice: "none", confidence: 0.95, probabilities: { none: 0.95 } },
  });
  const guard = new Guard({ policy, transport });
  const verdict = await guard.checkInput("hello there");
  assert.equal(verdict.action, "allow");
  assert.equal(transport.calls.length, 1);
  assert.equal((transport.calls[0]!.state as Record<string, unknown>)["user_message"], "hello there");
});

test("preview needs no api key", () => {
  const preview = new Guard({ policy }).preview("input", { user_message: "hi" });
  assert.ok(HAZARD in (preview["questions"] as Record<string, unknown>));
});

test("the shipped pack has not drifted from the canonical one", async () => {
  const { readFileSync } = await import("node:fs");
  // Tests run compiled, from dist-test/test/, so the repository root is three levels up.
  const canonical = new URL("../../../policies/standard-v1.json", import.meta.url);
  const shipped = new URL("../src/policies/standard-v1.json", import.meta.url);
  assert.deepEqual(
    JSON.parse(readFileSync(shipped, "utf8")),
    JSON.parse(readFileSync(canonical, "utf8")),
    "run scripts/sync-policies.sh",
  );
});

test("a sentinel probability is not read as doubt", () => {
  // A noul reports belief, not uncertainty. Treating its distance from 0.5 as confidence
  // double-counts the probability: every mid-range sentinel would escalate to review no matter
  // where the threshold sat, which makes the threshold meaningless for exactly the categories
  // that most need one.
  const verdict = decide(
    policy,
    "output",
    answers({ hazard: hazard({ prv: 0.05 }, 0.9), s_prv: noul(0.2), refusal: noul(0) }),
  );
  assert.equal(verdict.action, "flag");
  assert.ok(!verdict.appliedRules.includes("confidence-gate"));
  assert.equal(verdict.findings[0]?.source, "sentinel");
  assert.equal(verdict.findings[0]?.confidence, 0.9);
});

test("a sentinel still inherits a shaky request", () => {
  const verdict = decide(
    policy,
    "output",
    answers({ hazard: hazard({ prv: 0.05 }, 0.2), s_prv: noul(0.2), refusal: noul(0) }),
  );
  assert.equal(verdict.action, "review");
  assert.ok(verdict.appliedRules.includes("confidence-gate"));
});
