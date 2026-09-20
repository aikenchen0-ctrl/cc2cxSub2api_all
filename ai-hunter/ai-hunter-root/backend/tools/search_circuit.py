"""Fail-fast latch for one round of free-web searches.

The first SearchBackendError trips the circuit. Later keywords skip the
network instead of waiting through another HTML + lite timeout.
"""

from __future__ import annotations


class SearchCircuit:
    """Shared latch: trip once, later callers see the same error."""

    def __init__(self) -> None:
        self._error = ""

    def trip(self, error: str) -> None:
        text = str(error or "").strip()
        if text and not self._error:
            self._error = text

    def tripped(self) -> bool:
        return bool(self._error)

    @property
    def error(self) -> str:
        return self._error
