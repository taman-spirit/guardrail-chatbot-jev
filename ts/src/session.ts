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
  private floorAction: Action = "allow";
  private floorLeft = 0;

  constructor(config: SessionConfig = {}) {
    this.id = config.id ?? "";
    this.decay = config.decay ?? 0.5;
    this.carryTurns = config.carryTurns ?? 2;
    this.maxTurns = config.maxTurns ?? 10;
  }

  addTurn(role: Turn["role"], content: string): void {
    this.turns.push({ role, content });
    if (this.turns.length > this.maxTurns) this.turns.splice(0, this.turns.length - this.maxTurns);
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
  record(role: Turn["role"], content: string, verdict: Pick<Verdict, "deliverable">): void {
    this.addTurn(role, verdict.deliverable ? content : WITHHELD_PLACEHOLDER);
  }

  /**
   * The transcript to send to the chat model: withheld turns are left out, so the model never
   * sees a blocked request, not even as a placeholder it might try to answer.
   */
  modelHistory(): Turn[] {
    return this.turns.filter((turn) => turn.content !== WITHHELD_PLACEHOLDER);
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
