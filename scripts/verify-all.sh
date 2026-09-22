#!/usr/bin/env bash
# Everything, in one run: both test suites, the policy pack, the CLI, every example, the offline
# tuning tool, and the packaging.
#
#   scripts/verify-all.sh
#
# CI runs the parts that can run on a clean checkout. This adds the parts that need the examples'
# optional dependencies, and it is what to run before a release. Needs no API key and no network
# beyond installing dependencies.
#
# Set REDIS_URL to also exercise the Redis session store; without it that one check is skipped.
set -uo pipefail
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

PASS=0
FAIL=0
SKIP=0
ok()   { printf '  ok    %s\n' "$1"; PASS=$((PASS + 1)); }
no()   { printf '  FAIL  %s\n' "$1"; FAIL=$((FAIL + 1)); sed 's/^/        /' /tmp/verify.out | tail -20; }
skip() { printf '  skip  %s (%s)\n' "$1" "$2"; SKIP=$((SKIP + 1)); }
# Each check runs in a subshell: a `cd` inside one must not leak into the next.
run()  { if ( eval "$2" ) >/tmp/verify.out 2>&1; then ok "$1"; else no "$1"; fi; }

section() { printf '\n== %s\n' "$1"; }

section "python: unit tests"
run "pytest" "cd python && python3 -m pytest -q"

section "typescript: typecheck, tests, published build"
run "typecheck" "cd ts && npm run typecheck"
run "tests" "cd ts && npm test"
run "build" "cd ts && npm run build && test -f dist/index.js && test -f dist/policies/standard-v1.json"
run "the build loads and carries the pack" "cd ts && node -e \"
  import('./dist/index.js').then(m => {
    if (m.Policy.bundled().categories.size !== 18) throw new Error('pack missing from build');
  })\""

section "policy pack"
run "the three shipped copies are identical" "./scripts/sync-policies.sh >/dev/null && git diff --quiet -- python/src/guardrail_chatbot_jev/policies ts/src/policies"
run "thresholds ordered, rules resolve" "python3 -c \"
import sys; sys.path.insert(0, 'python/src')
from guardrail_chatbot_jev import Policy
p = Policy.bundled()
assert len(p.categories) == 18, len(p.categories)
for cid, c in p.categories.items():
    for surface in ('input', 'output', 'conversation'):
        if c.applies(surface):
            t = c.threshold(surface)
            assert t['block'] >= t['review'] >= t['flag'], (cid, surface, dict(t))
for r in p.rules:
    assert r['when']['signal'] in p.signals, r['id']
    add = r.get('then', {}).get('add_finding')
    assert add is None or add in p.categories, r['id']
    for other in list(r.get('except_categories') or ()) + list(r.get('only_categories') or ()):
        assert other in p.categories, (r['id'], other)
\""
run "every labelled case builds a request and carries a known label" "python3 -c \"
import json, sys; sys.path.insert(0, 'python/src')
from guardrail_chatbot_jev import Guard, Policy, RecordedTransport
from guardrail_chatbot_jev.questions import conversation_state, input_state, output_state
from guardrail_chatbot_jev.types import as_turns
clean = {'hazard': {'type': 'choice', 'choice': 'none', 'confidence': 0.95, 'probabilities': {'none': 0.95}}}
guard, policy = Guard(transport=RecordedTransport(clean)), Policy.bundled()
for path, surface in (('examples/cases-input.jsonl', 'input'),
                      ('examples/cases-output.jsonl', 'output'),
                      ('examples/conversations.jsonl', 'conversation')):
    for line in open(path, encoding='utf-8'):
        if not line.strip():
            continue
        case = json.loads(line)
        if surface == 'input':
            state = input_state(case['text'])
        elif surface == 'output':
            state = output_state(case['text'], user_message=case.get('user_message'),
                                 context=case.get('context') or None)
        else:
            state = conversation_state(as_turns(case['turns']))
        request = guard.preview(surface, state, has_context=bool(case.get('context')))
        assert 'hazard' in request['questions'], case['id']
        assert case['expected_action'] in ('allow', 'flag', 'review', 'block'), case['id']
        label = case.get('expected_category')
        assert label is None or label in policy.categories, (case['id'], label)
\""

section "cli"
run "dry run needs no key" "PYTHONPATH=python/src python3 -m guardrail_chatbot_jev.cli --surface input --text 'x' --dry-run"
run "a degraded verdict does not exit 0" "PYTHONPATH=python/src python3 -c \"
import sys; sys.argv = ['x']
from guardrail_chatbot_jev.cli import main
code = main(['--surface', 'input', '--text', 'x'])
assert code == 4, code
\""

