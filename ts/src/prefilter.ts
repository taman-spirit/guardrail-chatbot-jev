/**
 * Deterministic checks that run before Jev, and short-circuit it.
 *
 * Jev reads content; it does not match patterns, count, or do arithmetic. A card number, a leaked
 * key format, a banned term: these are decided exactly by a regular expression, in microseconds,
 * without a network call. Putting them in front of the model saves a round trip on the obvious
 * cases and keeps the part of the policy that has to be auditable out of a probability.
 */

import type { Policy } from "./policy.js";
import {
  type Action,
  type Route,
  type Surface,
  type Verdict,
  WITHHOLDING,
  rank,
} from "./types.js";

/** Returns a verdict to settle the check, or `undefined` to let Jev decide. */
export type Prefilter = (policy: Policy, surface: Surface, state: unknown) => Verdict | undefined;

export interface Pattern {
  readonly name: string;
  readonly regex: RegExp;
  readonly category: string;
  readonly action?: Action;
  readonly surfaces?: readonly Surface[];
}

/**
 * Matches patterns against the text in a state and settles the check on the first hit.
 *
 * Order matters only in that the first match wins, so put the most specific patterns first.
 */
export function patternPrefilter(patterns: readonly Pattern[]): Prefilter {
  return (policy, surface, state) => {
    const text = textOf(state);
    if (!text) return undefined;
    for (const pattern of patterns) {
      const surfaces = pattern.surfaces ?? (["input", "output", "conversation"] as const);
      if (!surfaces.includes(surface)) continue;
      // A global or sticky regex carries lastIndex between calls; reset so reuse is safe.
      pattern.regex.lastIndex = 0;
      if (!pattern.regex.test(text)) continue;
      const category = policy.categories.get(pattern.category);
      if (!category || !(category.enabled ?? true)) continue;
      if (!(category.surfaces ?? []).includes(surface)) continue;
      return prefilterVerdict(policy, surface, category.id, pattern.action ?? "block", pattern.name);
    }
    return undefined;
  };
}

/** Build a verdict that looks like any other, so callers need no special case. */
export function prefilterVerdict(
  policy: Policy,
  surface: Surface,
  categoryId: string,
  action: Action,
  rule: string,
): Verdict {
  const category = policy.categories.get(categoryId);
  if (!category) throw new Error(`prefilter refers to unknown category "${categoryId}"`);

  let route: Route =
    action === "block" ? "safe_response" : action === "review" ? "human_review" : "deliver";
  if (category.route === "crisis_support" && action !== "allow") route = "crisis_support";
  else if ((category.route === "redact" || category.route === "guide") && action !== "block") {
    route = category.route;
  }

  return {
    action,
    allowed: rank(action) <= rank("flag"),
    deliverable: !WITHHOLDING.has(route),
    surface,
    route,
    findings: [
      {
        category: category.id,
        name: category.name,
        probability: 1,
        confidence: 1,
        action,
        severity: category.base_severity ?? 2,
        refs: category.refs ?? [],
        source: `prefilter:${rule}`,
        notes: [`matched by ${rule}, Jev was not called`],
      },
    ],
    signals: {},
    confidence: 1,
    severity: category.base_severity ?? 2,
    appliedRules: [],
    policyId: policy.policyId,
    model: "",
    usage: { inputTokens: 0, outputTokens: 0 },
    latencyMs: 0,
    degraded: false,
    cached: false,
    partial: false,
    prefilter: rule,
  };
}

/**
 * A starting set. Every deployment should replace these with its own.
 *
 * These are examples of the shape, not a recommended list: what counts as a banned term is a
 * policy question for the deployment, and a pattern that is wrong blocks real users silently.
 */
export const COMMON_PATTERNS: readonly Pattern[] = [
  { name: "openai-style-key", regex: /\bsk-[A-Za-z0-9]{20,}\b/, category: "sid", surfaces: ["output", "conversation"] },
  { name: "jev-api-key", regex: /\bapikey_[a-f0-9]{30,}\b/, category: "sid", surfaces: ["output", "conversation"] },
  {
    name: "private-key-block",
    regex: /-----BEGIN (RSA |EC |OPENSSH |PGP )?PRIVATE KEY-----/,
    category: "sid",
    surfaces: ["output", "conversation"],
  },
  { name: "vn-national-id", regex: /\b0\d{11}\b/, category: "prv", action: "review" },
];

/** Pull the checkable text out of any of the three state shapes. */
function textOf(state: unknown): string {
  if (typeof state === "string") return state;
  if (!state || typeof state !== "object") return "";
  const record = state as Record<string, unknown>;
  if (Array.isArray(record["turns"])) {
    return (record["turns"] as Array<Record<string, unknown>>)
      .map((turn) => String(turn?.["content"] ?? ""))
      .join("\n");
  }
  return [record["user_message"], record["assistant_reply"]]
    .filter((part): part is string => typeof part === "string" && part.length > 0)
    .join("\n");
}
