/**
 * History is for understanding the current turn, never for convicting it.
 *
 * A classifier that reads a violation in the history tends to hand the same label to whatever
 * comes next: an apology, a question about the law, a request for the weather. Two things keep that
 * out:
 *
 * - The input check reads only the current message, so the past can never block a question.
 * - A reply in a risky session is also read against the earlier turns, and it is held for them only
 *   when it itself completes an earlier harmful request: the next step, more detail, a translation
 *   or a fictional retelling of it. A reply that merely follows on from the earlier turns, or refers
 *   back to them to apologise, ask about the law or change the subject, is not.
 *
 * The judgment lands on what is about to be delivered, not on a guess about intent: if a follow-up
 * is harmless, so is the answer to it, and there is nothing to hold.
 */

import { resolveRoute, sortFindings } from "./decide.js";
import type { Policy } from "./policy.js";
import { HAZARD, type Metadata, NONE_DESCRIPTION, NONE_LABEL, type Question } from "./questions.js";
import {
  type Action,
  type ContextRead,
  type Finding,
  type Turn,
  type Verdict,
  WITHHOLDING,
  rank,
  stronger,
} from "./types.js";

/**
 * How earlier turns may affect a later verdict.
 *
 * - `"attribute"` (the default) holds a turn only for what that turn or its reply does. Earlier
 *   risk decides when a reply is also read in context, and never holds a clean turn by itself.
 * - `"floor"` is the earlier behaviour: after a conversation-level review or a block, every verdict
 *   for the next turns is raised to a floor, clean ones included.
 */
export type MultiturnMode = "attribute" | "floor";

/** Tunes the in-context output check. */
export interface ContextCheck {
  /** Read every reply in context whenever there is history, not only in risky sessions. */
  readonly always?: boolean;
  /** Turn the in-context check off. */
  readonly never?: boolean;
  /**
   * The session risk from which replies are also read in context; defaults to 0.2. Risk decays on
   * every check, not every turn, so on its own it covers little more than the next check; the
   * turn-counted watch is what carries it forward.
   */
  readonly watchRisk?: number;
  /**
   * How sure Jev must be that the reply completes an earlier harmful request before the in-context
   * findings count; defaults to 0.5.
   */
  readonly attribution?: number;
}

/**
 * Stands in the transcript for a message the guardrail withheld. The attempt stays visible to the
 * conversation check, so repeated tries still show as a pattern, but its text does not: neither
 * Jev nor the model reads a blocked request again.
 *
 * The wording is neutral on purpose. Measured against Jev, "[message withheld by the safety
 * check]" followed by a question about the weather still drew sentinel answers of 0.08 to 0.15,
 * enough to block on the lowest thresholds, because the words themselves suggest danger; "[earlier
 * message omitted]" drew 0.04 or less, about what no placeholder at all draws.
 */
export const WITHHELD_PLACEHOLDER = "[earlier message omitted]";

/** Names of the questions the in-context check adds. */
export const CONTEXT_COMPLETES = "ctx_completes";
export const CONTEXT_DISENGAGES = "ctx_disengages";
export const CONTEXT_EVALUATING = "assistant_reply_in_context";
export const DEFAULT_WATCH_RISK = 0.2;
export const DEFAULT_ATTRIBUTION = 0.5;

const CONTEXT_HAZARD_INSTRUCTIONS =
  "Read the assistant reply together with the earlier turns. Which hazard category best describes harmful content that the reply itself supplies, once its meaning is read in the light of the earlier turns? Judge only what the reply provides: harm that appears only in earlier turns or in the user's messages does not count. Choose 'none' if the reply supplies nothing harmful.";

const COMPLETES_INSTRUCTIONS =
  "Read with the earlier turns, the assistant reply supplies harmful content or completes a harmful request from an earlier turn: for example the next step of a procedure, more detail on it, or a rephrasing, translation, summary or fictional retelling of it.";

const DISENGAGES_INSTRUCTIONS =
  "The latest user message refers to earlier harmful content only to step away from it: to withdraw it or apologise, to ask about the law, consequences, prevention, safety or reporting, to ask why a request was refused, or to change the subject.";

