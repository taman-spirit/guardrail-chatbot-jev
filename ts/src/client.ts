/**
 * Transports that put a question set in front of Jev and bring answers back.
 *
 * The `fetch` transport is the default and needs no dependency. `SdkTransport` wraps a provider
 * SDK client you supply, and `RecordedTransport` replays fixed answers for offline work.
 */

import { type Answers, GuardrailError, type Usage } from "./types.js";

/**
 * Jev is served from one endpoint. This is a default, not a constraint: set JEV_BASE_URL or
 * pass baseURL to point at a gateway or a proxy in front of it.
 */
export const DEFAULT_BASE_URL = "https://api.typesafe.ai";
export const DEFAULT_MODEL = "jev-latest";
export const ENDPOINT = "/v1/systemone";

export interface TransportResult {
  readonly answers: Answers;
  readonly model: string;
  readonly usage: Usage;
}

export interface CallOptions {
  readonly model?: string;
  readonly timeout?: number;
  readonly signal?: AbortSignal;
}

/** Anything that can answer a question set. */
export interface Transport {
  systemOne(
    state: unknown,
    questions: Record<string, unknown>,
    options?: CallOptions,
  ): Promise<TransportResult>;
}

/**
 * The shape `SdkTransport` needs from a provider SDK client. Structural, not a named dependency:
 * anything with this method fits, which is what keeps this package free of one.
 */
export interface JevSdkClient {
  systemOne(
    request: { state: unknown; questions: Record<string, unknown>; model?: string },
    options?: { timeout?: number; signal?: AbortSignal },
  ): Promise<{
    answers: unknown;
    model: string;
    usage?: { input_tokens?: number; output_tokens?: number };
  }>;
}

/** Transport backed by a provider SDK client you supply. */
export class SdkTransport implements Transport {
  readonly client: JevSdkClient;

  constructor(client: JevSdkClient) {
    if (!client || typeof client.systemOne !== "function") {
      throw new GuardrailError(
        "SdkTransport needs a client with a systemOne method. Use FetchTransport for the dependency-free path.",
      );
    }
    this.client = client;
  }

  async systemOne(
    state: unknown,
    questions: Record<string, unknown>,
    options: CallOptions = {},
  ): Promise<TransportResult> {
    try {
      const response = await this.client.systemOne(
        {
          state,
          questions,
          ...(options.model ? { model: options.model } : {}),
        },
        {
          ...(options.timeout === undefined ? {} : { timeout: options.timeout }),
          ...(options.signal ? { signal: options.signal } : {}),
        },
      );
      return {
        answers: response.answers as unknown as Answers,
        model: response.model,
        usage: {
          inputTokens: response.usage?.input_tokens ?? 0,
          outputTokens: response.usage?.output_tokens ?? 0,
        },
      };
    } catch (error) {
      throw new GuardrailError(`Jev SDK call failed: ${(error as Error).message}`, {
        cause: error,
      });
    }
  }
}

export interface FetchTransportConfig {
  readonly apiKey?: string;
  readonly baseURL?: string;
  readonly model?: string;
  readonly timeout?: number;
  readonly maxRetries?: number;
  readonly fetch?: typeof globalThis.fetch;
}

const RETRY_STATUSES = new Set([408, 429, 500, 502, 503, 504, 529]);

/** Dependency-free transport over `POST /v1/systemone`, for runtimes without the SDK. */
export class FetchTransport implements Transport {
  private readonly apiKey: string;
  private readonly baseURL: string;
  private readonly model: string;
  private readonly timeout: number;
  private readonly maxRetries: number;
  private readonly fetchImpl: typeof globalThis.fetch;

  constructor(config: FetchTransportConfig = {}) {
    const key = config.apiKey ?? process.env["JEV_API_KEY"] ?? "";
    if (!key.trim()) {
      throw new GuardrailError(
        "No Jev API key. Set JEV_API_KEY, pass apiKey, or use a RecordedTransport for offline work.",
      );
    }
    this.apiKey = key.trim();
    this.baseURL = (config.baseURL ?? process.env["JEV_BASE_URL"] ?? DEFAULT_BASE_URL)
      .trim()
      .replace(/\/+$/, "");
    this.model = config.model ?? process.env["JEV_MODEL"] ?? DEFAULT_MODEL;
    this.timeout = config.timeout ?? 10_000;
    this.maxRetries = config.maxRetries ?? 2;
    this.fetchImpl = config.fetch ?? globalThis.fetch;
  }

  async systemOne(
    state: unknown,
    questions: Record<string, unknown>,
    options: CallOptions = {},
  ): Promise<TransportResult> {
    const body = JSON.stringify({ model: options.model ?? this.model, state, questions });
    let delay = 500;
    let last: unknown;

    for (let attempt = 0; attempt <= this.maxRetries; attempt += 1) {
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), options.timeout ?? this.timeout);
      options.signal?.addEventListener("abort", () => controller.abort(), { once: true });
      try {
        const response = await this.fetchImpl(this.baseURL + ENDPOINT, {
          method: "POST",
          headers: {
            authorization: `Bearer ${this.apiKey}`,
            "content-type": "application/json",
            "user-agent": "guardrail-chatbot-jev/1.0 (+node)",
          },
          body,
          signal: controller.signal,
        });
        if (!response.ok) {
          const detail = (await response.text()).slice(0, 400);
          last = new GuardrailError(`Jev API returned ${response.status}: ${detail}`);
          if (!RETRY_STATUSES.has(response.status) || attempt === this.maxRetries) throw last;
        } else {
          const payload = (await response.json()) as {
            answers?: Answers;
            model?: string;
            usage?: { input_tokens?: number; output_tokens?: number };
          };
          return {
            answers: payload.answers ?? {},
            model: payload.model ?? "",
            usage: {
              inputTokens: payload.usage?.input_tokens ?? 0,
              outputTokens: payload.usage?.output_tokens ?? 0,
            },
          };
        }
      } catch (error) {
        if (error instanceof GuardrailError && attempt === this.maxRetries) throw error;
        last = error instanceof GuardrailError
          ? error
          : new GuardrailError(`Jev API unreachable: ${(error as Error).message}`, { cause: error });
        if (attempt === this.maxRetries) throw last;
      } finally {
        clearTimeout(timer);
      }
      await sleep(delay * (1 - Math.random() * 0.25));
      delay = Math.min(delay * 2, 5_000);
    }
    throw last ?? new GuardrailError("Jev API call failed");
  }
}

/** Replays answers recorded earlier. For tests, offline work and policy tuning. */
export class RecordedTransport implements Transport {
  readonly calls: Array<{ state: unknown; questions: Record<string, unknown> }> = [];

  constructor(
    private readonly answers: Answers,
    private readonly model = "recorded",
  ) {}

  async systemOne(
    state: unknown,
    questions: Record<string, unknown>,
  ): Promise<TransportResult> {
    this.calls.push({ state, questions });
    return { answers: this.answers, model: this.model, usage: { inputTokens: 0, outputTokens: 0 } };
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
