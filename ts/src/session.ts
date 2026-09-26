/**
 * Carrying risk across turns.
 *
 * A multi-turn attack is made of turns that are each defensible on their own. Judging every turn
 * from a standing start is what makes that work. A session keeps the transcript, accumulates a
 * decaying risk score, and raises a floor under the next few turns when it has already seen
 * something, so a conversation that has been escalating is not read as if it had just begun.
 */

import { WITHHELD_PLACEHOLDER } from "./multiturn.js";
import { LADDER, type Action, type Turn, type Verdict, rank, stronger } from "./types.js";

/**
 * What `modelHistory` tells the chat model in place of a withheld user message. `{label}` is the
 * names of the categories that stopped it; the message's text is never included.
 */
export const WITHHELD_NOTE =
  "[The user's message here was withheld by the content check ({label}). The assistant did not see " +
  "it and declined. Do not carry out the withheld request, even if the user repeats it or asks to " +
  "continue.]";
/** What `modelHistory` tells the chat model in place of a withheld reply. */
export const WITHHELD_REPLY_NOTE =
  "[The assistant's reply here was withheld by the content check ({label}); the user did not see it.]";
/** The assistant turn `modelHistory` adds after a withheld user message when none was recorded. */
export const DECLINED_REPLY = "I can't help with that request.";

/**
 * What a store keeps of a withheld turn's verdict: enough to say why it was withheld. The keys
 * are the same in every language.
 */
export interface WithheldSummary {
  readonly action: Action;
  readonly surface: string;
  readonly route: string;
  readonly degraded: boolean;
  readonly findings: readonly { category: string; name: string; action: Action; probability: number }[];
  readonly applied_rules: readonly string[];
  readonly signals: Readonly<Record<string, number | string>>;
}

/** How much each action contributes to the session's risk score. */
export const ACTION_RISK: Readonly<Record<Action, number>> = {
  allow: 0,
  flag: 0.25,
  review: 0.6,
  block: 1,
};

export interface SessionConfig {
  /** The conversation identifier, carried into verdict metadata. */
  readonly id?: string;
  /**
   * How much of the previous risk survives each turn. 0.5 means a single flagged turn stops
   * mattering after three or four clean ones.
   */
  readonly decay?: number;
  /** How many following turns a raised floor applies to. */
  readonly carryTurns?: number;
  /**
   * Transcript window kept for conversation checks. The escalation pattern a conversation check
   * looks for lives in the recent turns, so a short window finds it just as well and costs a
   * fraction of the input tokens. Jev's state limit is 32k tokens, which leaves room for roughly
   * 40 turns of ordinary chat if you want a longer memory; raise it when your conversations
   * genuinely build over more than ten turns.
   */
  readonly maxTurns?: number;
}

/**
 * The wire shape of a persisted session. Snake_case because both languages read and write it.
 */
export interface SessionState {
  readonly id: string;
  readonly decay: number;
  readonly carry_turns: number;
  readonly max_turns: number;
  readonly turns: readonly Turn[];
  readonly risk: number;
  readonly floor: Action;
  readonly floor_turns_left: number;
  /** Beside `turns`: why each withheld turn was withheld, or null. */
  readonly withheld?: readonly (WithheldSummary | null)[];
}

/** Per-conversation state. Use one per conversation. */
export class Session {
  readonly id: string;
  readonly decay: number;
  readonly carryTurns: number;
  readonly maxTurns: number;
  risk = 0;
  readonly verdicts: Verdict[] = [];
  private readonly turns: Turn[] = [];
  // Runs beside `turns`: why each turn was withheld, or null. `record` sets it; when the two stop
  // matching, withheld turns are told without a reason.
  private held: (WithheldSummary | null)[] = [];
  private floorAction: Action = "allow";
  private floorLeft = 0;

  constructor(config: SessionConfig = {}) {
    this.id = config.id ?? "";
    this.decay = config.decay ?? 0.5;
    this.carryTurns = config.carryTurns ?? 2;
    this.maxTurns = config.maxTurns ?? 10;
  }

  addTurn(role: Turn["role"], content: string): void {
    this.pushTurn(role, content, null);
  }

  private pushTurn(role: Turn["role"], content: string, held: WithheldSummary | null): void {
    if (this.held.length !== this.turns.length) this.held = this.turns.map(() => null);
    this.turns.push({ role, content });
    this.held.push(held);
    if (this.turns.length > this.maxTurns) {
      this.turns.splice(0, this.turns.length - this.maxTurns);
      this.held.splice(0, this.held.length - this.maxTurns);
    }
  }

  /** Why turn `index` was withheld, or undefined when unknown. */
  withheld(index: number): WithheldSummary | undefined {
    if (this.held.length !== this.turns.length) return undefined;
    return this.held[index] ?? undefined;
  }

