"""Transports that put a question set in front of Jev and bring answers back.

The default transport is stdlib-only, so the guardrail has no third-party dependency and can run
anywhere Python does. If your Jev provider ships its own SDK client, hand it to ``SdkTransport``
instead; anything with a ``system_one`` method will do.
"""

from __future__ import annotations

import http.client
import json
import os
import random
import time
import urllib.error
import urllib.request
from typing import Any, Mapping, Protocol, runtime_checkable

from .types import GuardrailError, Usage

#: Jev is served from one endpoint. This is a default, not a constraint: set JEV_BASE_URL or
#: pass base_url= to point at a gateway or a proxy in front of it.
DEFAULT_BASE_URL = "https://api.typesafe.ai"
DEFAULT_MODEL = "jev-latest"
ENDPOINT = "/v1/systemone"
RETRY_STATUSES = frozenset({408, 429, 500, 502, 503, 504, 529})


class Answers(dict):
    """Answers keyed by question name, each a plain dict in Jev's wire shape."""


@runtime_checkable
class Transport(Protocol):
    """Anything that can answer a question set."""

    def system_one(
        self, state: Any, questions: Mapping[str, Any], *, model: str | None = None, timeout: float | None = None
    ) -> tuple[Answers, str, Usage]: ...


class HttpTransport:
    """Zero-dependency transport over ``POST /v1/systemone``."""

    def __init__(
        self,
        *,
        api_key: str | None = None,
        base_url: str | None = None,
        model: str | None = None,
        timeout: float = 10.0,
        max_retries: int = 2,
        backoff_initial: float = 0.5,
        backoff_max: float = 5.0,
    ) -> None:
        self.api_key = (api_key or os.environ.get("JEV_API_KEY", "")).strip()
        if not self.api_key:
            raise GuardrailError(
                "No Jev API key. Set JEV_API_KEY, or pass api_key=..., "
                "or use a recorded transport for offline work."
            )
        self.base_url = (
            base_url or os.environ.get("JEV_BASE_URL") or DEFAULT_BASE_URL
        ).strip().rstrip("/")
        self.model = model or os.environ.get("JEV_MODEL") or DEFAULT_MODEL
        self.timeout = timeout
        self.max_retries = max_retries
        self.backoff_initial = backoff_initial
        self.backoff_max = backoff_max

    def system_one(
        self, state: Any, questions: Mapping[str, Any], *, model: str | None = None, timeout: float | None = None
    ) -> tuple[Answers, str, Usage]:
        body = json.dumps(
            {"model": model or self.model, "state": state, "questions": dict(questions)}
        ).encode("utf-8")
        request = urllib.request.Request(
            self.base_url + ENDPOINT,
            data=body,
            method="POST",
            headers={
                "Authorization": f"Bearer {self.api_key}",
                "Content-Type": "application/json",
                "User-Agent": "guardrail-chatbot-jev/1.0 (+python)",
            },
        )

        delay = self.backoff_initial
        last: Exception | None = None
        for attempt in range(self.max_retries + 1):
            try:
                with urllib.request.urlopen(request, timeout=timeout or self.timeout) as response:
                    payload = json.loads(response.read().decode("utf-8"))
                return _unpack(payload)
            except urllib.error.HTTPError as exc:
                last = GuardrailError(f"Jev API returned {exc.code}: {exc.read()[:400]!r}")
                if exc.code not in RETRY_STATUSES or attempt == self.max_retries:
                    raise last from exc
                retry_after = exc.headers.get("retry-after") if exc.headers else None
                delay = min(float(retry_after), self.backoff_max) if _is_number(retry_after) else delay
            except (
                urllib.error.URLError,
                http.client.HTTPException,
                ConnectionError,
                TimeoutError,
                json.JSONDecodeError,
            ) as exc:
                last = GuardrailError(f"Jev API unreachable: {exc}")
                if attempt == self.max_retries:
                    raise last from exc
            time.sleep(delay * (1 - random.random() * 0.25))
            delay = min(delay * 2, self.backoff_max)
        raise last or GuardrailError("Jev API call failed")  # pragma: no cover


