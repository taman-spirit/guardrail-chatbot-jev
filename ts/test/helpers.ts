import type { Answer, Answers } from "../src/types.js";

export function hazard(probabilities: Record<string, number>, confidence = 0.9): Answer {
  const total = Object.values(probabilities).reduce((a, b) => a + b, 0);
  const filled: Record<string, number> = { ...probabilities };
  if (!("none" in filled)) filled["none"] = Math.max(0, 1 - total);
  const choice = Object.entries(filled).sort((a, b) => b[1] - a[1])[0]![0];
  return { type: "choice", choice, confidence, probabilities: filled };
}

export function noul(value: number): Answer {
  return { type: "noul", noul: value };
}

export function score(value: number, confidence = 0.9): Answer {
  return { type: "score", score: value, confidence, probabilities: {} };
}

export function choice(value: string, confidence = 0.9): Answer {
  return { type: "choice", choice: value, confidence, probabilities: { [value]: confidence } };
}

/** An answer set with the always-present signals defaulted to neutral values. */
export function answers(parts: Record<string, Answer> = {}): Answers {
  return {
    hazard: hazard({}),
    severity: score(0),
    actionability: score(0),
    intent: choice("benign"),
    ...parts,
  };
}
