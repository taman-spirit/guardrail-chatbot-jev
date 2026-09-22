#!/usr/bin/env bash
# Run the labelled sets through Jev once, record the raw answers, and report the gap.
#
#   export JEV_API_KEY=sk-...
#   scripts/calibrate.sh [output-dir]
#
# The recording is the valuable part. Calling Jev costs tokens and rate-limit budget, and the
# labelled set is scarcer than either, so run this once and then tune offline with scripts/sweep.py.
#
# Needs outbound HTTPS to your Jev endpoint. Managed environments often deny it, in which case
# every verdict comes back with "degraded": true and says nothing about the content.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out="${1:-$root/calibration}"
check="$root/skill/guardrail-chatbot-jev/scripts/check.py"

if [[ -z "${JEV_API_KEY:-}" ]]; then
  echo "JEV_API_KEY is not set." >&2
  exit 2
fi

base="${JEV_BASE_URL:-https://api.typesafe.ai}"

echo "Checking that ${base} is reachable..." >&2
if ! curl -sS -m 15 -o /dev/null "${base%/}/v1/models" \
     -H "Authorization: Bearer $JEV_API_KEY"; then
  echo "Cannot reach ${base} from here. Run this from a machine with direct internet." >&2
  exit 3
fi

mkdir -p "$out"
status=0
for pair in "input:cases-input" "output:cases-output" "conversation:conversations"; do
  surface="${pair%%:*}"
  name="${pair##*:}"
  echo >&2
  echo "== $surface ==" >&2
  python3 "$check" --surface "$surface" \
    --jsonl "$root/examples/$name.jsonl" \
    --out "$out/$name.results.jsonl" \
    --record "$out/$name.answers.jsonl" \
    --expect expected_action --summary || status=$?
done

cat >&2 <<NEXT

Results in $out/

Raw answers are recorded, so tuning from here costs nothing:

  scripts/sweep.py report     $out/*.answers.jsonl
  scripts/sweep.py separation $out/cases-input.answers.jsonl
  scripts/sweep.py sweep      $out/cases-input.answers.jsonl --axis prv.input.review \\
      --from 0.2 --to 0.7 --step 0.05

Start with separation. A threshold only helps where the two groups do not overlap.
NEXT
exit "$status"
