/** The public entry point: one `Guard`, three checks, and the plumbing around them. */

import { type VerdictCache, cacheKey } from "./cache.js";
import { FetchTransport, type Transport } from "./client.js";
import { decide, errorVerdict, withFloor } from "./decide.js";
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
import type { Surface, Turn, Verdict } from "./types.js";

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
}

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
   */
  checkOutput(reply: string, options: OutputOptions = {}): Promise<Verdict> {
    const state = outputState(reply, {
      ...(options.userMessage === undefined ? {} : { userMessage: options.userMessage }),
      ...(options.context ? { context: options.context } : {}),
      metadata: mergeMetadata(options.metadata, options.session) ?? undefined,
    });
    return this.run("output", state, options, {
      hasContext: Boolean(options.context),
      subset: options.quick ? "sentinels" : "full",
    });
  }

  /**
   * Check a whole conversation for patterns no single turn reveals.
   *
   * Multi-turn jailbreaks look harmless turn by turn: the escalation is the attack. This check
   * reads the transcript as one state, and belongs off the critical path.
   */
  checkConversation(turns: readonly Turn[], options: CheckOptions = {}): Promise<Verdict> {
    const state = conversationState(turns, mergeMetadata(options.metadata, options.session));
    return this.run("conversation", state, options);
  }

  /** Run every applicable check for one exchange, in parallel. */
  async checkTurn(
    userMessage: string,
    reply: string,
    options: OutputOptions & { history?: readonly Turn[] } = {},
  ): Promise<{ input: Verdict; output: Verdict; conversation?: Verdict }> {
    const history = options.history ?? options.session?.history ?? [];
    const [input, output, conversation] = await Promise.all([
      this.checkInput(userMessage, options),
      this.checkOutput(reply, { ...options, userMessage }),
      history.length > 0
        ? this.checkConversation(
            [
              ...history,
              { role: "user", content: userMessage },
              { role: "assistant", content: reply },
            ],
            options,
          )
        : Promise.resolve(undefined),
    ]);
    return conversation ? { input, output, conversation } : { input, output };
  }

  /** Guard a streamed reply, releasing text one chunk behind its check. */
  stream(source: AsyncIterable<string>, options: StreamOptions = {}): AsyncGenerator<StreamEvent> {
    return guardStream(this, source, options);
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
    const subset = build.subset ?? "full";

    if (this.prefilter) {
      const decided = this.prefilter(this.policy, surface, state);
      if (decided) return this.finish(decided, options.session);
    }

    let key: string | undefined;
    if (this.cache && this.cacheSurfaces.has(surface)) {
      key = cacheKey(this.policy.policyId, surface, state, subset);
      const hit = this.cache.get(key);
      if (hit) return this.finish(hit, options.session);
    }

    const questions = buildQuestions(this.policy, surface, build);
    const started = performance.now();
    try {
      const { answers, model, usage } = await this.transport.systemOne(state, questions, {
        ...(options.model ? { model: options.model } : {}),
        ...(this.timeout === undefined ? {} : { timeout: this.timeout }),
        ...(options.signal ? { signal: options.signal } : {}),
      });
      let verdict = decide(this.policy, surface, answers, {
        model,
        usage,
        latencyMs: performance.now() - started,
      });
      if (subset !== "full") verdict = { ...verdict, partial: true };
      if (key && this.cache) this.cache.put(key, verdict);
      return this.finish(verdict, options.session);
    } catch (error) {
      if (this.throwOnError) throw error;
      return this.finish(
        errorVerdict(this.policy, surface, error, performance.now() - started),
        options.session,
      );
    }
  }

  /** Apply the session floor, tell the session, and emit to the observer. */
  private finish(verdict: Verdict, session: Session | undefined): Verdict {
    let final = verdict;
    if (session) {
      const floor = session.floor;
      if (floor !== "allow") {
        final = withFloor(this.policy, final, floor, `session-floor:${session.id || "unnamed"}`);
      }
      session.observe(final);
    }
    this.observer?.(final);
    return final;
  }
}

/** Merge caller metadata over the session's, so an explicit value always wins. */
function mergeMetadata(
  metadata: Metadata | undefined,
  session: Session | undefined,
): Metadata | undefined {
  if (!session) return metadata;
  return { ...session.metadata(), ...(metadata ?? {}) };
}
