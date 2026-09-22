"""Where sessions live between requests.

A `Session` carries risk between turns, which only works if it outlives the request that made it.
Where you put it decides two things the guardrail cannot decide for you:

  - **Memory.** A plain dict grows for as long as the process runs. Conversations are created by
    anyone who can reach the endpoint, so an unbounded one is a memory leak with a public trigger.
  - **Workers.** `uvicorn --workers 4` is four processes. With per-process state, four turns of one
    conversation land in four different dicts, each one convinced the conversation just began.
    Nothing errors. The floor simply stops carrying, which is the failure you would least like to
    be silent.

So there are two implementations behind one interface. `MemorySessionStore` is bounded and good
for a single process. `RedisSessionStore` is what you want the moment there is more than one.

Both also hand out the lock for a conversation, because the lock has the same problem: an
`asyncio.Lock` only serializes turns inside one process.
"""

from __future__ import annotations

import asyncio
import json
import time
import uuid
from collections import OrderedDict
from contextlib import asynccontextmanager
from typing import Any, AsyncContextManager, AsyncIterator, Optional, Protocol, Tuple

from guardrail_chatbot_jev import Session


class SessionStore(Protocol):
    """Load, save and lock a conversation's session."""

    def transaction(self, conversation_id: str) -> AsyncContextManager[Session]:
        """Hold the conversation, yield its session, and save whatever it became.

        A load/modify/save cycle that is not inside the lock is a lost update: two turns read the
        same risk and the second one writes over the first.
        """
        ...

    async def stats(self) -> dict[str, Any]: ...


class MemorySessionStore:
    """Bounded in-process store: an LRU with a TTL.

    Good for one process. With more than one it silently stops carrying risk between turns,
    because each process has its own dict. Use `RedisSessionStore` there.
    """

    def __init__(self, *, max_conversations: int = 10_000, ttl: float = 3600.0) -> None:
        self.max_conversations = max_conversations
        self.ttl = ttl
        self._entries: "OrderedDict[str, Tuple[float, Session]]" = OrderedDict()
        self._locks: "OrderedDict[str, asyncio.Lock]" = OrderedDict()
        self.evicted = 0
        self.expired = 0

    @asynccontextmanager
    async def transaction(self, conversation_id: str) -> AsyncIterator[Session]:
        async with self._lock_for(conversation_id):
            session = self._load(conversation_id)
            yield session
            # The Session was mutated in place, so there is nothing to write back. Re-stamping it
            # is what makes the TTL measure idleness rather than age, so a conversation that is
            # still going does not expire underneath itself.
            self._entries[conversation_id] = (time.monotonic(), session)
            self._entries.move_to_end(conversation_id)

    async def stats(self) -> dict[str, Any]:
        return {
            "backend": "memory",
            "conversations": len(self._entries),
            "capacity": self.max_conversations,
            "evicted": self.evicted,
            "expired": self.expired,
        }

    def _load(self, conversation_id: str) -> Session:
        self._sweep()
        entry = self._entries.get(conversation_id)
        if entry is not None:
            return entry[1]
        session = Session(id=conversation_id)
        self._entries[conversation_id] = (time.monotonic(), session)
        while len(self._entries) > self.max_conversations:
            self._entries.popitem(last=False)
            self.evicted += 1
        return session

    def _sweep(self) -> None:
        """Drop what has gone stale. Cheap because the oldest entries are at the front."""
        cutoff = time.monotonic() - self.ttl
        while self._entries:
            key, (created, _) = next(iter(self._entries.items()))
            if created > cutoff:
                break
            self._entries.pop(key, None)
            # A lock someone is holding stays: dropping it would let the next turn build a second
            # lock for the same conversation and run alongside the first.
            if not (key in self._locks and self._locks[key].locked()):
                self._locks.pop(key, None)
            self.expired += 1

    def _lock_for(self, conversation_id: str) -> asyncio.Lock:
        lock = self._locks.get(conversation_id)
        if lock is None:
            lock = self._locks[conversation_id] = asyncio.Lock()
        self._locks.move_to_end(conversation_id)
        # The lock table is bounded separately: a lock nobody is holding is safe to forget, and
        # forgetting it only means the next turn makes a new one.
        while len(self._locks) > self.max_conversations:
            _, dropped = self._locks.popitem(last=False)
            if dropped.locked():  # pragma: no cover - only under eviction pressure
                self._locks[conversation_id] = lock
                break
        return lock