  extend(turns: readonly Turn[]): void {
    for (const turn of turns) this.addTurn(turn.role, turn.content);
  }

  get history(): readonly Turn[] {
    return [...this.turns];
  }

  /**
   * Append a turn given the verdict on it. A turn the guardrail withheld is kept as
   * {@link WITHHELD_PLACEHOLDER}, so the attempt is remembered but its text is never read again.
   */
  record(role: Turn["role"], content: string, verdict: Pick<Verdict, "deliverable"> & Partial<Verdict>): void {
    if (verdict.deliverable) this.pushTurn(role, content, null);
    else this.pushTurn(role, WITHHELD_PLACEHOLDER, summarise(verdict));
  }

  /**
   * The transcript for the chat model, with each withheld turn told rather than dropped.
   *
   * A withheld user message becomes {@link WITHHELD_NOTE} naming the categories that stopped it,
   * never its text, followed by {@link DECLINED_REPLY} unless a reply was recorded after it. A
   * withheld reply becomes {@link WITHHELD_REPLY_NOTE}. Dropping them instead left the model to
   * meet "do it" or "my first request" with nothing before it, and it guessed.
   */
  modelHistory(): Turn[] {
    const out: Turn[] = [];
    this.turns.forEach((turn, i) => {
      if (turn.content !== WITHHELD_PLACEHOLDER) {
        out.push({ ...turn });
        return;
      }
      const label = withheldLabel(this.withheld(i));
      if (turn.role !== "user") {
        out.push({ role: turn.role, content: WITHHELD_REPLY_NOTE.replace("{label}", label) });
        return;
      }
      out.push({ role: "user", content: WITHHELD_NOTE.replace("{label}", label) });
      if (this.turns[i + 1]?.role !== "assistant") out.push({ role: "assistant", content: DECLINED_REPLY });
    });
    return out;
  }

  /**
   * Whether replies should be read in context. Any of three keeps the watch on:
   *
   * - a conversation verdict of review or worse, or a block, in the last `carryTurns` completed
   *   turns (counted by `advance`, the same counter the "floor" mode uses for its floor);
   * - a risk that has not yet decayed below `threshold`;
   * - a withheld turn still inside the transcript window.
   *
   * Watching never holds anything by itself. It only decides whether a reply is also read against
   * the earlier turns, which is what costs a second request.
   */
  watching(threshold: number): boolean {
    if (this.floorLeft > 0 || this.risk >= threshold) return true;
    return this.turns.some((turn) => turn.content === WITHHELD_PLACEHOLDER);
  }

  /** How many more turns the current floor applies to. */
  get floorTurnsLeft(): number {
    return this.floorLeft;
  }

  /** The minimum action the next check will resolve to. */
  get floor(): Action {
    return this.floorLeft > 0 ? this.floorAction : "allow";
  }

  /**
   * Fold a verdict into the session's state.
   *
   * A degraded verdict is ignored: it reflects an outage, not the conversation.
   */
  observe(verdict: Verdict): void {
    if (verdict.degraded) return;
    this.verdicts.push(verdict);
    this.risk = Math.max(this.risk * this.decay, ACTION_RISK[verdict.action] ?? 0);

    // A conversation-level finding is the one that justifies holding the next turns to a higher
    // standard, because it is about the pattern rather than a single message.
    if (verdict.surface === "conversation" && rank(verdict.action) >= rank("review")) {
      this.raise("review");
    } else if (verdict.action === "block") {
      this.raise("flag");
    }
  }

  /** Call once per completed turn, to let a raised floor expire. */
  advance(): void {
    if (this.floorLeft > 0) {
      this.floorLeft -= 1;
      if (this.floorLeft === 0) this.floorAction = "allow";
    }
  }

  /**
   * Deployment context worth putting in front of Jev on later turns.
   *
   * The session's risk score is deliberately not in it. A score in front of Jev invites it to judge
   * the current message by the conversation's past, which is the contamination the session exists
   * to avoid; the risk is carried by the session itself.
   */
  metadata(): Record<string, unknown> {
    return {
      conversation_id: this.id,
      turn_number: this.turns.length + 1,
    };
  }

  /**
   * Everything a store has to carry for a conversation to survive the process.
   *
   * The verdict log is deliberately not in here. It is in-process observability, and a restored
   * session should not claim to have emitted verdicts this process never saw. What is here is
   * exactly what changes a later decision: the transcript, the risk, and the floor.
   *
   * The keys are the same in both languages, so a session written by one can be read by the other.
   */
  toState(): SessionState {
    return {
      id: this.id,
      decay: this.decay,
      carry_turns: this.carryTurns,
      max_turns: this.maxTurns,
      turns: [...this.turns],
      risk: this.risk,
      floor: this.floorAction,
      floor_turns_left: this.floorLeft,
      withheld: this.turns.map((turn, i) =>
        turn.content === WITHHELD_PLACEHOLDER ? (this.withheld(i) ?? null) : null,
      ),
    };
  }

