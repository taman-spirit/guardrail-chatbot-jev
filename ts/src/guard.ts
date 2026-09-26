/** The public entry point: one `Guard`, three checks, and the plumbing around them. */

import { type VerdictCache, cacheKey } from "./cache.js";
import { FetchTransport, type Transport } from "./client.js";
import { decide, errorVerdict, withFloor } from "./decide.js";
import {
  CONTEXT_COMPLETES,
  CONTEXT_DISENGAGES,
  type ContextCheck,
  type ContextResult,
  DEFAULT_ATTRIBUTION,
  DEFAULT_WATCH_RISK,
  type MultiturnMode,
  WITHHELD_PLACEHOLDER,
  attribute,
  contextQuestions,
  outputInContextState,
} from "./multiturn.js";
import { Policy, type PolicyPack } from "./policy.js";
import type { Prefilter } from "./prefilter.js";
import {
  buildQuestions,
  conversationState,
  inputState,
  type Metadata,
  outputState,
} from "./questions.js";
import type { Session } from "./session.js";
import { type StreamEvent, type StreamOptions, guardStream } from "./streaming.js";
import { type Answers, type Surface, type Turn, type Verdict, WITHHOLDING, rank } from "./types.js";

/** Conversation state changes every turn, so caching it buys nothing and only costs memory. */
export const DEFAULT_CACHE_SURFACES: readonly Surface[] = ["input", "output"];

export interface GuardConfig {
  /** A pack name, a path to a pack file, a pack object, or a `Policy`. Defaults to `standard-v1`. */
  readonly policy?: string | PolicyPack | Policy;
  /** How to reach Jev. Defaults to the official SDK. */
  readonly transport?: Transport;
  /** Optional verdict cache. Identical content skips the round trip. */
  readonly cache?: VerdictCache;
  readonly cacheSurfaces?: readonly Surface[];
  /**
   * Optional deterministic check run before Jev; a verdict from it settles the check without a
   * network call.
   */
  readonly prefilter?: Prefilter;
  /**
   * Called with every verdict, including cached and degraded ones. This is the metrics hook;
   * keep it fast and non-throwing.
   */
  readonly observer?: (verdict: Verdict) => void;
  /** Throw instead of returning a degraded verdict when Jev is unreachable. */
  readonly throwOnError?: boolean;
  /** Per-call timeout in milliseconds. */
  readonly timeout?: number;
  /** How earlier turns may affect a later verdict; see {@link MultiturnMode}. Defaults to "attribute". */
  readonly multiturn?: MultiturnMode;
  /** Tunes the in-context output check that the "attribute" mode uses. */
  readonly contextCheck?: ContextCheck;
  /** What a review verdict does to the content; see {@link ReviewHandling}. Defaults to "hold". */
  readonly reviewHandling?: ReviewHandling;
}

/**
 * What a review verdict does to the content.
 *
 * - `"hold"` (the default) withholds content at review until a person has looked at it.
 * - `"audit"` is for realtime chat, where nobody can look before the reply is due: review delivers
 *   the content and queues it for a person afterwards. Only block stops content, and crisis
 *   support still replaces it. A degraded verdict is not affected: when Jev could not be reached
 *   nothing was checked, so a fail-closed surface still holds.
 */
export type ReviewHandling = "hold" | "audit";

export interface CheckOptions {
  readonly metadata?: Metadata;
  readonly model?: string;
  readonly session?: Session;
  readonly signal?: AbortSignal;
}

export interface OutputOptions extends CheckOptions {
  readonly userMessage?: string;
  readonly context?: string | readonly string[];
  /**
   * Ask only the sentinel questions. What mid-stream checks use on incomplete text; a complete
   * reply should get the full set.
   */
  readonly quick?: boolean;
  /** The conversation before this exchange. Defaults to the session's. */
  readonly history?: readonly Turn[];
}

