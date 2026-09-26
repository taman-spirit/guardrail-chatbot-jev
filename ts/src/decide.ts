/**
 * The decision engine: Jev answers in, a verdict out.
 *
 * This module makes no network calls and holds no state, so the whole policy can be tested
 * against recorded answers without an API key.
 */

import type { Category, Policy, SentinelCorroboration } from "./policy.js";
import { HAZARD, NONE_LABEL, SENTINEL_PREFIX } from "./questions.js";
import {
  type Action,
  type Answers,
  type Finding,
  type Route,
  type Surface,
  type Usage,
  type Verdict,
  WITHHOLDING,
  rank,
  shift,
  stronger,
  weaker,
} from "./types.js";

/** Handling modes a category may ask for when it fires below the block line. */
const CATEGORY_ROUTES: ReadonlySet<string> = new Set(["redact", "guide", "crisis_support"]);

export interface DecideOptions {
  readonly model?: string;
  readonly usage?: Usage;
  readonly latencyMs?: number;
}

/** Turn one set of Jev answers into a verdict. */
export function decide(
  policy: Policy,
  surface: Surface,
  answers: Answers,
  options: DecideOptions = {},
): Verdict {
  const signals = readSignals(policy, answers);
  const { probabilities, confidences, sentinelSourced } = hazardProbabilities(policy, surface, answers);

  const corroboration = policy.sentinelCorroboration();
  const choice = choiceProbabilities(answers);

  let findings: Finding[] = [];
  for (const category of policy.forSurface(surface)) {
    const sentinel = sentinelSourced.has(category.id);
    const uncorroborated =
      corroboration !== undefined && sentinel && (choice[category.id] ?? 0) < corroboration.minChoice;
    const finding = toFinding(
      policy,
      category,
      surface,
      probabilities[category.id] ?? 0,
      confidences[category.id] ?? 1,
      sentinel,
      uncorroborated,
    );
    if (finding) findings.push(finding);
  }

  const { floors, applied } = applyRules(policy, surface, findings, signals, probabilities);
  findings = floors.findings;
  if (corroboration && surface === "output") {
    findings = capUncorroboratedOnRefusal(findings, signals, corroboration);
  }
  if (corroboration && corroboration.redactInsteadOfBlockBelow > 0) {
    // Measured: a reply with a first name in it ("Kính gửi anh Nam") drew s_prv 0.40 to 0.50 with
    // the choice at 0.01. Personal data is handled by masking it; blocking the whole reply on a
    // sentinel alone is the wrong remedy.
    findings = findings.map((f) =>
      f.uncorroborated &&
      f.action === "block" &&
      f.probability < corroboration.redactInsteadOfBlockBelow &&
      policy.categories.get(f.category)?.route === "redact"
        ? withAction(f, "review", "uncorroborated sentinel on a redacted category: block -> review")
        : f,
    );
  }
  if (corroboration && corroboration.weakAtMostFlag) {
    findings = findings.map((f) =>
      WEAK.has(f) && rank(f.action) > rank("flag")
        ? withAction(f, "flag", `uncorroborated sentinel below its block band: ${f.action} -> flag`)
        : f,
    );
  }

  let action: Action = "allow";
  for (const finding of findings) action = stronger(action, finding.action);
  for (const floor of floors.actions) action = stronger(action, floor);

  const confidence = overallConfidence(answers, findings, confidences);
  const gated = confidenceGate(
    policy,
    action,
    confidence,
    findings,
    probabilities,
    surface,
    choice,
    signals,
    corroboration?.minChoice ?? 0,
  );
  if (gated !== action) applied.push("confidence-gate");
  action = gated;

  sortFindings(findings);
  const route = resolveRoute(policy, findings, action);

  return {
    action,
    allowed: rank(action) <= rank("flag"),
    deliverable: !WITHHOLDING.has(route),
    surface,
    route,
    findings,
    signals,
    confidence,
    severity: severityOf(signals, findings),
    appliedRules: applied,
    policyId: policy.policyId,
    model: options.model ?? "",
    usage: options.usage ?? { inputTokens: 0, outputTokens: 0 },
    latencyMs: options.latencyMs ?? 0,
    degraded: false,
    cached: false,
    partial: false,
  };
}

/** Findings ordered most serious first: by action, then by probability. Stable. */
export function sortFindings(findings: Finding[]): Finding[] {
  return findings.sort((a, b) => rank(b.action) - rank(a.action) || b.probability - a.probability);
}

