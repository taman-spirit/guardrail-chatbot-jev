"""Adding a compliance domain of your own on top of the shipped pack.

The shipped taxonomy is the part every deployment shares. What a regulated product needs on top of
it is specific: a clinic cares about dosing, a broker about performance promises, a lender about
adverse action notices. This file shows how to add that layer, and it runs the four checks that
catch the mistakes which are easy to make and hard to notice.

Healthcare is only the illustration. The mechanism is the same for any domain.

    cd python && pip install -e .
    python examples/domain_policy.py

Four things are worth knowing before you write one, all of them demonstrated below:

  1. A rule cannot set a route. `then` takes add_finding, upgrade, downgrade, cap_action and
     floor_action, and nothing else. Routes belong to categories, and only `redact`, `guide` and
     `crisis_support` do anything there, so a rule reaches a route by adding a finding for a
     category that owns it.
  2. `never_below` means "once this category fires, never resolve it below X". It is not a floor on
     the whole check: under the category's lowest threshold nothing fires at all and never_below
     has nothing to act on. The `flag` threshold is the switch that puts a category in play.
  3. The shipped softening rules apply to your new category too. A category that must not be
     softened has to be named in each rule's `except_categories`, which you can do by patching the
     rule by id rather than restating it.
  4. A malformed pack is refused when it loads, not on the first request.
"""

from __future__ import annotations

import copy
from typing import Any, Mapping

from guardrail_chatbot_jev import (
    Guard,
    Pattern,
    PatternPrefilter,
    Policy,
    RecordedTransport,
    decide,
)

# --- the overlay --------------------------------------------------------------

DOMAIN: dict[str, Any] = {
    "id": "healthcare-v1",
    "version": "1",
    "name": "Standard v1 plus clinical rules",
    "categories": {
        "dos": {
            "name": "Dosing or regimen instruction",
            "description": (
                "Telling the reader what dose, frequency, route or duration of a medicine to "
                "take, or how to change a dose they are already on, as an instruction rather than "
                "as a description of what a clinician decides."
            ),
            "refs": ["internal policy MED-1"],
            "surfaces": ["output", "conversation"],
            "weight": 0.8,
            "base_severity": 4,
            # A sentinel is asked as its own yes/no question alongside the main one, and is what
            # mid-stream checks ask. Use it for the categories where a miss is unacceptable.
            "sentinel": True,
            "sentinel_instructions": (
                "Does this reply tell the reader a specific dose, frequency, route or duration to "
                "take, or how to change one?"
            ),
            "thresholds": {"default": {"block": 0.7, "review": 0.3, "flag": 0.15}},
            "never_below": "review",
        },
        "phi": {
            "name": "Patient identifiers",
            "description": (
                "Reproducing information that identifies a patient: a name together with a "
                "condition, a medical record or insurance number, or a date of birth alongside a "
                "diagnosis."
            ),
            "refs": ["internal policy MED-3"],
            "surfaces": ["output", "conversation"],
            "weight": 0.7,
            "base_severity": 3,
            "sentinel": False,
            "thresholds": {"default": {"block": 0.75, "review": 0.35, "flag": 0.15}},
            "route": "redact",
        },
        "emg": {
            "name": "Emergency symptom present",
            "description": (
                "The conversation describes a symptom that needs emergency care now: chest pain, "
                "trouble breathing, one-sided weakness, a bleed in pregnancy."
            ),
            "refs": ["internal policy MED-4"],
            "surfaces": ["input", "output", "conversation"],
            "weight": 0.9,
            "base_severity": 4,
            "sentinel": False,
            "thresholds": {"default": {"block": 0.95, "review": 0.4, "flag": 0.2}},
            # This is how the rule below reaches the crisis path: the route lives here.
            "route": "crisis_support",
        },
    },
    "signals": {
        "red_flag": {
            "type": "noul",
            "surfaces": ["input", "output", "conversation"],
            "instructions": (
                "Does the conversation describe a symptom that needs emergency care now, such as "
                "chest pain, trouble breathing, one-sided weakness, or a bleed in pregnancy?"
            ),
        }
    },
    "rules": [
        # Patch shipped rules by id to widen their exceptions. A reply that refuses, or that stays
        # non-operational, can still carry a dose or a patient identifier.
        {
            "id": "refusal-caps-output-risk",
            "except_categories": ["sid", "prv", "ipv", "dos", "phi"],
        },
        {
            "id": "low-actionability-softens",
            "except_categories": ["cse", "ssh", "sid", "prv", "dos"],
        },
        {
            "id": "red-flag-goes-to-emergency",
            "when": {"signal": "red_flag", "op": ">=", "value": 0.5},
            "then": {"floor_action": "review", "add_finding": "emg"},
            "why": "An emergency symptom must reach a human path, whatever else the reply does.",
        },
    ],
}


def overlay(base: Policy, patch: Mapping[str, Any]) -> Policy:
    """Merge a patch onto a pack, so you inherit upstream changes instead of forking.

    Categories and signals merge by key, so the shipped taxonomy survives. Rules append, and a
    patch sharing an id updates the shipped rule in place, which is how you widen an existing
    rule's exceptions without copying the rest of it.
    """
    data = copy.deepcopy(dict(base.data))
    for block in ("categories", "signals"):
        merged = dict(data.get(block) or {})
        for key, value in (patch.get(block) or {}).items():
            merged[key] = {**merged.get(key, {}), **value} if isinstance(value, Mapping) else value
        data[block] = merged

    by_id = {rule.get("id"): dict(rule) for rule in data.get("rules") or ()}
    order = list(by_id)
    for rule in patch.get("rules") or ():
        rid = rule.get("id")
        if rid in by_id:
            by_id[rid].update(rule)
        else:
            by_id[rid] = dict(rule)
            order.append(rid)
    data["rules"] = [by_id[rid] for rid in order]

    for key, value in patch.items():
        if key not in ("categories", "signals", "rules"):
            data[key] = value
    return Policy(data)  # validates, so a bad overlay fails here


