/**
 * Content guardrails for AI chatbots, decided by the Jev decision model.
 *
 * ```ts
 * import { Guard } from "guardrail-chatbot-jev";
 *
 * const guard = new Guard();
 * const verdict = await guard.checkInput(userMessage);
 * if (!verdict.allowed) return safeResponse(verdict);
 * ```
 */

export { cacheKey, LRUCache, VOLATILE_STATE_KEYS } from "./cache.js";
export type { LRUCacheConfig, VerdictCache } from "./cache.js";
export { FetchTransport, RecordedTransport, SdkTransport } from "./client.js";
export type {
  CallOptions,
  FetchTransportConfig,
  JevSdkClient,
  Transport,
  TransportResult,
} from "./client.js";
export { decide, errorVerdict, withFloor } from "./decide.js";
export type { DecideOptions } from "./decide.js";
export { DEFAULT_CACHE_SURFACES, Guard } from "./guard.js";
export type { CheckOptions, GuardConfig, OutputOptions } from "./guard.js";
export { Policy } from "./policy.js";
export type { Category, CategorySpec, PolicyPack, RuleSpec, SignalSpec, Thresholds } from "./policy.js";
export { COMMON_PATTERNS, patternPrefilter, prefilterVerdict } from "./prefilter.js";
export type { Pattern, Prefilter } from "./prefilter.js";
export {
  buildQuestions,
  conversationState,
  HAZARD,
  inputState,
  NONE_LABEL,
  outputState,
  SENTINEL_PREFIX,
} from "./questions.js";
export type { Metadata, Question } from "./questions.js";
export { ACTION_RISK, Session } from "./session.js";
export type { SessionConfig, SessionState } from "./session.js";
export { guardStream } from "./streaming.js";
export type { StreamEvent, StreamEventType, StreamOptions } from "./streaming.js";
export { GuardrailError, LADDER, rank, shift, stronger, weaker, WITHHOLDING } from "./types.js";
export type { Action, Answer, Answers, Finding, Route, Surface, Turn, Usage, Verdict } from "./types.js";
