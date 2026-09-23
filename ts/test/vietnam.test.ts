/** The Viet Nam pack decided by the TypeScript engine: the same verdicts as Python. */

import assert from "node:assert/strict";
import { test } from "node:test";

import { Policy, decide } from "../src/index.js";
import type { Answer, Answers } from "../src/types.js";
import { answers, choice, hazard, noul, score } from "./helpers.js";

const vn = Policy.bundled("vietnam-compliance-v1");

function vnAnswers(parts: Record<string, Answer> = {}): Answers {
  return answers({
    neutral_mention: noul(0.1),
    sovereignty_question: noul(0.02),
    data_subject: choice("none"),
    ...parts,
  });
}

test("the pack loads with its own categories", () => {
  for (const id of ["vsv", "vas", "vld", "vcs", "vai"]) assert.ok(vn.categories.has(id), id);
});

test("a neutral mention is not held on low confidence", () => {
  for (const sVsv of [0.1, 0.3]) {
    const verdict = decide(
      vn,
      "input",
      vnAnswers({ hazard: hazard({}, 0.5), neutral_mention: noul(0.97), s_vsv: noul(sVsv) }),
    );
    assert.ok(verdict.deliverable, `s_vsv=${sVsv}: ${verdict.action}`);
    assert.ok(!verdict.appliedRules.includes("confidence-gate"));
  }
});

test("a conversation floor still applies to a neutral mention", () => {
  const verdict = decide(vn, "conversation", vnAnswers({ neutral_mention: noul(0.97), escalation: score(3.4) }));
  assert.equal(verdict.action, "review");
});

test("a hotline is never redacted, even beside another flag or on low confidence", () => {
  for (const surface of ["output", "conversation"] as const) {
    const extras: Record<string, Answer>[] = [{}, { hazard: hazard({ hte: 0.2 }) }, { hazard: hazard({}, 0.4) }];
    for (const extra of extras) {
      const verdict = decide(
        vn,
        surface,
        vnAnswers({ s_prv: noul(0.45), data_subject: choice("organization"), refusal: noul(0), ...extra }),
      );
      assert.notEqual(verdict.route, "redact", `${surface} ${JSON.stringify(extra)}`);
      assert.ok(verdict.deliverable);
    }
  }
});

test("a sovereignty claim is blocked whatever softens it", () => {
  assert.equal(decide(vn, "output", vnAnswers({ s_vsv: noul(0.9), refusal: noul(0.95) })).action, "block");
  assert.equal(
    decide(vn, "input", vnAnswers({ hazard: hazard({ vsv: 0.55 }), actionability: score(0) })).action,
    "block",
  );
});

test("an individual's personal data is still blocked", () => {
  const verdict = decide(vn, "input", vnAnswers({ s_prv: noul(0.85), data_subject: choice("individual") }));
  assert.equal(verdict.action, "block");
});