class SdkTransport:
    """Transport backed by a provider SDK client you supply.

    The client only has to expose ``system_one(state=..., questions=...)`` and return an object
    with ``answers``, ``model`` and ``usage``. Keeping the client on the outside is what lets this
    package stay dependency-free and stay out of the business of whose SDK you use.
    """

    def __init__(self, client: Any, *, model: str | None = None) -> None:
        if client is None:
            raise GuardrailError(
                "SdkTransport needs a client. Pass your provider's SDK client, or use "
                "HttpTransport, which needs no dependency at all."
            )
        self.client = client
        self.model = model

    def system_one(
        self, state: Any, questions: Mapping[str, Any], *, model: str | None = None, timeout: float | None = None
    ) -> tuple[Answers, str, Usage]:
        extra: dict[str, Any] = {}
        if model or self.model:
            extra["model"] = model or self.model
        if timeout is not None:
            extra["timeout"] = timeout
        try:
            response = self.client.system_one(state=state, questions=dict(questions), **extra)
        except Exception as exc:  # noqa: BLE001 - normalized for the caller
            raise GuardrailError(f"Jev SDK call failed: {exc}") from exc
        answers = Answers(
            {name: _dump(answer) for name, answer in getattr(response, "answers", {}).items()}
        )
        usage = getattr(response, "usage", None)
        return (
            answers,
            getattr(response, "model", ""),
            Usage(
                input_tokens=int(getattr(usage, "input_tokens", 0) or 0),
                output_tokens=int(getattr(usage, "output_tokens", 0) or 0),
            ),
        )


class RecordedTransport:
    """Replays answers recorded earlier. For tests, offline work and policy tuning."""

    def __init__(self, answers: Mapping[str, Mapping[str, Any]], *, model: str = "recorded") -> None:
        self.answers = Answers(dict(answers))
        self.model = model
        self.calls: list[tuple[Any, dict[str, Any]]] = []

    def system_one(
        self, state: Any, questions: Mapping[str, Any], *, model: str | None = None, timeout: float | None = None
    ) -> tuple[Answers, str, Usage]:
        self.calls.append((state, dict(questions)))
        return self.answers, self.model, Usage()


class RecordingTransport:
    """Wraps another transport and keeps every exchange.

    A calibration run is expensive: it costs tokens, it costs rate-limit budget, and the labelled
    set it runs against is the scarce thing. Recording the raw answers makes that one run
    replayable, so every later threshold change is an offline question instead of another call.
    """

    def __init__(self, inner: Transport, sink: list[dict[str, Any]] | None = None) -> None:
        self.inner = inner
        self.records: list[dict[str, Any]] = sink if sink is not None else []

    def system_one(
        self, state: Any, questions: Mapping[str, Any], *, model: str | None = None, timeout: float | None = None
    ) -> tuple[Answers, str, Usage]:
        answers, used_model, usage = self.inner.system_one(
            state, questions, model=model, timeout=timeout
        )
        self.records.append(
            {
                "state": state,
                "answers": dict(answers),
                "model": used_model,
                "usage": usage.as_dict(),
            }
        )
        return answers, used_model, usage


def auto_transport(client: Any | None = None, **kwargs: Any) -> Transport:
    """Wrap ``client`` if one is given, otherwise use the dependency-free stdlib transport."""
    if client is not None:
        return SdkTransport(client, model=kwargs.get("model"))
    return HttpTransport(**kwargs)


def _unpack(payload: Mapping[str, Any]) -> tuple[Answers, str, Usage]:
    usage = payload.get("usage") or {}
    return (
        Answers(dict(payload.get("answers") or {})),
        str(payload.get("model", "")),
        Usage(
            input_tokens=int(usage.get("input_tokens") or 0),
            output_tokens=int(usage.get("output_tokens") or 0),
        ),
    )


def _dump(answer: Any) -> dict[str, Any]:
    if isinstance(answer, Mapping):
        return dict(answer)
    dump = getattr(answer, "model_dump", None)
    if callable(dump):
        return dict(dump(mode="json"))
    return {k: v for k, v in vars(answer).items() if not k.startswith("_")}  # pragma: no cover


def _is_number(value: Any) -> bool:
    try:
        float(value)
    except (TypeError, ValueError):
        return False
    return True
