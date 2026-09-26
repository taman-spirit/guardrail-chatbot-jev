"""Loading and querying a policy pack."""

from __future__ import annotations

import json
from dataclasses import dataclass
from importlib import resources
from pathlib import Path
from typing import Any, Mapping

from .types import Surface

_BUNDLED = "standard-v1"


@dataclass(frozen=True, slots=True)
class Category:
    id: str
    name: str
    description: str
    refs: tuple[str, ...]
    surfaces: frozenset[str]
    weight: float
    base_severity: int
    sentinel: bool
    sentinel_instructions: str | None
    thresholds: Mapping[str, Mapping[str, float]]
    route: str | None
    never_below: str | None
    enabled: bool

    def threshold(self, surface: str) -> Mapping[str, float]:
        """Thresholds for a surface, falling back to ``default``."""
        return self.thresholds.get(surface) or self.thresholds.get("default") or {}

    def applies(self, surface: str) -> bool:
        return self.enabled and surface in self.surfaces


@dataclass(frozen=True, slots=True)
class SentinelCorroboration:
    """How a pack asks for a sentinel to be backed by the hazard choice before it can drive the
    strongest actions on its own. See ``defaults.sentinel_corroboration``."""

    min_choice: float
    refusal: float
    refusal_max_sentinel: float
    refusal_except: frozenset[str]
    weak_at_most_flag: bool
    redact_instead_of_block_below: float


@dataclass(frozen=True, slots=True)
class ConfidenceGateOptions:
    """When a low-confidence answer escalates to review. See ``defaults.confidence_gate``."""

    needs_corroboration: bool
    skip_when_intent: frozenset[str]
    skip_min_confidence: float


class Policy:
    """A parsed policy pack.

    A policy pack is a JSON document: the hazard taxonomy, the thresholds that turn a
    probability into an action, and the rules that adjust it. The descriptions in the pack are
    also the text sent to Jev as question criteria, so editing the pack changes both what the
    model is asked and how the answer is judged.
    """

    def __init__(self, data: Mapping[str, Any]) -> None:
        self.data = dict(data)
        self.id: str = str(data.get("id", "custom"))
        self.version: str = str(data.get("version", "0"))
        self.name: str = str(data.get("name", self.id))
        self.content_note: str = str(data.get("content_note", ""))
        self.defaults: Mapping[str, Any] = dict(data.get("defaults") or {})
        self.signals: Mapping[str, Any] = dict(data.get("signals") or {})
        self.rules: tuple[Mapping[str, Any], ...] = tuple(data.get("rules") or ())
        self.routes: Mapping[str, str] = dict(data.get("routes") or {})
        self.scales: dict[str, Any] = {
            key: value for key, value in data.items() if key.endswith("_scale")
        }
        self.categories: dict[str, Category] = {
            cid: _category(cid, raw) for cid, raw in (data.get("categories") or {}).items()
        }
        _validate(self)

    # -- construction -------------------------------------------------

    @classmethod
    def load(cls, source: str | Path | Mapping[str, Any] | None = None) -> "Policy":
        """Load the bundled pack, a pack by name, a path to a JSON (or YAML) file, or a dict."""
        if source is None:
            return cls.bundled()
        if isinstance(source, Mapping):
            return cls(source)
        text_source = str(source)
        if not any(sep in text_source for sep in ("/", "\\")) and not text_source.endswith(
            (".json", ".yaml", ".yml")
        ):
            return cls.bundled(text_source)
        path = Path(text_source)
        raw = path.read_text(encoding="utf-8")
        if path.suffix in (".yaml", ".yml"):
            try:
                import yaml  # type: ignore[import-untyped]
            except ImportError as exc:  # pragma: no cover - optional path
                raise RuntimeError(
                    "YAML policy packs need PyYAML: pip install 'guardrail-chatbot-jev[yaml]'"
                ) from exc
            return cls(yaml.safe_load(raw))
        return cls(json.loads(raw))

    @classmethod
    def bundled(cls, name: str = _BUNDLED) -> "Policy":
        """Load a pack shipped inside the package."""
        text = resources.files(__package__).joinpath(f"policies/{name}.json").read_text("utf-8")
        return cls(json.loads(text))

    # -- queries ------------------------------------------------------

    def for_surface(self, surface: Surface) -> list[Category]:
        """Enabled categories that apply to a surface, most serious first."""
        cats = [c for c in self.categories.values() if c.applies(surface)]
        cats.sort(key=lambda c: (-c.weight, -c.base_severity, c.id))
        return cats

    def sentinels(self, surface: Surface) -> list[Category]:
        """Categories that get a dedicated yes/no question on this surface.

        A choice question picks one label. Content can carry more than one hazard at once, and
        the ones where a miss is unacceptable get their own independent question.
        """
        return [c for c in self.for_surface(surface) if c.sentinel and c.sentinel_instructions]

    def signals_for(self, surface: Surface, *, has_context: bool = False) -> dict[str, Any]:
        """Signal definitions that apply to a surface."""
        out: dict[str, Any] = {}
        for name, spec in self.signals.items():
            if surface not in (spec.get("surfaces") or []):
                continue
            if spec.get("requires_context") and not has_context:
                continue
            out[name] = spec
        return out

    def criteria_for(self, spec: Mapping[str, Any]) -> Any:
        """Resolve a signal's criteria, following ``criteria_ref`` into the pack's scales."""
        if "criteria" in spec:
            return spec["criteria"]
        ref = spec.get("criteria_ref")
        return self.scales.get(str(ref)) if ref else None

    def min_confidence(self) -> float:
        return float(self.defaults.get("min_confidence", 0.65))

    def on_low_confidence(self) -> str:
        return str(self.defaults.get("on_low_confidence", "escalate"))

    def fail_closed(self, surface: Surface | None = None) -> bool:
        """Whether an unreachable Jev blocks on this surface.

        Set per surface, because the two are not the same risk: the input check sits in front of a
        model that has its own safety, so failing open there degrades gracefully, while the output
        check is the last line and has nothing behind it.
        """
        setting = self.defaults.get("on_error", "fail_closed")
        if isinstance(setting, Mapping):
            setting = setting.get(surface or "", "fail_closed")
        return str(setting) == "fail_closed"

    def sentinel_corroboration(self) -> SentinelCorroboration | None:
        """The pack's ``defaults.sentinel_corroboration``, or ``None`` when it is off."""
        raw = self.defaults.get("sentinel_corroboration")
        if not isinstance(raw, Mapping):
            return None
        return SentinelCorroboration(
            min_choice=float(raw.get("min_choice", 0.02)),
            refusal=float(raw.get("refusal", 0.8)),
            refusal_max_sentinel=float(raw.get("refusal_max_sentinel", 0.5)),
            refusal_except=frozenset(raw.get("refusal_except") or ()),
            weak_at_most_flag=bool(raw.get("weak_at_most_flag", False)),
            redact_instead_of_block_below=float(raw.get("redact_instead_of_block_below", 0) or 0),
        )

    def confidence_gate(self) -> ConfidenceGateOptions:
        """The pack's ``defaults.confidence_gate`` options."""
        raw = self.defaults.get("confidence_gate")
        raw = raw if isinstance(raw, Mapping) else {}
        return ConfidenceGateOptions(
            needs_corroboration=bool(raw.get("needs_corroboration", False)),
            skip_when_intent=frozenset(raw.get("skip_when_intent") or ()),
            skip_min_confidence=float(raw.get("skip_min_confidence", 0.5)),
        )

    def error_action(self) -> str:
        return str(self.defaults.get("error_action", "review"))

    def __repr__(self) -> str:  # pragma: no cover
        return f"<Policy {self.id} v{self.version}: {len(self.categories)} categories>"