/**
 * Raise a verdict to at least `floor`, recomputing the route to match.
 *
 * Used to carry risk forward: a session that has already shown an escalation pattern should not
 * have its next turn judged as if the conversation had just started.
 */
export function withFloor(policy: Policy, verdict: Verdict, floor: Action, note: string): Verdict {
  const action = stronger(verdict.action, floor);
  if (action === verdict.action) return verdict;
  const route = resolveRoute(policy, verdict.findings, action);
  return {
    ...verdict,
    action,
    route,
    allowed: rank(action) <= rank("flag"),
    deliverable: !WITHHOLDING.has(route),
    appliedRules: [...verdict.appliedRules, note],
  };
}

/**
 * The verdict to use when Jev could not be reached.
 *
 * Fail-closed is the default: an unavailable guardrail is not an approval.
 */
export function errorVerdict(
  policy: Policy,
  surface: Surface,
  error: unknown,
  latencyMs = 0,
): Verdict {
  const action: Action = policy.failClosed(surface) ? policy.errorAction() : "allow";
  const route: Route = action === "block" ? "safe_response" : action === "review" ? "human_review" : "deliver";
  return {
    action,
    allowed: rank(action) <= rank("flag"),
    deliverable: !WITHHOLDING.has(route),
    surface,
    route,
    findings: [],
    signals: {},
    confidence: 0,
    severity: 0,
    appliedRules: [],
    policyId: policy.policyId,
    model: "",
    usage: { inputTokens: 0, outputTokens: 0 },
    latencyMs,
    degraded: true,
    cached: false,
    partial: false,
    error: error instanceof Error ? `${error.name}: ${error.message}` : String(error),
  };
}

/**
 * Lower to flag an uncorroborated sentinel finding on a reply that declines.
 *
 * Measured against Jev, a plain refusal ("I can't help with that") drew sentinel answers of up to
 * 0.26 for categories the reply never touched, while the hazard choice gave them nothing. The
 * categories a refusal can still leak, such as a system prompt or personal data, are exempt.
 */
function capUncorroboratedOnRefusal(
  findings: Finding[],
  signals: Record<string, number | string>,
  c: SentinelCorroboration,
): Finding[] {
  const refusal = signals["refusal"];
  if (typeof refusal !== "number" || refusal < c.refusal) return findings;
  return findings.map((f) =>
    !f.uncorroborated ||
    f.probability >= c.refusalMaxSentinel ||
    c.refusalExcept.has(f.category) ||
    rank(f.action) <= rank("flag")
      ? f
      : withAction(f, "flag", `refusal with an uncorroborated sentinel: ${f.action} -> flag`),
  );
}

// -- internals --------------------------------------------------------

/**
 * Uncorroborated findings below their block band. never_below does not lift them. Kept off the
 * public shape, as the Go package keeps its `weak` field unexported.
 */
const WEAK = new WeakSet<Finding>();

/** A copy of a finding at a new action with a note, keeping its internal weak mark. */
function withAction(finding: Finding, action: Action, note: string): Finding {
  const next: Finding = { ...finding, action, notes: [...finding.notes, note] };
  if (WEAK.has(finding)) WEAK.add(next);
  return next;
}

/** The hazard choice's own probabilities, before any sentinel is folded in. */
function choiceProbabilities(answers: Answers): Record<string, number> {
  const hazard = answers[HAZARD];
  if (!hazard || hazard.type !== "choice") return {};
  const out: Record<string, number> = {};
  for (const [label, value] of Object.entries(hazard.probabilities ?? {})) {
    if (typeof value === "number") out[label] = value;
  }
  return out;
}

function readSignals(policy: Policy, answers: Answers): Record<string, number | string> {
  const signals: Record<string, number | string> = {};
  for (const name of Object.keys(policy.signals)) {
    const answer = answers[name];
    if (!answer) continue;
    if (answer.type === "noul") signals[name] = answer.noul;
    else if (answer.type === "score") signals[name] = answer.score;
    else signals[name] = answer.choice;
  }
  return signals;
}

