/** Loading and querying a policy pack. */

import { readFileSync } from "node:fs";
import type { Surface } from "./types.js";

export interface Thresholds {
  readonly block: number;
  readonly review: number;
  readonly flag: number;
}

export interface CategorySpec {
  readonly name: string;
  readonly description: string;
  readonly refs?: readonly string[];
  readonly surfaces?: readonly Surface[];
  readonly weight?: number;
  readonly base_severity?: number;
  readonly sentinel?: boolean;
  readonly sentinel_instructions?: string;
  readonly thresholds: Readonly<Record<string, Thresholds>>;
  readonly route?: "redact" | "guide" | "crisis_support";
  readonly never_below?: string;
  readonly enabled?: boolean;
}

export interface SignalSpec {
  readonly type: "noul" | "choice" | "score";
  readonly surfaces: readonly Surface[];
  readonly instructions: string;
  readonly criteria?: unknown;
  readonly criteria_ref?: string;
  readonly requires_context?: boolean;
}

export interface RuleSpec {
  readonly id: string;
  readonly when: { readonly signal: string; readonly op: string; readonly value: unknown };
  readonly except_categories?: readonly string[];
  readonly then: {
    readonly upgrade?: number;
    readonly downgrade?: number;
    readonly cap_action?: string;
    readonly floor_action?: string;
    readonly add_finding?: string;
    readonly skip_confidence_gate?: boolean;
  };
  readonly why?: string;
}

export interface PolicyPack {
  readonly id: string;
  readonly version: string;
  readonly name?: string;
  readonly content_note?: string;
  readonly defaults?: Readonly<Record<string, unknown>>;
  readonly categories: Readonly<Record<string, CategorySpec>>;
  readonly signals: Readonly<Record<string, SignalSpec>>;
  readonly rules?: readonly RuleSpec[];
  readonly routes?: Readonly<Record<string, string>>;
  readonly [scale: string]: unknown;
}

/** How a pack asks for a sentinel to be backed by the hazard choice. */
export interface SentinelCorroboration {
  /** The hazard-choice probability below which a sentinel counts as uncorroborated. */
  readonly minChoice: number;
  /** The refusal score from which a reply counts as declining. */
  readonly refusal: number;
  /** The sentinel answer from which even a declining reply is not capped. */
  readonly refusalMaxSentinel: number;
  /** Categories a declining reply can still carry, so they are never capped. */
  readonly refusalExcept: ReadonlySet<string>;
  /**
   * Cap an uncorroborated sentinel below its block band at flag: it is recorded and delivered,
   * rather than held, unless it crosses the block band on its own.
   */
  readonly weakAtMostFlag: boolean;
  /**
   * Resolve an uncorroborated sentinel for a category handled by redaction to review, so it is
   * masked and delivered, unless it reaches this value. Zero is off.
   */
  readonly redactInsteadOfBlockBelow: number;
  /** Categories never weakened: an uncorroborated sentinel keeps never_below and is not capped. */
  readonly weakExcept: ReadonlySet<string>;
}

/** When a low-confidence answer escalates to review. */
export interface ConfidenceGateOptions {
  /** Count only findings and near misses the hazard choice backs. */
  readonly needsCorroboration: boolean;
  /**
   * Intent labels for which the gate does not escalate, as long as the answer's confidence is at
   * least `skipMinConfidence`: below that the intent is itself a guess.
   */
  readonly skipWhenIntent: ReadonlySet<string>;
  readonly skipMinConfidence: number;
}

export interface Category extends CategorySpec {
  readonly id: string;
}

const BUNDLED = "standard-v1";

/**
 * A parsed policy pack.
 *
 * The pack is the hazard taxonomy, the thresholds that turn a probability into an action, and
 * the rules that adjust it. Its descriptions are also the text sent to Jev as question criteria,
 * so editing the pack changes both what the model is asked and how the answer is judged.
 */
