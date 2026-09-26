"""Content guardrails for AI chatbots, decided by the Jev decision model.

    from guardrail_chatbot_jev import Guard

    guard = Guard()
    verdict = guard.check_input(user_message)
    if not verdict.allowed:
        return safe_response(verdict)
"""

from .cache import VOLATILE_STATE_KEYS, LRUCache, VerdictCache, cache_key
from .client import (
    Answers,
    HttpTransport,
    RecordedTransport,
    RecordingTransport,
    SdkTransport,
    Transport,
    auto_transport,
)
from .decide import decide, error_verdict, with_floor
from .guard import ContextCheck, Guard
from .policy import Category, Policy
from .prefilter import COMMON_PATTERNS, Pattern, PatternPrefilter, Prefilter, prefilter_verdict
from .questions import build_questions, conversation_state, input_state, output_state
from .session import WITHHELD_PLACEHOLDER, Session
from .streaming import StreamEvent, guard_stream
from .tuning import Record, Report
from .types import (
    LADDER,
    Action,
    Finding,
    GuardrailError,
    Surface,
    Turn,
    Usage,
    Verdict,
)

__version__ = "1.0.1"

__all__ = [
    "COMMON_PATTERNS",
    "LADDER",
    "Action",
    "Answers",
    "Category",
    "ContextCheck",
    "Finding",
    "Guard",
    "GuardrailError",
    "HttpTransport",
    "LRUCache",
    "Pattern",
    "PatternPrefilter",
    "Policy",
    "Prefilter",
    "Record",
    "RecordedTransport",
    "RecordingTransport",
    "Report",
    "SdkTransport",
    "Session",
    "StreamEvent",
    "Surface",
    "Transport",
    "Turn",
    "Usage",
    "Verdict",
    "VOLATILE_STATE_KEYS",
    "WITHHELD_PLACEHOLDER",
    "VerdictCache",
    "auto_transport",
    "build_questions",
    "cache_key",
    "conversation_state",
    "decide",
    "error_verdict",
    "guard_stream",
    "input_state",
    "output_state",
    "prefilter_verdict",
    "with_floor",
    "__version__",
]