# --- answer helpers, the same shapes the test suite uses ----------------------


def hazard(probabilities: Mapping[str, float], confidence: float = 0.9) -> dict[str, Any]:
    filled = dict(probabilities)
    filled.setdefault("none", max(0.0, 1.0 - sum(probabilities.values())))
    return {
        "type": "choice",
        "choice": max(filled, key=lambda k: filled[k]),
        "confidence": confidence,
        "probabilities": filled,
    }


def noul(value: float) -> dict[str, Any]:
    return {"type": "noul", "noul": value}


def score(value: float) -> dict[str, Any]:
    return {"type": "score", "score": value, "confidence": 0.9, "probabilities": {}, "legend": {}}


def choice(value: str) -> dict[str, Any]:
    return {"type": "choice", "choice": value, "confidence": 0.9, "probabilities": {value: 0.9}}


def answers(**parts: Any) -> dict[str, Any]:
    base: dict[str, Any] = {
        "hazard": hazard({}),
        "severity": score(0.0),
        "actionability": score(0.0),
        "intent": choice("benign"),
    }
    base.update(parts)
    return base


def main() -> None:
    base = Policy.bundled()
    policy = overlay(base, DOMAIN)

    print("1. it loads and validates")
    print(f"   {policy.id} v{policy.version}: {len(policy.categories)} categories "
          f"({len(base.categories)} shipped plus {len(DOMAIN['categories'])})")
    print("   sentinels on output:", [c.id for c in policy.sentinels("output")])

    print("\n2. the new criteria are what Jev is asked")
    request = Guard(policy, transport=RecordedTransport({})).preview(
        "output", {"evaluating": "output", "assistant_reply": "..."}
    )
    labels = request["questions"]["hazard"]["criteria"]
    print("   new hazard labels:", sorted(k for k in labels if k in DOMAIN["categories"]))
    print("   red_flag asked separately:", "red_flag" in request["questions"])

    print("\n3. never_below holds a dose that crossed the flag threshold")
    for probability in (0.05, 0.16, 0.75):
        verdict = decide(policy, "output", answers(hazard=hazard({"dos": probability})))
        fired = [f.category for f in verdict.findings]
        print(f"   p={probability:<5} action={verdict.action:<6} route={verdict.route:<14} findings={fired}")
    # Below the flag threshold of 0.15 nothing fires, so there is nothing for never_below to lift.
    assert decide(policy, "output", answers(hazard=hazard({"dos": 0.05}))).action == "allow"
    assert decide(policy, "output", answers(hazard=hazard({"dos": 0.16}))).action == "review"

    print("\n4. the shipped softening rules no longer erase a dose")
    dosing = decide(policy, "output", answers(hazard=hazard({"dos": 0.45}), refusal=noul(0.9)))
    advice = decide(policy, "output", answers(hazard=hazard({"spc": 0.45}), refusal=noul(0.9)))
    print(f"   a refusal naming a dose:   {dosing.action}")
    print(f"   a refusal naming advice:   {advice.action}  (still capped, as shipped)")
    assert dosing.action in ("review", "block") and advice.action == "flag"

    print("\n5. a rule reaches a route by adding a finding that owns one")
    emergency = decide(policy, "conversation", answers(hazard=hazard({"spc": 0.1}), red_flag=noul(0.8)))
    print(f"   action={emergency.action} route={emergency.route} findings={[f.category for f in emergency.findings]}")
    print("   rules:", emergency.applied_rules)
    assert emergency.route == "crisis_support"

    print("\n6. the deterministic half costs no round trip")
    prefilter = PatternPrefilter([
        Pattern.of("mrn", r"\bMRN[-\s:]?\d{6,10}\b", "phi", "review", ("output", "conversation")),
    ])
    leak = Guard(policy, prefilter=prefilter, transport=RecordedTransport({})).check_output(
        "Patient record for J. Doe, MRN 4471902, shows..."
    )
    print(f"   action={leak.action} route={leak.route} settled_by={leak.prefilter}")
    assert leak.prefilter == "mrn" and leak.route == "redact"

    print("\n7. a malformed overlay is refused at load")
    broken = [
        ({"categories": {"dos": {"thresholds": {"default": {"block": 0.2, "review": 0.5, "flag": 0.1}}}}},
         "thresholds out of order"),
        ({"rules": [{"id": "x", "when": {"signal": "nope", "op": ">=", "value": 1}, "then": {"upgrade": 1}}]},
         "unknown signal"),
        ({"rules": [{"id": "y", "when": {"signal": "red_flag", "op": ">=", "value": 1},
                     "then": {"add_finding": "ghost"}}]},
         "unknown category"),
    ]
    for patch, why in broken:
        try:
            overlay(policy, patch)
        except ValueError as exc:
            print(f"   {why}: {exc}")
        else:  # pragma: no cover - would be a validation gap
            raise AssertionError(f"not caught: {why}")

    print("\nThresholds here are numbers someone chose, not numbers anyone measured. Add labelled "
          "cases, run scripts/sweep.py separation first, and only then sweep.")


if __name__ == "__main__":
    main()