/**
 * Checks chatbot content against a policy pack using Jev.
 *
 * @example
 * ```ts
 * const guard = new Guard({ cache: new LRUCache(), observer: (v) => metrics.emit(v) });
 * const verdict = await guard.checkInput(userMessage);
 * if (!verdict.allowed) return safeResponse(verdict);
 * ```
 */
export class Guard {
  readonly policy: Policy;
  readonly cache: VerdictCache | undefined;
  readonly cacheSurfaces: ReadonlySet<Surface>;
  readonly prefilter: Prefilter | undefined;
  readonly observer: ((verdict: Verdict) => void) | undefined;
  private transportImpl: Transport | undefined;
  readonly multiturn: MultiturnMode;
  readonly contextCheck: ContextCheck;
  readonly reviewHandling: ReviewHandling;
  private readonly throwOnError: boolean;
  private readonly timeout: number | undefined;

  constructor(config: GuardConfig = {}) {
    this.policy = Policy.load(config.policy);
    this.transportImpl = config.transport;
    this.cache = config.cache;
    this.cacheSurfaces = new Set(config.cacheSurfaces ?? DEFAULT_CACHE_SURFACES);
    this.prefilter = config.prefilter;
    this.observer = config.observer;
    this.throwOnError = config.throwOnError ?? false;
    this.timeout = config.timeout;
    this.multiturn = config.multiturn ?? "attribute";
    this.contextCheck = config.contextCheck ?? {};
    this.reviewHandling = config.reviewHandling ?? "hold";
  }

  /** Built on first use, so constructing a `Guard` never needs an API key. */
  get transport(): Transport {
    this.transportImpl ??= new FetchTransport();
    return this.transportImpl;
  }

  /** Check a user message before it reaches the model. */
  checkInput(content: string, options: CheckOptions = {}): Promise<Verdict> {
    const state = inputState(content, mergeMetadata(options.metadata, options.session));
    return this.run("input", state, options);
  }

  /**
   * Check an assistant reply before it reaches the user.
   *
   * Pass `context` (the retrieved passages the reply was meant to be based on) to enable the
   * groundedness signal, which catches claims the context does not support.
   *
   * In a session with recent risk, a complete reply is also read against the earlier turns, and
   * only a reply that itself completes an earlier harmful request is held for it; see
   * {@link MultiturnMode}.
   */
  async checkOutput(reply: string, options: OutputOptions = {}): Promise<Verdict> {
    const metadata = mergeMetadata(options.metadata, options.session);
    const state = outputState(reply, {
      ...(options.userMessage === undefined ? {} : { userMessage: options.userMessage }),
      ...(options.context ? { context: options.context } : {}),
      metadata: metadata ?? undefined,
    });
    const build = {
      hasContext: Boolean(options.context),
      subset: options.quick ? ("sentinels" as const) : ("full" as const),
    };

    const earlier =
      options.history && options.history.length > 0 ? options.history : (options.session?.history ?? []);
    if (options.quick || !this.contextCheckApplies(options.session, earlier)) {
      return this.run("output", state, options, build);
    }

    // Both requests go out together, so the in-context read adds no latency, and the standalone
    // check never sees the history: its answer stays uncontaminated by what came before.
    const inContext = this.checkInContext(reply, options.userMessage, earlier, metadata, options);
    let verdict: Verdict;
    try {
      verdict = await this.evaluate("output", state, options, build);
    } catch (error) {
      await inContext;
      throw error;
    }
    const result = await inContext;
    return this.finish(
      attribute(this.policy, verdict, result, this.attributionThreshold()),
      options.session,
    );
  }