/** The question set for reading a reply against the earlier turns. */
export function contextQuestions(policy: Policy): Record<string, Question> {
  const categories = policy.forSurface("output");
  if (categories.length === 0) {
    throw new Error(`policy "${policy.id}" has no categories for surface "output"`);
  }
  const note = policy.contentNote ? ` ${policy.contentNote}` : "";
  const criteria: Record<string, unknown> = { [NONE_LABEL]: NONE_DESCRIPTION };
  for (const category of categories) criteria[category.id] = category.description;
  return {
    [HAZARD]: { type: "choice", instructions: CONTEXT_HAZARD_INSTRUCTIONS + note, criteria },
    [CONTEXT_COMPLETES]: { type: "noul", instructions: COMPLETES_INSTRUCTIONS },
    [CONTEXT_DISENGAGES]: { type: "noul", instructions: DISENGAGES_INSTRUCTIONS },
  };
}

/** State for reading a reply against the earlier turns. */
export function outputInContextState(
  reply: string,
  userMessage: string | undefined,
  earlier: readonly Turn[],
  metadata?: Metadata,
): Record<string, unknown> {
  return {
    evaluating: CONTEXT_EVALUATING,
    earlier_turns: earlier.map((turn) => ({ role: turn.role, content: turn.content })),
    assistant_reply: reply,
    ...(userMessage ? { user_message: userMessage } : {}),
    ...(metadata && Object.keys(metadata).length > 0 ? { deployment_context: { ...metadata } } : {}),
  };
}

/** What the in-context read came back with, before it is merged. */
export interface ContextResult {
  readonly verdict?: Verdict;
  readonly completes: number;
  readonly disengages: number;
  readonly error?: unknown;
}

/**
 * Merge the in-context findings into the standalone verdict, but only when the reply itself
 * completes an earlier harmful request. Otherwise they are recorded and change nothing.
 */
export function attribute(
  policy: Policy,
  verdict: Verdict,
  result: ContextResult,
  threshold: number = DEFAULT_ATTRIBUTION,
): Verdict {
  if (result.error !== undefined || !result.verdict) {
    // The standalone check still stands; an outage of the second read is not evidence.
    const error = result.error;
    const read: ContextRead = {
      ran: true,
      attributed: false,
      completes: result.completes,
      disengages: result.disengages,
      categories: [],
      error: error instanceof Error ? error.message : String(error),
    };
    return { ...verdict, context: read, appliedRules: [...verdict.appliedRules, "context-unavailable"] };
  }

  const fired = result.verdict.findings.filter((f) => rank(f.action) >= rank("flag"));
  const attributed =
    fired.length > 0 && result.completes >= threshold && result.completes >= result.disengages;
  const read: ContextRead = {
    ran: true,
    attributed,
    completes: result.completes,
    disengages: result.disengages,
    categories: fired.map((f) => f.category),
  };
  if (fired.length === 0) return { ...verdict, context: read };
  if (!attributed) {
    return { ...verdict, context: read, appliedRules: [...verdict.appliedRules, "context-not-attributed"] };
  }

  const findings: Finding[] = [...verdict.findings];
  for (const original of fired) {
    const f: Finding = {
      ...original,
      source: `context:${original.source}`,
      notes: [...original.notes, "completes an earlier request"],
    };
    const i = findings.findIndex((x) => x.category === f.category);
    if (i >= 0) {
      if (rank(f.action) > rank(findings[i]!.action)) findings[i] = f;
      continue;
    }
    findings.push(f);
  }
  sortFindings(findings);
  let action: Action = verdict.action;
  for (const f of findings) action = stronger(action, f.action);
  const route = resolveRoute(policy, findings, action);
  return {
    ...verdict,
    findings,
    action,
    route,
    allowed: rank(action) <= rank("flag"),
    deliverable: !WITHHOLDING.has(route),
    severity: Math.max(verdict.severity, result.verdict.severity),
    context: read,
    appliedRules: [...verdict.appliedRules, "context-attributed"],
  };
}
