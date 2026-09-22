/**
 * Guarding a streamed reply.
 *
 * A streamed reply cannot be checked before its first token, and a check that waits for the last
 * one gives up streaming altogether. The middle path here is to release the text one chunk
 * behind: chunk *k* is held until its check returns, while the model is already producing chunk
 * *k+1*. Only the first chunk pays the full latency.
 *
 * Mid-stream checks ask the sentinel questions only: the categories where a miss is unacceptable,
 * which are also the ones judgeable from partial text. Skipping the 18-label choice is what keeps
 * a per-chunk check cheap. The complete reply gets the full question set at the end.
 */

import type { Session } from "./session.js";
import type { Verdict } from "./types.js";

/** Prefer to cut after sentence-ending punctuation, including the Vietnamese and CJK forms. */
const BOUNDARY = /(?<=[.!?;:\n。！？])\s/;

export type StreamEventType = "delta" | "blocked" | "done";

/**
 * One thing that happened while the reply was being guarded.
 *
 * `delta` carries text cleared for delivery. `blocked` means the stream stopped and nothing
 * further should be sent. `done` carries the verdict on the complete reply.
 */
export interface StreamEvent {
  readonly type: StreamEventType;
  readonly text: string;
  readonly verdict?: Verdict;
}

export interface StreamOptions {
  readonly userMessage?: string;
  readonly context?: string | readonly string[];
  readonly session?: Session;
  readonly metadata?: Record<string, unknown>;
  /**
   * Minimum characters before looking for a sentence boundary to cut at. Smaller means more
   * round trips and a tighter hold; larger means fewer, coarser checks.
   */
  readonly chunkChars?: number;
  readonly signal?: AbortSignal;
}

interface OutputChecker {
  checkOutput(reply: string, options: Record<string, unknown>): Promise<Verdict>;
}

/**
 * Wrap a token stream, releasing text one chunk behind its safety check.
 *
 * Stop consuming on `blocked`; `done` is always last unless blocked.
 */
export async function* guardStream(
  guard: OutputChecker,
  source: AsyncIterable<string>,
  options: StreamOptions = {},
): AsyncGenerator<StreamEvent> {
  const chunkChars = options.chunkChars ?? 280;
  const base = {
    ...(options.userMessage === undefined ? {} : { userMessage: options.userMessage }),
    ...(options.context ? { context: options.context } : {}),
    ...(options.session ? { session: options.session } : {}),
    ...(options.metadata ? { metadata: options.metadata } : {}),
    ...(options.signal ? { signal: options.signal } : {}),
  };

  const pump = new Pump(source);
  let delivered = "";
  let pending = "";

  try {
    for (;;) {
      const next = await pump.next();
      if (next === undefined) break;
      pending += next;

      const chunk = takeChunk(pending, chunkChars);
      if (chunk === undefined) continue;
      pending = pending.slice(chunk.length);

      const verdict = await guard.checkOutput(delivered + chunk, { ...base, quick: true });
      if (!verdict.deliverable) {
        yield { type: "blocked", text: "", verdict };
        return;
      }
      delivered += chunk;
      yield { type: "delta", text: chunk };
    }

    pump.throwIfFailed();

    const finalText = delivered + pending;
    const verdict = await guard.checkOutput(finalText, base);
    if (!verdict.deliverable) {
      yield { type: "blocked", text: "", verdict };
      return;
    }
    if (pending) yield { type: "delta", text: pending };
    yield { type: "done", text: finalText, verdict };
  } finally {
    await pump.close();
  }
}

/** The next releasable chunk, cut at a sentence boundary once past `chunkChars`. */
function takeChunk(pending: string, chunkChars: number): string | undefined {
  if (pending.length < chunkChars) return undefined;
  const tail = pending.slice(chunkChars);
  const match = BOUNDARY.exec(tail);
  if (match) return pending.slice(0, chunkChars + match.index + 1);
  // No boundary in sight and the buffer is getting long: cut anyway rather than hold the stream
  // hostage to a model that is writing one very long sentence.
  if (pending.length >= chunkChars * 3) return pending.slice(0, chunkChars);
  return undefined;
}

/**
 * Drains the source into a buffer in the background.
 *
 * Without this the generator would only pull from the model between checks, so generation and
 * checking would take turns instead of overlapping, which is the whole point.
 */
class Pump {
  private readonly buffer: string[] = [];
  private waiting: (() => void) | undefined;
  private finished = false;
  private failure: unknown;
  private readonly task: Promise<void>;

  constructor(source: AsyncIterable<string>) {
    this.task = (async () => {
      try {
        for await (const delta of source) {
          this.buffer.push(delta);
          this.wake();
        }
      } catch (error) {
        this.failure = error;
      } finally {
        this.finished = true;
        this.wake();
      }
    })();
  }

  async next(): Promise<string | undefined> {
    for (;;) {
      const value = this.buffer.shift();
      if (value !== undefined) return value;
      if (this.finished) return undefined;
      await new Promise<void>((resolve) => {
        this.waiting = resolve;
      });
    }
  }

  throwIfFailed(): void {
    if (this.failure) throw this.failure;
  }

  async close(): Promise<void> {
    await this.task.catch(() => undefined);
  }

  private wake(): void {
    const waiting = this.waiting;
    this.waiting = undefined;
    waiting?.();
  }
}