  /**
   * Check a whole conversation for patterns no single turn reveals.
   *
   * Multi-turn jailbreaks look harmless turn by turn: the escalation is the attack. This check
   * reads the transcript as one state, and belongs off the critical path.
   *
   * A transcript with nothing in it but withheld turns is not sent: there is no content to read,
   * and Jev, asked to judge only omissions, answers from what they might have been.
   */
  async checkConversation(turns: readonly Turn[], options: CheckOptions = {}): Promise<Verdict> {
    if (turns.every((turn) => turn.content === WITHHELD_PLACEHOLDER)) {
      return this.finish(
        {
          action: "allow",
          allowed: true,
          deliverable: true,
          surface: "conversation",
          route: "deliver",
          findings: [],
          signals: {},
          confidence: 1,
          severity: 0,
          appliedRules: ["nothing-to-read"],
          policyId: this.policy.policyId,
          model: "",
          usage: { inputTokens: 0, outputTokens: 0 },
          latencyMs: 0,
          degraded: false,
          cached: false,
          partial: false,
        },
        options.session,
      );
    }
    const state = conversationState(turns, mergeMetadata(options.metadata, options.session));
    return this.run("conversation", state, options);
  }

  /**
   * Run every applicable check for one exchange, in order: input, output, then the conversation
   * when there is history to read.
   *
   * In order, not in parallel, because each check tells the session before the next one reads it:
   * a withheld input is what puts the reply under the in-context watch.
   */
  async checkTurn(
    userMessage: string,
    reply: string,
    options: OutputOptions & { history?: readonly Turn[] } = {},
  ): Promise<{ input: Verdict; output: Verdict; conversation?: Verdict }> {
    const { history: given, quick: _quick, ...rest } = options;
    const history = given && given.length > 0 ? given : (options.session?.history ?? []);
    const input = await this.checkInput(userMessage, rest);
    const output = await this.checkOutput(reply, { ...rest, userMessage });
    if (history.length === 0) return { input, output };
    const conversation = await this.checkConversation(
      [...history, { role: "user", content: userMessage }, { role: "assistant", content: reply }],
      rest,
    );
    return { input, output, conversation };
  }

  /** Guard a streamed reply, releasing text one chunk behind its check. */
  stream(source: AsyncIterable<string>, options: StreamOptions = {}): AsyncGenerator<StreamEvent> {
    return guardStream(this, source, options);
  }

  /**
   * Whether a streamed reply is held whole for the final check.
   *
   * Mid-stream checks read each chunk on its own. In a watched session a part that is harmful only
   * in the light of earlier turns would get past them, so the reply is held whole for the final
   * check, which reads it in context.
   */
  holdsWholeReply(session: Session | undefined): boolean {
    if (this.multiturn !== "attribute" || this.contextCheck.never || !session) return false;
    return Boolean(this.contextCheck.always) || session.watching(this.watchRisk());
  }

  /** The exact request body that would be sent, without sending it. */
  preview(
    surface: Surface,
    state: unknown,
    options: { hasContext?: boolean; subset?: "full" | "sentinels" } = {},
  ): Record<string, unknown> {
    return { state, questions: buildQuestions(this.policy, surface, options) };
  }

  private async run(
    surface: Surface,
    state: unknown,
    options: CheckOptions,
    build: { hasContext?: boolean; subset?: "full" | "sentinels" } = {},
  ): Promise<Verdict> {
    return this.finish(await this.evaluate(surface, state, options, build), options.session);
  }

  /** Reach a verdict for one state without touching the session or the observer. */
  private async evaluate(
    surface: Surface,
    state: unknown,
    options: CheckOptions,
    build: { hasContext?: boolean; subset?: "full" | "sentinels" } = {},
  ): Promise<Verdict> {
    const subset = build.subset ?? "full";

    if (this.prefilter) {
      const decided = this.prefilter(this.policy, surface, state);
      if (decided) return decided;
    }

    let key: string | undefined;
    if (this.cache && this.cacheSurfaces.has(surface)) {
      key = cacheKey(this.policy.policyId, surface, state, subset);
      const hit = this.cache.get(key);
      if (hit) return hit;
    }

    const questions = buildQuestions(this.policy, surface, build);
    const started = performance.now();
    try {
      const { answers, model, usage } = await this.call(state, questions, options);
      let verdict = decide(this.policy, surface, answers, {
        model,
        usage,
        latencyMs: performance.now() - started,
      });
      if (subset !== "full") verdict = { ...verdict, partial: true };
      if (key && this.cache) this.cache.put(key, verdict);
      return verdict;
    } catch (error) {
      if (this.throwOnError) throw error;
      return errorVerdict(this.policy, surface, error, performance.now() - started);
    }
  }

