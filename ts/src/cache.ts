/**
 * Caching verdicts, so identical content does not pay for a second round trip.
 *
 * Repeat messages are common in a deployed chatbot, and a verdict is a pure function of the
 * policy, the surface and the content. The key carries the policy id, so publishing a new pack
 * invalidates every entry without anyone having to remember to flush.
 */

import { createHash } from "node:crypto";
import type { Surface, Verdict } from "./types.js";

/**
 * Dropped from the cache key by default.
 *
 * A session puts the turn number and a running risk score in here, and both change on every turn,
 * so keying on them would mean the cache never hits for exactly the repeated messages it exists
 * to serve.
 */
export const VOLATILE_STATE_KEYS: ReadonlySet<string> = new Set(["deployment_context"]);

/**
 * A stable key for one check.
 *
 * Deployment context is dropped by default, because it is per-request context rather than the
 * content being judged. If a deployment's metadata genuinely changes what a verdict should be,
 * pass an empty `ignore` set and accept the lower hit rate.
 */
export function cacheKey(
  policyId: string,
  surface: Surface,
  state: unknown,
  subset = "full",
  ignore: ReadonlySet<string> = VOLATILE_STATE_KEYS,
): string {
  let subject = state;
  if (ignore.size > 0 && state && typeof state === "object" && !Array.isArray(state)) {
    subject = Object.fromEntries(
      Object.entries(state as Record<string, unknown>).filter(([k]) => !ignore.has(k)),
    );
  }
  const material = [policyId, surface, subset, canonical(subject)].join("\0");
  return createHash("sha256").update(material, "utf8").digest("hex");
}

/** Anything that can remember a verdict. Bring your own Redis by implementing this. */
export interface VerdictCache {
  get(key: string): Verdict | undefined;
  put(key: string, verdict: Verdict): void;
}

export interface LRUCacheConfig {
  /** Maximum entries before the least recently used one is dropped. */
  readonly capacity?: number;
  /** Entry lifetime in milliseconds; 0 disables expiry. */
  readonly ttlMs?: number;
}

/**
 * In-process LRU with a TTL.
 *
 * Degraded verdicts are never stored. A verdict produced because Jev was unreachable says nothing
 * about the content, and caching one would turn a brief outage into a lasting wrong answer for
 * that exact message.
 */
export class LRUCache implements VerdictCache {
  readonly capacity: number;
  readonly ttlMs: number;
  hits = 0;
  misses = 0;
  private readonly entries = new Map<string, { at: number; verdict: Verdict }>();

  constructor(config: LRUCacheConfig = {}) {
    this.capacity = config.capacity ?? 4096;
    this.ttlMs = config.ttlMs ?? 300_000;
    if (this.capacity < 1) throw new Error("capacity must be at least 1");
  }

  get(key: string): Verdict | undefined {
    const entry = this.entries.get(key);
    if (!entry) {
      this.misses += 1;
      return undefined;
    }
    if (this.ttlMs > 0 && Date.now() - entry.at > this.ttlMs) {
      this.entries.delete(key);
      this.misses += 1;
      return undefined;
    }
    // Map preserves insertion order, so re-inserting marks the entry most recently used.
    this.entries.delete(key);
    this.entries.set(key, entry);
    this.hits += 1;
    return { ...entry.verdict, cached: true, latencyMs: 0 };
  }

  put(key: string, verdict: Verdict): void {
    if (verdict.degraded) return;
    this.entries.delete(key);
    this.entries.set(key, { at: Date.now(), verdict });
    while (this.entries.size > this.capacity) {
      const oldest = this.entries.keys().next();
      if (oldest.done) break;
      this.entries.delete(oldest.value);
    }
  }

  clear(): void {
    this.entries.clear();
  }

  get size(): number {
    return this.entries.size;
  }

  get stats(): Record<string, number> {
    const total = this.hits + this.misses;
    return {
      size: this.entries.size,
      capacity: this.capacity,
      hits: this.hits,
      misses: this.misses,
      hitRate: total ? Number((this.hits / total).toFixed(4)) : 0,
    };
  }
}

/** JSON with object keys sorted, so two equal states produce one key. */
function canonical(value: unknown): string {
  if (value === null || typeof value !== "object") return JSON.stringify(value) ?? "null";
  if (Array.isArray(value)) return `[${value.map(canonical).join(",")}]`;
  const entries = Object.entries(value as Record<string, unknown>)
    .filter(([, v]) => v !== undefined)
    .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0));
  return `{${entries.map(([k, v]) => `${JSON.stringify(k)}:${canonical(v)}`).join(",")}}`;
}