/** Per-category probability, taking the stronger of the choice question and its sentinel. */
function hazardProbabilities(
  policy: Policy,
  surface: Surface,
  answers: Answers,
): {
  probabilities: Record<string, number>;
  confidences: Record<string, number>;
  sentinelSourced: Set<string>;
} {
  const probabilities: Record<string, number> = {};
  const confidences: Record<string, number> = {};
  const sentinelSourced = new Set<string>();

  const hazard = answers[HAZARD];
  const hazardConfidence = hazard && hazard.type === "choice" ? hazard.confidence : 1;
  if (hazard && hazard.type === "choice") {
    for (const [label, value] of Object.entries(hazard.probabilities ?? {})) {
      if (label === NONE_LABEL) continue;
      probabilities[label] = value;
      confidences[label] = hazard.confidence;
    }
  }

  for (const category of policy.sentinels(surface)) {
    const answer = answers[SENTINEL_PREFIX + category.id];
    if (!answer || answer.type !== "noul") continue;
    if (answer.noul >= (probabilities[category.id] ?? 0)) {
      probabilities[category.id] = answer.noul;
      sentinelSourced.add(category.id);
      // A noul reports belief, not uncertainty: 0.4 means "40% likely", which the threshold
      // already accounts for. It carries no confidence of its own, so the request-level
      // confidence stands in. Reading distance from 0.5 as doubt would double-count the
      // probability and send every mid-range sentinel to review whatever the threshold says.
      confidences[category.id] = hazardConfidence;
    }
  }
  return { probabilities, confidences, sentinelSourced };
}

function toFinding(
  policy: Policy,
  category: Category,
  surface: Surface,
  probability: number,
  confidence: number,
  sentinel = false,
  uncorroborated = false,
): Finding | undefined {
  const bands = policy.thresholds(category, surface);
  if (!bands) return undefined;

  let action: Action;
  if (probability >= bands.block) action = "block";
  else if (probability >= bands.review) action = "review";
  else if (probability >= bands.flag) action = "flag";
  else return undefined;

  const notes: string[] = [];
  if (category.route && CATEGORY_ROUTES.has(category.route)) notes.push(`handled by ${category.route}`);
  // A sentinel alone, which the hazard choice does not back, answers at its own band: it may still
  // reach block by crossing the block band, but never_below does not lift it there.
  const weak = uncorroborated && probability < bands.block;
  if (category.never_below && weak) {
    notes.push(`uncorroborated sentinel: never below ${category.never_below} not applied`);
  } else if (category.never_below) {
    action = stronger(action, category.never_below);
    notes.push(`never below ${category.never_below}`);
  }

  const finding: Finding = {
    category: category.id,
    name: category.name,
    probability,
    confidence,
    action,
    severity: category.base_severity ?? 2,
    refs: category.refs ?? [],
    source: sentinel ? "sentinel" : "category",
    notes,
    uncorroborated,
  };
  if (weak) WEAK.add(finding);
  return finding;
}

function matches(op: string, value: unknown, target: unknown): boolean {
  if (op === "==" || op === "!=") {
    const equal = String(value) === String(target);
    return op === "==" ? equal : !equal;
  }
  const left = Number(value);
  const right = Number(target);
  if (Number.isNaN(left) || Number.isNaN(right)) return false;
  switch (op) {
    case ">=":
      return left >= right;
    case "<=":
      return left <= right;
    case ">":
      return left > right;
    case "<":
      return left < right;
    default:
      return false;
  }
}

function applyRules(
  policy: Policy,
  surface: Surface,
  findings: Finding[],
  signals: Record<string, number | string>,
  probabilities: Record<string, number>,
): { floors: { findings: Finding[]; actions: string[] }; applied: string[] } {
  const actions: string[] = [];
  const applied: string[] = [];
  let current = findings;

  for (const rule of policy.rules) {
    const signal = rule.when?.signal;
    if (!signal || !(signal in signals)) continue;
    if (!matches(rule.when.op ?? "==", signals[signal], rule.when.value)) continue;

    applied.push(rule.id);
    const exempt = new Set(rule.except_categories ?? []);

    const added = rule.then?.add_finding;
    if (added && !current.some((f) => f.category === added)) {
      const category = policy.categories.get(added);
      if (category && (category.enabled ?? true) && (category.surfaces ?? []).includes(surface)) {
        current = [
          ...current,
          {
            category: category.id,
            name: category.name,
            probability: probabilities[category.id] ?? 0,
            confidence: 1,
            action: "flag",
            severity: category.base_severity ?? 2,
            refs: category.refs ?? [],
            source: `rule:${rule.id}`,
            notes: [`raised by ${rule.id}`],
            uncorroborated: false,
          },
        ];
      }
    }

    const steps = (rule.then?.upgrade ?? 0) - (rule.then?.downgrade ?? 0);
    const cap = rule.then?.cap_action;
    if (steps !== 0 || cap) {
      current = current.map((finding) =>
        exempt.has(finding.category) ? finding : adjust(policy, finding, steps, cap, rule.id),
      );
    }

    if (rule.then?.floor_action) actions.push(rule.then.floor_action);
  }

  return { floors: { findings: current, actions }, applied };
}

