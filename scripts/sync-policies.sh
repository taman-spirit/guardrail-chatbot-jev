#!/usr/bin/env bash
# The policy pack lives in policies/. Both packages ship a copy; this keeps them identical.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mkdir -p "$root/python/src/guardrail_chatbot_jev/policies" "$root/ts/src/policies"
for pack in "$root"/policies/*.json; do
  cp "$pack" "$root/python/src/guardrail_chatbot_jev/policies/"
  cp "$pack" "$root/ts/src/policies/"
done
echo "synced $(ls "$root"/policies/*.json | wc -l) policy pack(s)"