class RedisSessionStore:
    """Shared store, so risk carries across workers and across restarts.

    Takes any client with the async redis-py surface (`get`, `set`, `delete`, `eval`), so this
    file has no dependency of its own and you can hand it a fake in a test.
    """

    #: Release only if we still hold the lock. Without the compare, a handler that overran its
    #: timeout would delete a lock another worker had already taken.
    _RELEASE = """
    if redis.call('get', KEYS[1]) == ARGV[1] then
      return redis.call('del', KEYS[1])
    end
    return 0
    """

    def __init__(
        self,
        client: Any,
        *,
        prefix: str = "guardrail:session:",
        ttl: int = 3600,
        lock_timeout: float = 15.0,
        lock_poll: float = 0.05,
    ) -> None:
        self.client = client
        self.prefix = prefix
        self.ttl = ttl
        self.lock_timeout = lock_timeout
        self.lock_poll = lock_poll

    @asynccontextmanager
    async def transaction(self, conversation_id: str) -> AsyncIterator[Session]:
        token = await self._acquire(conversation_id)
        try:
            session = await self._load(conversation_id)
            yield session
            await self._save(session)
        finally:
            await self.client.eval(self._RELEASE, 1, self._lock_key(conversation_id), token)

    async def stats(self) -> dict[str, Any]:
        return {"backend": "redis", "prefix": self.prefix, "ttl": self.ttl}

    def _key(self, conversation_id: str) -> str:
        return f"{self.prefix}{conversation_id}"

    def _lock_key(self, conversation_id: str) -> str:
        return f"{self.prefix}lock:{conversation_id}"

    async def _acquire(self, conversation_id: str) -> str:
        token = uuid.uuid4().hex
        key = self._lock_key(conversation_id)
        deadline = time.monotonic() + self.lock_timeout
        while True:
            # The lock carries its own expiry, so a worker that dies holding it does not wedge the
            # conversation forever.
            if await self.client.set(key, token, nx=True, px=int(self.lock_timeout * 1000)):
                return token
            if time.monotonic() >= deadline:
                # Better a turn judged without the lock than a request that never answers. The
                # cost is a possible lost update on one turn, not a wrong verdict on this one.
                return token
            await asyncio.sleep(self.lock_poll)

    async def _load(self, conversation_id: str) -> Session:
        raw = await self.client.get(self._key(conversation_id))
        if raw:
            try:
                return Session.from_state(json.loads(raw))
            except (ValueError, TypeError):
                # A record we cannot read is worse than no record: it would carry a floor we
                # cannot vouch for. Start the conversation over and let the checks rebuild it.
                await self.client.delete(self._key(conversation_id))
        return Session(id=conversation_id)

    async def _save(self, session: Session) -> None:
        await self.client.set(
            self._key(session.id), json.dumps(session.as_state()), ex=self.ttl
        )


def build_store(redis_url: Optional[str] = None) -> SessionStore:
    """Redis when there is a URL for it, bounded memory otherwise.

    The Redis path needs `pip install redis`. It is imported here rather than at the top so the
    memory path, which is what the example runs by default, needs nothing installed.
    """
    if not redis_url:
        return MemorySessionStore()
    from redis.asyncio import Redis  # imported here so the memory path needs nothing installed

    return RedisSessionStore(Redis.from_url(redis_url, decode_responses=True))