function adjust(
  policy: Policy,
  finding: Finding,
  steps: number,
  cap: string | undefined,
  ruleId: string,
): Finding {
  let action: Action = steps === 0 ? finding.action : shift(finding.action, steps);
  if (steps < 0) {
    // A softening rule lowers the response, it does not erase the record: a finding that fired
    // stays visible at "flag" so the deployment can still count and audit it.
    action = stronger(action, "flag");
  }
  if (cap) action = weaker(action, cap);
  const neverBelow = policy.categories.get(finding.category)?.never_below;
  if (neverBelow && !WEAK.has(finding)) action = stronger(action, neverBelow);
  if (action === finding.action) return finding;
  return withAction(finding, action, `${ruleId}: ${finding.action} -> ${action}`);
}

function overallConfidence(
  answers: Answers,
  findings: readonly Finding[],
  confidences: Record<string, number>,
): number {
  if (findings.length > 0) {
    return Math.min(...findings.map((f) => confidences[f.category] ?? 1));
  }
  const hazard = answers[HAZARD];
  return hazard && hazard.type === "choice" ? hazard.confidence : 1;
}

/** A low-confidence answer is not evidence of safety, so it escalates toward review. */
function confidenceGate(
  policy: Policy,
  action: Action,
  confidence: number,
  findings: readonly Finding[],
  probabilities: Record<string, number>,
  surface: Surface,
  choice: Record<string, number>,
  signals: Record<string, number | string>,
  minChoice: number,
): Action {
  if (confidence >= policy.minConfidence() || policy.onLowConfidence() !== "escalate") return action;
  const options = policy.confidenceGate();
  const intent = signals["intent"];
  if (
    typeof intent === "string" &&
    options.skipWhenIntent.has(intent) &&
    confidence >= options.skipMinConfidence
  ) {
    return action;
  }
  let counted = findings;
  let candidates = Object.entries(probabilities);
  if (options.needsCorroboration) {
    // Only what the hazard choice backs counts: a sentinel on its own is already answered at its
    // own band, and is not a reason to hold on top of that.
    counted = findings.filter((f) => !f.uncorroborated);
    candidates = candidates.filter(([id]) => (choice[id] ?? 0) >= minChoice);
  }
  const nearMiss = candidates.some(([id, probability]) => {
    const category = policy.categories.get(id);
    if (!category) return false;
    const bands = policy.thresholds(category, surface);
    return probability >= (bands ? bands.flag : 1) * 0.5;
  });
  if (counted.length === 0 && !nearMiss) return action;
  return rank(action) >= rank("review") ? action : "review";
}

/**
 * How the deployment should handle the content, given the decision and the hazard.
 *
 * The action says whether the content goes out; the route says what to do about it.
 */
export function resolveRoute(policy: Policy, findings: readonly Finding[], action: Action): Route {
  // Only a finding that still counts sets the handling. One a rule capped to allow is a record,
  // and a sentinel the hazard choice does not back, still at flag, must not replace an ordinary
  // answer with a crisis message.
  const hazardRoute = findings
    .filter((f) => rank(f.action) >= rank("flag") && !(f.uncorroborated && f.action === "flag"))
    .map((f) => policy.categories.get(f.category)?.route)
    .find((route): route is NonNullable<typeof route> => !!route && CATEGORY_ROUTES.has(route));

  if (hazardRoute === "crisis_support" && action !== "allow") return "crisis_support";
  if (action === "block") return "safe_response";
  if (action === "allow") return "deliver";
  if (hazardRoute === "redact" || hazardRoute === "guide") return hazardRoute;
  return action === "review" ? "human_review" : "deliver";
}

function severityOf(
  signals: Record<string, number | string>,
  findings: readonly Finding[],
): number {
  const reported = signals["severity"];
  if (typeof reported === "number") return reported;
  return findings.reduce((max, f) => Math.max(max, f.severity), 0);
}