export class Policy {
  readonly id: string;
  readonly version: string;
  readonly name: string;
  readonly contentNote: string;
  readonly categories: ReadonlyMap<string, Category>;
  readonly signals: Readonly<Record<string, SignalSpec>>;
  readonly rules: readonly RuleSpec[];
  readonly routes: Readonly<Record<string, string>>;
  readonly raw: PolicyPack;

  constructor(pack: PolicyPack) {
    this.raw = pack;
    this.id = pack.id ?? "custom";
    this.version = pack.version ?? "0";
    this.name = pack.name ?? this.id;
    this.contentNote = pack.content_note ?? "";
    this.signals = pack.signals ?? {};
    this.rules = pack.rules ?? [];
    this.routes = pack.routes ?? {};
    this.categories = new Map(
      Object.entries(pack.categories ?? {}).map(([id, spec]) => [id, { ...spec, id }]),
    );
    validate(this);
  }

  /** Load the bundled pack, a pack object, or a pack read from a JSON file. */
  static load(source?: string | PolicyPack | Policy): Policy {
    if (source instanceof Policy) return source;
    if (source === undefined) return Policy.bundled();
    if (typeof source !== "string") return new Policy(source);
    if (!source.includes("/") && !source.endsWith(".json")) return Policy.bundled(source);
    return new Policy(JSON.parse(readFileSync(source, "utf8")) as PolicyPack);
  }

  static bundled(name: string = BUNDLED): Policy {
    const url = new URL(`./policies/${name}.json`, import.meta.url);
    return new Policy(JSON.parse(readFileSync(url, "utf8")) as PolicyPack);
  }

  /** Enabled categories that apply to a surface, most serious first. */
  forSurface(surface: Surface): Category[] {
    return [...this.categories.values()]
      .filter((c) => (c.enabled ?? true) && (c.surfaces ?? []).includes(surface))
      .sort(
        (a, b) =>
          (b.weight ?? 0.5) - (a.weight ?? 0.5) ||
          (b.base_severity ?? 2) - (a.base_severity ?? 2) ||
          a.id.localeCompare(b.id),
      );
  }

  /**
   * Categories that get a dedicated yes/no question on this surface.
   *
   * A choice question picks one label. Content can carry more than one hazard at once, and the
   * ones where a miss is unacceptable get their own independent question.
   */
  sentinels(surface: Surface): Category[] {
    return this.forSurface(surface).filter((c) => c.sentinel && c.sentinel_instructions);
  }

  signalsFor(surface: Surface, hasContext = false): Record<string, SignalSpec> {
    const out: Record<string, SignalSpec> = {};
    for (const [name, spec] of Object.entries(this.signals)) {
      if (!spec.surfaces.includes(surface)) continue;
      if (spec.requires_context && !hasContext) continue;
      out[name] = spec;
    }
    return out;
  }

  /** Resolve a signal's criteria, following `criteria_ref` into the pack's scales. */
  criteriaFor(spec: SignalSpec): unknown {
    if (spec.criteria !== undefined) return spec.criteria;
    return spec.criteria_ref ? this.raw[spec.criteria_ref] : undefined;
  }

  thresholds(category: Category, surface: Surface): Thresholds | undefined {
    return category.thresholds[surface] ?? category.thresholds["default"];
  }

  minConfidence(): number {
    return Number(this.raw.defaults?.["min_confidence"] ?? 0.65);
  }

  onLowConfidence(): string {
    return String(this.raw.defaults?.["on_low_confidence"] ?? "escalate");
  }

  /**
   * Whether an unreachable Jev blocks on this surface.
   *
   * Set per surface, because the two are not the same risk: the input check sits in front of a
   * model that has its own safety, so failing open there degrades gracefully, while the output
   * check is the last line and has nothing behind it.
   */
  failClosed(surface?: Surface): boolean {
    const setting = this.raw.defaults?.["on_error"] ?? "fail_closed";
    if (setting && typeof setting === "object") {
      return String((setting as Record<string, unknown>)[surface ?? ""] ?? "fail_closed") === "fail_closed";
    }
    return String(setting) === "fail_closed";
  }

