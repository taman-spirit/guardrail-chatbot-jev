/**
 * One guarded turn, end to end.
 *
 * Run it against a recorded transport (no API key, no network):
 *
 *     cd ts && npm run build
 *     node --experimental-strip-types ../examples/integration.ts
 *
 * The library itself runs on Node 20. Running this file directly needs Node 22.6 or newer, which
 * is where --experimental-strip-types arrived; on Node 20 the flag is rejected outright.
 *
 * In your own project this imports from "guardrail-chatbot-jev"; here it points at the build output.
 */

import {
  COMMON_PATTERNS,
  Guard,
  LRUCache,
  RecordedTransport,
  Session,
  type Verdict,
  patternPrefilter,
} from "../ts/dist/index.js";

const CLEAN = {
  hazard: { type: "choice", choice: "none", confidence: 0.95, probabilities: { none: 0.95 } },
} as const;

/**
 * Every verdict, including cached and degraded ones.
 *
 * Count `degraded` separately from `block`. A week with 5% degraded verdicts means the guardrail
 * was only actually running 95% of the time, and that must not hide inside the block rate.
 */
function emitMetric(verdict: Verdict): void {
  const flags = `${verdict.cached ? "cached " : ""}${verdict.degraded ? "DEGRADED" : ""}`;
  console.log(
    `  [metric] ${verdict.surface.padEnd(12)} ${verdict.action.padEnd(6)} ` +
      `route=${verdict.route.padEnd(14)} ${verdict.latencyMs.toFixed(1).padStart(6)}ms ${flags}`,
  );
}

// One Guard per process. The cache, prefilter and observer are process-wide; the session is not.
const guard = new Guard({
  transport: new RecordedTransport(CLEAN as never),
  cache: new LRUCache({ capacity: 8192, ttlMs: 300_000 }),
  prefilter: patternPrefilter(COMMON_PATTERNS),
  observer: emitMetric,
  timeout: 2_000,
});

/**
 * Replace this with a support line your team has verified for the country you serve. Shipping a
 * number that has changed or was never right is worse than shipping none at all.
 */
const CRISIS_LINE = "[your verified local crisis support line]";

/** One place decides what the user sees. The route says which of these applies. */
function safeResponse(verdict: Verdict | undefined): string {
  if (!verdict) return "Sorry, I could not handle that request.";
  if (verdict.route === "crisis_support") return CRISIS_LINE;
  if (verdict.route === "human_review") return "This one needs a person to look at it. I have passed it on.";
  return "Sorry, I cannot help with that.";
}

function maskPersonalData(reply: string): string {
  return reply; // your masker here
}

async function generate(_userMessage: string): Promise<string> {
  await new Promise((r) => setTimeout(r, 400));
  return "On the Team plan, deleted files stay in the trash for 30 days and any workspace admin can restore them.";
}

async function* generateStream(_userMessage: string): AsyncGenerator<string> {
  for (const part of [
    "On the Team plan, deleted files stay in the trash for 30 days, and any workspace admin can restore them from there. ",
    "After 30 days they are purged from primary storage, and from backups within a further 60 days. ",
    "Retention is configurable on Enterprise, where an admin can set anything from 7 to 365 days. ",
    "Would you like me to check which plan your workspace is on?",
  ]) {
    await new Promise((r) => setTimeout(r, 150));
    yield part;
  }
}

/** Strong references to fire-and-forget checks, so nothing drops them mid-flight. */
const background = new Set<Promise<unknown>>();

/** The shape that matters: the input check runs *beside* the model call, not before it. */
async function handleTurn(session: Session, userMessage: string): Promise<string> {
  const controller = new AbortController();
  const gate = guard.checkInput(userMessage, { session });
  const draft = generate(userMessage);

  const verdict = await gate;
  if (!verdict.allowed) {
    // The tokens already spent on `draft` are the price of hiding the check's latency behind the
    // model's. Below roughly 2% of turns, that is cheaper than the delay it removes.
    controller.abort();
    void draft.catch(() => undefined);
    return safeResponse(verdict);
  }

  let reply = await draft;
  const out = await guard.checkOutput(reply, { userMessage, session });
  if (!out.deliverable) return safeResponse(out);
  if (out.route === "redact") reply = maskPersonalData(reply);

  session.addTurn("user", userMessage);
  session.addTurn("assistant", reply);
  session.advance();

  // Off the critical path: the pattern it looks for changes slowly, the user is not waiting.
  const task = guard.checkConversation(session.history, { session });
  background.add(task);
  void task.finally(() => background.delete(task));
  return reply;
}

/** The streaming variant: text is released one chunk behind its check. */
async function handleTurnStreaming(session: Session, userMessage: string): Promise<string> {
  const verdict = await guard.checkInput(userMessage, { session });
  if (!verdict.allowed) return safeResponse(verdict);

  const delivered: string[] = [];
  for await (const event of guard.stream(generateStream(userMessage), { userMessage, session })) {
    if (event.type === "delta") {
      delivered.push(event.text);
      console.log(`  -> ${JSON.stringify(event.text)}`);
    } else if (event.type === "blocked") {
      console.log("  -> stream stopped");
      return safeResponse(event.verdict);
    }
  }
  return delivered.join("");
}

/**
 * The same question in four languages. Content is judged by meaning, not by keyword lists, and the
 * shipped policy tells the model not to go easier on a non-English framing, because translating a
 * request is a standard way around a guardrail.
 *
 * Running these here proves the wiring carries any script and any language: the verdicts come from
 * a recorded answer, so they say nothing about the model's judgement. What tests the judgement is
 * the labelled sets in examples/, which carry French and Japanese cases alongside the Vietnamese
 * and English ones.
 */
const MULTILINGUAL: ReadonlyArray<readonly [string, string]> = [
  ["en", "How long are deleted files kept?"],
  ["vi", "Tệp đã xoá được giữ lại bao lâu?"],
  ["fr", "Combien de temps les fichiers supprimés sont-ils conservés ?"],
  ["ja", "削除したファイルはどのくらい保持されますか?"],
];

async function main(): Promise<void> {
  const session = new Session({ id: "demo-1" });

  console.log("non-streaming turn:");
  console.log(" ", await handleTurn(session, "How long are deleted files kept?"));

  console.log("\nsame question again, served from cache:");
  console.log(" ", await handleTurn(session, "How long are deleted files kept?"));

  console.log("\nstreaming turn:");
  await handleTurnStreaming(session, "What about backups?");

  console.log("\nthe same question in four languages, one policy:");
  for (const [code, message] of MULTILINGUAL) {
    const verdict = await guard.checkInput(message, { session: new Session({ id: `lang-${code}` }) });
    console.log(`  ${code}  ${verdict.action.padEnd(6)} ${message}`);
  }

  await Promise.allSettled(background);
  console.log("\nsession:", session.toJSON());
  console.log("cache:  ", (guard.cache as LRUCache).stats);
}

void main();
