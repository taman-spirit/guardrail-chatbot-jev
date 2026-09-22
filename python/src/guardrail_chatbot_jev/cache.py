"""Caching verdicts, so identical content does not pay for a second round trip.

Repeat messages are common in a deployed chatbot, and a verdict is a pure function of the policy,
the surface and the content. The key carries the policy id, so publishing a new pack invalidates
every entry without anyone having to remember to flush.
"""

from __future__ import annotations

import hashlib
import json
import threading
import time
from collections import OrderedDict
from dataclasses import replace
from typing import Any, Mapping, Protocol, runtime_checkable

from .types import Surface, Verdict


#: Dropped from the cache key by default. A session puts the turn number and a running risk
#: score in here, and both change on every turn, so keying on them would mean the cache never
#: hits for exactly the repeated messages it exists to serve.
VOLATILE_STATE_KEYS: frozenset[str] = frozenset({"deployment_context"})


def cache_key(
    policy_id: str,
    surface: Surface,
    state: Any,
    subset: str = "full",
    *,
    ignore: frozenset[str] = VOLATILE_STATE_KEYS,
) -> str:
    """A stable key for one check.

    Deployment context is dropped by default, because it is per-request context rather than the
    content being judged. If a deployment's metadata genuinely changes what a verdict should be,
    pass ``ignore=frozenset()`` and accept the lower hit rate.
    """
    if ignore and isinstance(state, Mapping):
        state = {k: v for k, v in state.items() if k not in ignore}
    canonical = json.dumps(state, sort_keys=True, ensure_ascii=False, separators=(",", ":"))
    material = "\0".join((policy_id, surface, subset, canonical))
    return hashlib.sha256(material.encode("utf-8")).hexdigest()


@runtime_checkable
class VerdictCache(Protocol):
    """Anything that can remember a verdict. Bring your own Redis by implementing this."""

    def get(self, key: str) -> Verdict | None: ...

    def put(self, key: str, verdict: Verdict) -> None: ...


class LRUCache:
    """In-process LRU with a TTL, safe to share across threads.

    Degraded verdicts are never stored. A verdict produced because Jev was unreachable says
    nothing about the content, and caching one would turn a brief outage into a lasting wrong
    answer for that exact message.
    """

    def __init__(self, capacity: int = 4096, ttl: float = 300.0) -> None:
        if capacity < 1:
            raise ValueError("capacity must be at least 1")
        self.capacity = capacity
        self.ttl = ttl
        self._entries: OrderedDict[str, tuple[float, Verdict]] = OrderedDict()
        self._lock = threading.Lock()
        self.hits = 0
        self.misses = 0

    def get(self, key: str) -> Verdict | None:
        now = time.monotonic()
        with self._lock:
            entry = self._entries.get(key)
            if entry is None:
                self.misses += 1
                return None
            stored_at, verdict = entry
            if self.ttl and now - stored_at > self.ttl:
                del self._entries[key]
                self.misses += 1
                return None
            self._entries.move_to_end(key)
            self.hits += 1
        return replace(verdict, cached=True, latency_ms=0.0)

    def put(self, key: str, verdict: Verdict) -> None:
        if verdict.degraded:
            return
        with self._lock:
            self._entries[key] = (time.monotonic(), verdict)
            self._entries.move_to_end(key)
            while len(self._entries) > self.capacity:
                self._entries.popitem(last=False)

    def clear(self) -> None:
        with self._lock:
            self._entries.clear()

    @property
    def stats(self) -> Mapping[str, Any]:
        total = self.hits + self.misses
        return {
            "size": len(self._entries),
            "capacity": self.capacity,
            "hits": self.hits,
            "misses": self.misses,
            "hit_rate": round(self.hits / total, 4) if total else 0.0,
        }

    def __len__(self) -> int:
        return len(self._entries)