  /**
   * The pack's `defaults.sentinel_corroboration`, or undefined when the pack does not set it.
   *
   * It is how a pack asks for a sentinel to be backed by the hazard choice before it can drive the
   * strongest actions on its own.
   */
  sentinelCorroboration(): SentinelCorroboration | undefined {
    const raw = this.raw.defaults?.["sentinel_corroboration"];
    if (!isRecord(raw)) return undefined;
    return {
      minChoice: numberOr(raw["min_choice"], 0.02),
      refusal: numberOr(raw["refusal"], 0.8),
      refusalMaxSentinel: numberOr(raw["refusal_max_sentinel"], 0.5),
      refusalExcept: new Set(stringsOf(raw["refusal_except"])),
      weakAtMostFlag: truthy(raw["weak_at_most_flag"]),
      redactInsteadOfBlockBelow: numberOr(raw["redact_instead_of_block_below"], 0),
      weakExcept: new Set(stringsOf(raw["weak_except"])),
    };
  }

  /** The pack's `defaults.confidence_gate` options. */
  confidenceGate(): ConfidenceGateOptions {
    const setting = this.raw.defaults?.["confidence_gate"];
    const raw = isRecord(setting) ? setting : {};
    return {
      needsCorroboration: truthy(raw["needs_corroboration"]),
      skipWhenIntent: new Set(stringsOf(raw["skip_when_intent"])),
      skipMinConfidence: numberOr(raw["skip_min_confidence"], 0.5),
    };
  }

  errorAction(): Action {
    return (this.raw.defaults?.["error_action"] ?? "review") as Action;
  }

  get policyId(): string {
    return `${this.id}@${this.version}`;
  }
}

import type { Action } from "./types.js";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function numberOr(value: unknown, fallback: number): number {
  if (typeof value === "number") return value;
  if (typeof value === "boolean") return value ? 1 : 0;
  if (typeof value === "string" && value.trim() !== "") {
    const parsed = Number(value);
    if (!Number.isNaN(parsed)) return parsed;
  }
  return fallback;
}

function stringsOf(value: unknown): string[] {
  return Array.isArray(value) ? value.map((item) => String(item)) : [];
}

/** Truthiness the way the Python and Go packages read a pack setting. */
function truthy(value: unknown): boolean {
  if (value === null || value === undefined) return false;
  if (typeof value === "boolean") return value;
  if (typeof value === "string") return value !== "";
  if (Array.isArray(value)) return value.length > 0;
  if (typeof value === "object") return Object.keys(value).length > 0;
  return typeof value !== "number" || value !== 0;
}

function validate(policy: Policy): void {
  if (policy.categories.size === 0) throw new Error("policy pack has no categories");
  for (const [id, category] of policy.categories) {
    const banded = Object.entries(category.thresholds ?? {});
    if (banded.length === 0) throw new Error(`category "${id}" has no thresholds`);
    for (const [surface, bands] of banded) {
      for (const band of ["block", "review", "flag"] as const) {
        if (typeof bands[band] !== "number") {
          throw new Error(`category "${id}" thresholds[${surface}] is missing "${band}"`);
        }
      }
      if (!(bands.block >= bands.review && bands.review >= bands.flag)) {
        throw new Error(
          `category "${id}" thresholds[${surface}] are not ordered block >= review >= flag`,
        );
      }
    }
  }
  for (const rule of policy.rules) {
    if (rule.when?.signal && !(rule.when.signal in policy.signals)) {
      throw new Error(`rule "${rule.id}" refers to unknown signal "${rule.when.signal}"`);
    }
    const added = rule.then?.add_finding;
    if (added && !policy.categories.has(added)) {
      throw new Error(`rule "${rule.id}" adds unknown category "${added}"`);
    }
  }
}
