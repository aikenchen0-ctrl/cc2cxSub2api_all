"""Decide whether a failed queue job should be retried.

The consumer used to requeue every exception, including 401 / invalid key,
which burned LLM quota in a loop. This module only answers yes/no.
"""

from __future__ import annotations

_PERMANENT_MARKERS = (
    "unauthorized",
    "invalid api key",
    "incorrect api key",
    "authentication",
    "insufficient_balance",
    "access forbidden",
    "invalid_api_key",
    "incorrect_api_key",
)


def is_permanent_failure(error_message: str) -> bool:
    """True when retrying cannot recover (auth / billing / bad key)."""
    text = str(error_message or "").lower()
    if not text:
        return False
    if "401" in text or "error code: 401" in text or "http_code\":\"401" in text:
        return True
    return any(marker in text for marker in _PERMANENT_MARKERS)


def should_give_up(*, attempt_count: int, error_message: str, max_attempts: int) -> bool:
    """True when the consumer must mark the job failed instead of requeue."""
    if is_permanent_failure(error_message):
        return True
    limit = max(1, int(max_attempts or 1))
    return int(attempt_count or 0) >= limit