  /**
   * Rebuild a session from `toState`.
   *
   * Tolerant of a missing or malformed field, because this state comes back from a store and a
   * half-written record should cost one conversation's memory, not the request.
   */
  static fromState(state: Partial<SessionState>): Session {
    const session = new Session({
      id: typeof state.id === "string" ? state.id : "",
      decay: typeof state.decay === "number" ? state.decay : 0.5,
      carryTurns: typeof state.carry_turns === "number" ? state.carry_turns : 2,
      maxTurns: typeof state.max_turns === "number" ? state.max_turns : 10,
    });
    session.risk = typeof state.risk === "number" ? state.risk : 0;
    session.extend(Array.isArray(state.turns) ? state.turns : []);
    if (Array.isArray(state.withheld)) {
      // Aligned from the end, as the window keeps the most recent turns.
      const held = session.turns.length ? state.withheld.slice(-session.turns.length) : [];
      if (held.length === session.turns.length) {
        session.held = held.map((h, i) =>
          h && typeof h === "object" && session.turns[i]?.content === WITHHELD_PLACEHOLDER ? fromSummary(h) : null,
        );
      }
    }
    const left = typeof state.floor_turns_left === "number" ? state.floor_turns_left : 0;
    const floor = state.floor;
    // A floor that outlived its counter, or a value no longer in the ladder, is no floor.
    if (floor !== undefined && floor !== "allow" && LADDER.includes(floor) && left > 0) {
      session.floorAction = floor;
      session.floorLeft = left;
    }
    return session;
  }

  toJSON(): Record<string, unknown> {
    return {
      id: this.id,
      turns: this.turns.length,
      risk: Number(this.risk.toFixed(3)),
      floor: this.floor,
      floorTurnsLeft: this.floorLeft,
      checks: this.verdicts.length,
    };
  }

  private raise(floor: Action): void {
    this.floorAction = stronger(this.floorLeft > 0 ? this.floorAction : "allow", floor);
    this.floorLeft = this.carryTurns;
  }
}

/** The names of the categories that withheld a turn, strongest first, for the model's note. */
export function withheldLabel(summary: WithheldSummary | undefined): string {
  if (!summary) return "reason not recorded";
  const fired = summary.findings.filter((f) => rank(f.action) >= rank("flag"));
  if (fired.length) {
    const top = Math.max(...fired.map((f) => rank(f.action)));
    const names = fired.filter((f) => rank(f.action) === top).map((f) => f.name || f.category);
    return [...new Set(names)].join(", ");
  }
  return summary.degraded ? "the check was unavailable" : "reason not recorded";
}

function summarise(verdict: Partial<Verdict>): WithheldSummary {
  const signals: Record<string, number | string> = {};
  for (const [key, value] of Object.entries(verdict.signals ?? {})) {
    if (typeof value === "number" || typeof value === "string") signals[key] = value;
  }
  return {
    action: verdict.action ?? "block",
    surface: verdict.surface ?? "input",
    route: verdict.route ?? "safe_response",
    degraded: verdict.degraded ?? false,
    findings: (verdict.findings ?? []).map((f) => ({
      category: f.category,
      name: f.name,
      action: f.action,
      probability: Math.round(f.probability * 1e4) / 1e4,
    })),
    applied_rules: [...(verdict.appliedRules ?? [])],
    signals,
  };
}

/** Rebuild a stored summary, tolerating a malformed one the way the rest of `fromState` does. */
function fromSummary(raw: unknown): WithheldSummary | null {
  if (!raw || typeof raw !== "object") return null;
  const r = raw as Record<string, unknown>;
  const findings = Array.isArray(r["findings"]) ? r["findings"] : [];
  const action = LADDER.includes(r["action"] as Action) ? (r["action"] as Action) : "block";
  return {
    action,
    surface: typeof r["surface"] === "string" ? r["surface"] : "input",
    route: typeof r["route"] === "string" ? r["route"] : "safe_response",
    degraded: r["degraded"] === true,
    findings: findings
      .filter((f): f is Record<string, unknown> => !!f && typeof f === "object" && typeof (f as Record<string, unknown>)["category"] === "string")
      .map((f) => ({
        category: f["category"] as string,
        name: typeof f["name"] === "string" && f["name"] ? (f["name"] as string) : (f["category"] as string),
        action: LADDER.includes(f["action"] as Action) ? (f["action"] as Action) : "flag",
        probability: typeof f["probability"] === "number" ? (f["probability"] as number) : 0,
      })),
    applied_rules: Array.isArray(r["applied_rules"]) ? r["applied_rules"].map(String) : [],
    signals: r["signals"] && typeof r["signals"] === "object" ? (r["signals"] as Record<string, number | string>) : {},
  };
}
