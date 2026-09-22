/**
 * Turning a policy pack into a single Jev request.
 *
 * Jev answers every question in a request in parallel and charges only for input tokens, so the
 * guardrail asks everything it could need in one round trip rather than chaining calls.
 */

import type { Policy } from "./policy.js";
import type { Surface, Turn } from "./types.js";

export const HAZARD = "hazard";
export const NONE_LABEL = "none";
export const SENTINEL_PREFIX = "s_";

export type Question =
  | { type: "noul"; instructions: unknown; criteria?: unknown }
  | { type: "choice"; instructions: unknown; criteria: Record<string, unknown> }
  | { type: "score"; instructions: unknown; criteria: unknown[] };

const HAZARD_INSTRUCTIONS: Record<Surface, string> = {
  input:
    "Which hazard category best describes the most serious safety problem in this user message? Choose 'none' if the message raises no safety problem.",
  output:
    "Which hazard category best describes the most serious safety problem in this assistant reply? Judge the reply itself, not the user message that prompted it. Choose 'none' if the reply raises no safety problem.",
  conversation:
    "Across this whole conversation, which hazard category best describes the most serious safety problem? Choose 'none' if the conversation raises no safety problem.",
};

const NONE_DESCRIPTION = "No safety problem: the content raises none of the listed hazards.";

/**
 * The question set for a surface, in Jev wire form.
 *
 * `subset: "sentinels"` asks only the yes/no questions for the categories where a miss is
 * unacceptable. Mid-stream checks use it: the text is incomplete, so the categories that need the
 * whole reply to judge would only produce noise, and skipping the 18-label choice is what keeps a
 * per-chunk check cheap.
 */
export function buildQuestions(
  policy: Policy,
  surface: Surface,
  options: { hasContext?: boolean; subset?: "full" | "sentinels" } = {},
): Record<string, Question> {
  if (options.subset === "sentinels") {
    const sentinels = policy.sentinels(surface);
    if (sentinels.length === 0) {
      throw new Error(`policy "${policy.id}" has no sentinels for surface "${surface}"`);
    }
    return Object.fromEntries(
      sentinels.map((c) => [
        SENTINEL_PREFIX + c.id,
        { type: "noul", instructions: c.sentinel_instructions } satisfies Question,
      ]),
    );
  }

  const categories = policy.forSurface(surface);
  if (categories.length === 0) {
    throw new Error(`policy "${policy.id}" has no categories for surface "${surface}"`);
  }

  const note = policy.contentNote ? ` ${policy.contentNote}` : "";
  const criteria: Record<string, unknown> = { [NONE_LABEL]: NONE_DESCRIPTION };
  for (const category of categories) criteria[category.id] = category.description;

  const questions: Record<string, Question> = {
    [HAZARD]: { type: "choice", instructions: HAZARD_INSTRUCTIONS[surface] + note, criteria },
  };

  for (const category of policy.sentinels(surface)) {
    questions[SENTINEL_PREFIX + category.id] = {
      type: "noul",
      instructions: category.sentinel_instructions,
    };
  }

  for (const [name, spec] of Object.entries(policy.signalsFor(surface, options.hasContext))) {
    const resolved = policy.criteriaFor(spec);
    questions[name] = {
      type: spec.type,
      instructions: spec.instructions,
      ...(resolved === undefined ? {} : { criteria: resolved }),
    } as Question;
  }

  return questions;
}

export interface Metadata {
  readonly [key: string]: unknown;
}

/** State for a user message about to be sent to the model. */
export function inputState(content: string, metadata?: Metadata): Record<string, unknown> {
  return {
    evaluating: "user_message",
    user_message: content,
    ...(metadata ? { deployment_context: metadata } : {}),
  };
}

/** State for an assistant reply about to be delivered. */
export function outputState(
  reply: string,
  options: { userMessage?: string; context?: string | readonly string[]; metadata?: Metadata } = {},
): Record<string, unknown> {
  return {
    evaluating: "assistant_reply",
    assistant_reply: reply,
    ...(options.userMessage === undefined ? {} : { user_message: options.userMessage }),
    ...(options.context ? { reference_context: options.context } : {}),
    ...(options.metadata ? { deployment_context: options.metadata } : {}),
  };
}

/** State for a whole conversation, used to catch patterns no single turn shows. */
export function conversationState(
  turns: readonly Turn[],
  metadata?: Metadata,
): Record<string, unknown> {
  return {
    evaluating: "conversation",
    turns: turns.map((turn) => ({ role: turn.role, content: turn.content })),
    ...(metadata ? { deployment_context: metadata } : {}),
  };
}
