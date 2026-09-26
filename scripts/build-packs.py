#!/usr/bin/env python3
"""Build the derived policy packs from policies/overlays/.

    scripts/build-packs.py            # write policies/<id>.json for every overlay
    scripts/build-packs.py --check    # fail if a built pack is out of date

An overlay is layered on standard-v1 instead of forking it, so a change to the shipped taxonomy
reaches every derived pack on the next build. Categories and signals merge by key, a rule with a
shipped id updates that rule in place, and anything else at the top level replaces the base value.

The merge is one level deep; "defaults" merges by key. A category or signal patch replaces each key it names whole, so a
patch that sets "thresholds" replaces every surface's bands, not just the ones it lists; a rule
patch replaces "when" or "then" whole in the same way. Restate what you mean to keep.

One macro keeps rule exceptions readable: "$all_but:a,b" in except_categories expands to every
category in the built pack except a and b, which is how a rule is made to apply to only a few.

Run scripts/sync-policies.sh afterwards to copy the result into the packages.
"""

from __future__ import annotations

import copy
import json
import sys
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
BASE = ROOT / "policies" / "standard-v1.json"
OVERLAYS = ROOT / "policies" / "overlays"
MACRO = "$all_but:"


def merge(base: dict[str, Any], patch: dict[str, Any]) -> dict[str, Any]:
    data = copy.deepcopy(base)
    for block in ("categories", "signals"):
        merged = dict(data.get(block) or {})
        for key, value in (patch.get(block) or {}).items():
            merged[key] = {**merged.get(key, {}), **value}
        data[block] = merged

    by_id = {rule["id"]: dict(rule) for rule in data.get("rules") or ()}
    order = list(by_id)
    for rule in patch.get("rules") or ():
        if rule["id"] in by_id:
            by_id[rule["id"]].update(rule)
        else:
            by_id[rule["id"]] = dict(rule)
            order.append(rule["id"])
    data["rules"] = [by_id[rid] for rid in order]

    # defaults merge by key, so an overlay can set one setting without dropping the others.
    if isinstance(patch.get("defaults"), dict):
        data["defaults"] = {**(data.get("defaults") or {}), **copy.deepcopy(patch["defaults"])}

    for key, value in patch.items():
        if key not in ("categories", "signals", "rules", "defaults"):
            data[key] = copy.deepcopy(value)

    everything = list(data["categories"])
    for rule in data["rules"]:
        expanded: list[str] = []
        for item in rule.get("except_categories") or ():
            if item.startswith(MACRO):
                keep = set(item[len(MACRO):].split(","))
                unknown = keep - set(everything)
                if unknown:
                    raise ValueError(f"rule {rule['id']!r}: {MACRO} names unknown {sorted(unknown)}")
                expanded.extend(c for c in everything if c not in keep)
            else:
                expanded.append(item)
        if "except_categories" in rule:
            rule["except_categories"] = expanded
    return data


def render(data: dict[str, Any]) -> str:
    return json.dumps(data, ensure_ascii=False, indent=2) + "\n"


def main(argv: list[str]) -> int:
    check = "--check" in argv
    base = json.loads(BASE.read_text("utf-8"))
    stale = []
    for overlay in sorted(OVERLAYS.glob("*.json")):
        patch = json.loads(overlay.read_text("utf-8"))
        built = render(merge(base, patch))
        target = ROOT / "policies" / f"{patch['id']}.json"
        current = target.read_text("utf-8") if target.exists() else None
        if check:
            if current != built:
                stale.append(target.name)
        elif current != built:
            target.write_text(built, "utf-8")
            print(f"built {target.relative_to(ROOT)}")
    if stale:
        print(f"out of date: {', '.join(stale)}; run scripts/build-packs.py", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
