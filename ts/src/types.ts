/** Typed results returned by the guardrail. */

/**
 * The enforcement decision, ordered from least to most restrictive.
 *
 * Only four steps, and every one of them is a different answer to "does this content go out?".
 * How to handle it once decided, mask the personal data, steer the reply, hand it to a human, is a
 * separate axis carried by {@link Verdict.route}. Keeping them apart is what lets a rule move a
 * verdict one step without landing on a handling mode that makes no sense for the hazard.
 */
export const LADDER = ["allow", "flag", "review", "block"] as const;
export type Action = (typeof LADDER)[number];

export type Surface = "input" | "output" | "conversation";

export type Route =
  | "deliver"
  | "redact"
  | "guide"
  | "crisis_support"
  | "human_review"
  | "safe_response";

/** Routes that withhold the content and put something else in its place. */
export const WITHHOLDING: ReadonlySet<Route> = new Set<Route>([
  "safe_response",
  "crisis_support",
  "human_review",
]);

export function rank(action: string): number {
  const index = (LADDER as readonly string[]).indexOf(action);
  return index < 0 ? 0 : index;
}

export function stronger(a: string, b: string): Action {
  return (rank(a) >= rank(b) ? a : b) as Action;
}

export function weaker(a: string, b: string): Action {
  return (rank(a) <= rank(b) ? a : b) as Action;
}

export function shift(action: string, steps: number): Action {
  const index = Math.max(0, Math.min(LADDER.length - 1, rank(action) + steps));
  return LADDER[index]!;
}

/** One hazard category that fired, with the evidence behind it. */
export interface Finding {
  readonly category: string;
  readonly name: string;
  readonly probability: number;
  readonly confidence: number;
  readonly action: Action;
  readonly severity: number;
  readonly refs: readonly string[];
  readonly source: string;
  readonly notes: readonly string[];
}

export interface Usage {
  readonly inputTokens: number;
  readonly outputTokens: number;
}

/** The decision for one piece of content. */
export interface Verdict {
  readonly action: Action;
  /** True when the content may be delivered as-is or with a flag only. */
  readonly allowed: boolean;
  /** True when the content still reaches the user, possibly redacted or steered first. */
  readonly deliverable: boolean;
  readonly surface: Surface;
  readonly route: Route;
  readonly findings: readonly Finding[];
  readonly signals: Readonly<Record<string, number | string>>;
  readonly confidence: number;
  readonly severity: number;
  readonly appliedRules: readonly string[];
  readonly policyId: string;
  readonly model: string;
  readonly usage: Usage;
  readonly latencyMs: number;
  /** True when Jev could not be reached and the fallback decided instead. */
  readonly degraded: boolean;
  /** True when this verdict came from the cache rather than a fresh call. */
  readonly cached: boolean;
  /** True when only the sentinel questions were asked, as mid-stream checks do. */
  readonly partial: boolean;
  /** Set when a prefilter decided without calling Jev. */
  readonly prefilter?: string;
  readonly error?: string;
}

export interface Turn {
  readonly role: "user" | "assistant" | "system" | "tool";
  readonly content: string;
}

export class GuardrailError extends Error {
  override name = "GuardrailError";
}

/** Jev's wire form for one answer. */
export type Answer =
  | { type: "noul"; noul: number }
  | { type: "choice"; choice: string; confidence: number; probabilities: Record<string, number> }
  | {
      type: "score";
      score: number;
      confidence: number;
      probabilities: Record<string, number>;
      legend?: Record<string, unknown>;
    };

export type Answers = Readonly<Record<string, Answer>>;