section "examples"
run "integration.py" "PYTHONPATH=python/src python3 examples/integration.py"
run "domain_policy.py" "PYTHONPATH=python/src python3 examples/domain_policy.py"
if node --experimental-strip-types --eval "" >/dev/null 2>&1; then
  run "integration.ts" "node --experimental-strip-types examples/integration.ts"
else
  skip "integration.ts" "needs Node 22.6+ for --experimental-strip-types"
fi
if python3 -c "import fastapi" >/dev/null 2>&1; then
  run "chatbot_server.py imports" "PYTHONPATH=python/src:. python3 -c 'import examples.chatbot_server'"
  run "the in-memory session store" "PYTHONPATH=python/src:. python3 -c \"
import asyncio
from examples.session_store import MemorySessionStore
async def main():
    store = MemorySessionStore(max_conversations=3, ttl=100)
    for i in range(5):
        async with store.transaction(f'c{i}') as session:
            session.add_turn('user', 'hello')
    stats = await store.stats()
    assert stats['conversations'] == 3 and stats['evicted'] == 2, stats
asyncio.run(main())
\""
else
  skip "chatbot_server.py" "pip install -e './python[server]'"
fi

section "offline tuning"
run "sweep.py on a synthetic recording" "python3 -c \"
import json, pathlib
rows = []
for i in range(12):
    firing = i < 6
    p = 0.62 if firing else 0.06
    rows.append({'id': f'c{i}', 'surface': 'input',
                 'expected_action': 'review' if firing else 'allow',
                 **({'expected_category': 'prv'} if firing else {}),
                 'answers': {
                     'hazard': {'type': 'choice', 'choice': 'prv' if firing else 'none',
                                'confidence': 0.9, 'probabilities': {'prv': p, 'none': 1 - p}},
                     'actionability': {'type': 'score', 'score': 2.0, 'confidence': 0.9,
                                       'probabilities': {}, 'legend': {}},
                     'intent': {'type': 'choice', 'choice': 'benign', 'confidence': 0.9,
                                'probabilities': {'benign': 0.9}},
                     'severity': {'type': 'score', 'score': 1.0, 'confidence': 0.9,
                                  'probabilities': {}, 'legend': {}}}})
pathlib.Path('/tmp/verify-records.jsonl').write_text('\\\\n'.join(json.dumps(r) for r in rows))
\" && ./scripts/sweep.py report /tmp/verify-records.jsonl \
   && ./scripts/sweep.py separation /tmp/verify-records.jsonl --category prv \
   && ./scripts/sweep.py sweep /tmp/verify-records.jsonl --axis prv.input.review --from 0.2 --to 0.8 --step 0.2"

section "packaging"
if python3 -c "import build" >/dev/null 2>&1; then
  run "the wheel builds and is usable from a clean venv" "(cd python && python3 -m build --wheel --outdir /tmp/verify-dist) \
     && rm -rf /tmp/verify-venv && python3 -m venv /tmp/verify-venv \
     && /tmp/verify-venv/bin/pip -q install /tmp/verify-dist/*.whl \
     && /tmp/verify-venv/bin/python -c 'from guardrail_chatbot_jev import Policy; assert len(Policy.bundled().categories) == 18' \
     && /tmp/verify-venv/bin/guardrail-chatbot-jev --surface input --text x --dry-run >/dev/null"
else
  skip "wheel build" "pip install build"
fi

section "shell scripts"
for f in ./scripts/*.sh; do run "bash -n $(basename "$f")" "bash -n '$f'"; done

section "redis session store"
if [[ -n "${REDIS_URL:-}" ]] && python3 -c "import redis" >/dev/null 2>&1; then
  run "risk carries across store instances" "PYTHONPATH=python/src:. python3 -c \"
import asyncio, os
from redis.asyncio import Redis
from examples.session_store import RedisSessionStore
async def main():
    client = Redis.from_url(os.environ['REDIS_URL'], decode_responses=True)
    prefix = 'verify:session:'
    first, second = RedisSessionStore(client, prefix=prefix), RedisSessionStore(client, prefix=prefix)
    async with first.transaction('c1') as session:
        session.add_turn('user', 'hello')
        session.risk = 0.6
    async with second.transaction('c1') as session:
        assert session.risk == 0.6 and len(session.history) == 1, session.as_dict()
    for key in await client.keys(prefix + '*'):
        await client.delete(key)
    await client.aclose()
asyncio.run(main())
\""
else
  skip "redis session store" "set REDIS_URL and pip install redis"
fi

printf '\n== %d ok, %d failed, %d skipped\n' "$PASS" "$FAIL" "$SKIP"
test "$FAIL" -eq 0
