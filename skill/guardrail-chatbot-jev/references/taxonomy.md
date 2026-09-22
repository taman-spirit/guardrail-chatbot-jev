# Hazard taxonomy

Eighteen categories in `policies/standard-v1.json`, drawn from three public standards rather than
invented here, so a verdict can be mapped back to something an auditor recognises.

## Sources

| Standard | What it contributes |
| --- | --- |
| MLCommons AILuminate v1.1 | The twelve content hazards and their codes (`vcr`, `ncr`, `src`, `cse`, `ssh`, `iwp`, `ipv`, `dfm`, `hte`, `prv`, `spc`, `sxc`) |
| Meta Llama Guard 3/4 | The parallel `S1`-`S14` codes most moderation tooling already speaks |
| OWASP Top 10 for LLM Applications 2025 | The security hazards a content taxonomy misses: `pij`, `sid`, `exa`, `mis` |
| NIST AI RMF / AI 600-1, EU AI Act Art. 50 | Framing for the governance obligations these map onto |

## Content hazards

| id | Category | Maps to | Default severity |
| --- | --- | --- | --- |
| `cse` | Child sexual exploitation | AILuminate cse, LG S4 | critical, never resolves below `block` |
| `iwp` | Indiscriminate weapons (CBRNE) | AILuminate iwp, LG S9 | critical |
| `ssh` | Suicide and self-harm | AILuminate ssh, LG S11 | critical, routes to `crisis_support` |
| `vcr` | Violent crimes | AILuminate vcr, LG S1 | severe |
| `src` | Sex-related crimes | AILuminate src, LG S3 | severe |
| `hte` | Hate and discrimination | AILuminate hte, LG S10 | severe |
| `ncr` | Non-violent crimes | AILuminate ncr, LG S2 | severe |
| `prv` | Privacy and personal data | AILuminate prv, LG S7, GDPR Art. 4(1) | severe, routes to `redact` |
| `elc` | Elections and civic integrity | AILuminate spc_ele, LG S13 | severe |
| `sxc` | Adult sexual content | AILuminate sxc_prn, LG S12 | moderate |
| `ipv` | Intellectual property | AILuminate ipv, LG S8 | moderate, output only |
| `dfm` | Defamation | AILuminate dfm, LG S5 | moderate, output only |
| `spc` | Specialized advice | AILuminate spc_hlt/spc_fin, LG S6 | moderate, routes to `guide` |

## Security hazards

| id | Category | Maps to | Notes |
| --- | --- | --- | --- |
| `pij` | Prompt injection and jailbreak | OWASP LLM01:2025 | Input and conversation; also raised by the evasion and crescendo rules |
| `sid` | Sensitive information disclosure | OWASP LLM02, LLM07 | Output only, and a refusal does not excuse it |
| `exa` | Excessive agency | OWASP LLM06 | Output only |
| `mis` | Misinformation and unsupported claims | OWASP LLM09 | Output only; also raised when groundedness is low |

`scp` (out of scope) ships disabled. Enable it and write the deployment's purpose into its
description when a bot is meant to stay on one subject.

## Signals

Signals are not hazards. They are the context that decides how seriously a hazard is taken.

| Signal | Type | Surfaces | What it changes |
| --- | --- | --- | --- |
| `severity` | score 0-4 | all | Reported on the verdict |
| `intent` | choice | input, conversation | `academic` softens, `evasion` and `seeking_capability` harden |
| `actionability` | score 0-3 | input, output | Talk about a hazard is softened; a usable recipe is hardened |
| `refusal` | noul | output | Caps a refusing reply at `flag`, except for `sid`, `prv` and `ipv` |
| `groundedness` | score 0-3 | output, with context | Low groundedness raises `mis` |
| `escalation` | score 0-4 | conversation | High escalation floors the verdict at `review` |
| `crescendo` | noul | conversation | Multi-turn jailbreak: hardens and raises `pij` |
| `persona_drift` | noul | conversation | The assistant has left its assigned role |

## Why sentinels exist

A `choice` question distributes probability across every label. With 18 labels, a real violation
often lands at 0.3, not 0.9, and a threshold set for a two-way question would miss it.

Two fixes, both in the pack. Thresholds are calibrated for a wide choice, which is why they look
low. And the categories where a miss is unacceptable, `cse`, `iwp`, `ssh`, `prv`, `pij`, `sid`,
carry a `sentinel`: an independent yes/no question asked in the same request, whose probability is
taken whenever it is higher than the choice's. Questions in one request run in parallel and output
tokens are free, so a sentinel costs a few input tokens and no extra latency.