def _category(cid: str, raw: Mapping[str, Any]) -> Category:
    return Category(
        id=cid,
        name=str(raw.get("name", cid)),
        description=str(raw.get("description", "")),
        refs=tuple(raw.get("refs") or ()),
        surfaces=frozenset(raw.get("surfaces") or ("input", "output", "conversation")),
        weight=float(raw.get("weight", 0.5)),
        base_severity=int(raw.get("base_severity", 2)),
        sentinel=bool(raw.get("sentinel", False)),
        sentinel_instructions=raw.get("sentinel_instructions"),
        thresholds={k: {kk: float(vv) for kk, vv in v.items()} for k, v in (raw.get("thresholds") or {}).items()},
        route=raw.get("route"),
        never_below=raw.get("never_below"),
        enabled=bool(raw.get("enabled", True)),
    )


def _validate(policy: "Policy") -> None:
    if not policy.categories:
        raise ValueError("policy pack has no categories")
    for cid, cat in policy.categories.items():
        if not cat.threshold("input") and not cat.thresholds.get("default"):
            raise ValueError(f"category {cid!r} has no default thresholds")
        for surface, bands in cat.thresholds.items():
            missing = {"block", "review", "flag"} - set(bands)
            if missing:
                raise ValueError(f"category {cid!r} thresholds[{surface}] missing {sorted(missing)}")
            if not bands["block"] >= bands["review"] >= bands["flag"]:
                raise ValueError(f"category {cid!r} thresholds[{surface}] are not ordered block >= review >= flag")
    for rule in policy.rules:
        signal = rule.get("when", {}).get("signal")
        if signal and signal not in policy.signals:
            raise ValueError(f"rule {rule.get('id')!r} refers to unknown signal {signal!r}")
        for cid in rule.get("then", {}).get("add_finding", "") and [rule["then"]["add_finding"]] or []:
            if cid not in policy.categories:
                raise ValueError(f"rule {rule.get('id')!r} adds unknown category {cid!r}")