  private call(
    state: unknown,
    questions: Record<string, unknown>,
    options: CheckOptions,
  ): Promise<{ answers: Answers; model: string; usage: { inputTokens: number; outputTokens: number } }> {
    return this.transport.systemOne(state, questions, {
      ...(options.model ? { model: options.model } : {}),
      ...(this.timeout === undefined ? {} : { timeout: this.timeout }),
      ...(options.signal ? { signal: options.signal } : {}),
    });
  }

  private watchRisk(): number {
    const threshold = this.contextCheck.watchRisk ?? 0;
    return threshold > 0 ? threshold : DEFAULT_WATCH_RISK;
  }

  private attributionThreshold(): number {
    const threshold = this.contextCheck.attribution ?? 0;
    return threshold > 0 ? threshold : DEFAULT_ATTRIBUTION;
  }

  private contextCheckApplies(session: Session | undefined, earlier: readonly Turn[]): boolean {
    if (this.contextCheck.never || this.multiturn === "floor" || earlier.length === 0) return false;
    if (this.contextCheck.always) return true;
    return session !== undefined && session.watching(this.watchRisk());
  }

  /** Read the reply against the earlier turns. Never rejects: a failure is carried in the result. */
  private async checkInContext(
    reply: string,
    userMessage: string | undefined,
    earlier: readonly Turn[],
    metadata: Metadata | undefined,
    options: CheckOptions,
  ): Promise<ContextResult> {
    try {
      const questions = contextQuestions(this.policy);
      const state = outputInContextState(reply, userMessage, earlier, metadata);
      const { answers, model, usage } = await this.call(state, questions, options);
      return {
        verdict: decide(this.policy, "output", answers, { model, usage }),
        completes: noulOf(answers, CONTEXT_COMPLETES),
        disengages: noulOf(answers, CONTEXT_DISENGAGES),
      };
    } catch (error) {
      return { completes: 0, disengages: 0, error };
    }
  }

  /**
   * Mark the audit level, apply the session floor ("floor" mode only), tell the session, and emit
   * to the observer. Runs exactly once per check.
   */
  private finish(verdict: Verdict, session: Session | undefined): Verdict {
    let final = this.audit(verdict);
    if (session) {
      const floor = session.floor;
      if (floor !== "allow" && this.multiturn === "floor") {
        final = withFloor(this.policy, final, floor, `session-floor:${session.id || "unnamed"}`);
      }
      session.observe(final);
    }
    this.observer?.(final);
    return final;
  }

  /** Mark what a person should look at later, and in "audit" mode let a review through. */
  private audit(verdict: Verdict): Verdict {
    const audit =
      rank(verdict.action) >= rank("review") ? "priority" : verdict.action === "flag" ? "sample" : null;
    if (this.reviewHandling === "audit" && verdict.route === "human_review" && !verdict.degraded) {
      return {
        ...verdict,
        audit,
        route: "deliver_and_audit",
        deliverable: !WITHHOLDING.has("deliver_and_audit"),
      };
    }
    return { ...verdict, audit };
  }
}

function noulOf(answers: Answers, name: string): number {
  const answer = answers[name];
  return answer && answer.type === "noul" && typeof answer.noul === "number" ? answer.noul : 0;
}

/** Merge caller metadata over the session's, so an explicit value always wins. */
function mergeMetadata(
  metadata: Metadata | undefined,
  session: Session | undefined,
): Metadata | undefined {
  if (!session) return metadata;
  return { ...session.metadata(), ...(metadata ?? {}) };
}
