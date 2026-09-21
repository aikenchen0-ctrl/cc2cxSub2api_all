"""Work-email local-part formats guessed against company domains.

Python rewrite of ai-outreach-engine `server/src/providers/emailPatterns.ts`
(MIT, Copyright (c) 2026 Rohit Malhotra). Logic ported, not copied as TypeScript.

Order is the default probe sequence when no domain preference is known.
Prefer first.last — first@ alone is rarely the corporate inbox.
"""

from __future__ import annotations

import json
import re
import unicodedata
from pathlib import Path
from typing import Iterable

EMAIL_PATTERN_KEYS = (
    "first.last",
    "flast",
    "firstlast",
    "first.lastInitial",
    "first_last",
    "f.last",
    "first",
    "last",
)

RANKED_PATTERN_LIMIT = 2

_PATTERN_HITS: dict[str, dict[str, int]] = {}
_STORE_PATH: Path | None = None


def is_email_pattern_key(value: str) -> bool:
    return value in EMAIL_PATTERN_KEYS


def normalise_name_part(value: str) -> str:
    decomposed = unicodedata.normalize("NFD", value or "")
    stripped = "".join(ch for ch in decomposed if unicodedata.category(ch) != "Mn")
    return re.sub(r"[^a-z]", "", stripped.lower())


def parse_name_parts(full_name: str) -> dict[str, str] | None:
    parts = [p for p in re.split(r"\s+", (full_name or "").strip()) if p]
    first = normalise_name_part(parts[0] if parts else "")
    last = normalise_name_part(parts[-1] if len(parts) > 1 else "")
    if not first:
        return None
    return {"first": first, "last": last}


def apply_email_pattern(key: str, full_name: str, domain: str) -> str | None:
    parts = parse_name_parts(full_name)
    if not parts:
        return None
    first = parts["first"]
    last = parts["last"]
    host = (domain or "").strip().lower()
    if not host:
        return None
    if not last:
        return f"{first}@{host}" if key == "first" else None
    local = {
        "first": first,
        "last": last,
        "first.last": f"{first}.{last}",
        "first_last": f"{first}_{last}",
        "firstlast": f"{first}{last}",
        "first.lastInitial": f"{first}.{last[0]}",
        "flast": f"{first[0]}{last}",
        "f.last": f"{first[0]}.{last}",
    }.get(key)
    if not local:
        return None
    return f"{local}@{host}"


def detect_email_pattern(email: str, full_name: str, domain: str | None = None) -> str | None:
    at = (email or "").find("@")
    if at <= 0:
        return None
    local = email[:at].lower()
    email_domain = email[at + 1 :].lower()
    if domain and email_domain != domain.strip().lower():
        return None
    if not parse_name_parts(full_name):
        return None
    for key in EMAIL_PATTERN_KEYS:
        candidate = apply_email_pattern(key, full_name, email_domain)
        if not candidate:
            continue
        if candidate.split("@", 1)[0].lower() == local:
            return key
    return None


def ordered_email_candidates(
    full_name: str,
    domain: str,
    ranked_keys: Iterable[str] = (),
) -> list[str]:
    seen: set[str] = set()
    out: list[str] = []

    def push(email: str | None) -> None:
        if not email:
            return
        normalised = email.lower()
        if normalised in seen:
            return
        seen.add(normalised)
        out.append(normalised)

    for key in ranked_keys:
        if is_email_pattern_key(key):
            push(apply_email_pattern(key, full_name, domain))
    for key in EMAIL_PATTERN_KEYS:
        push(apply_email_pattern(key, full_name, domain))
    return out


def email_patterns(full_name: str, domain: str) -> list[str]:
    return ordered_email_candidates(full_name, domain)


def set_pattern_store_path(path: str | Path | None) -> None:
    global _STORE_PATH
    _STORE_PATH = Path(path) if path else None


def _store_path() -> Path:
    if _STORE_PATH is not None:
        return _STORE_PATH
    return Path(__file__).resolve().parent.parent / "data" / "domain_email_patterns.json"


def _load_hits() -> dict[str, dict[str, int]]:
    global _PATTERN_HITS
    if _PATTERN_HITS:
        return _PATTERN_HITS
    path = _store_path()
    if path.exists():
        try:
            raw = json.loads(path.read_text(encoding="utf-8"))
            if isinstance(raw, dict):
                _PATTERN_HITS = {
                    str(domain).lower(): {
                        str(key): int(count)
                        for key, count in (hits or {}).items()
                        if is_email_pattern_key(str(key))
                    }
                    for domain, hits in raw.items()
                    if isinstance(hits, dict)
                }
        except (OSError, json.JSONDecodeError, TypeError, ValueError):
            _PATTERN_HITS = {}
    return _PATTERN_HITS


def _save_hits() -> None:
    path = _store_path()
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(_PATTERN_HITS, ensure_ascii=False, indent=2), encoding="utf-8")


def remember_domain_pattern(domain: str, key: str) -> None:
    host = (domain or "").strip().lower()
    if not host or not is_email_pattern_key(key):
        return
    hits = _load_hits()
    bucket = hits.setdefault(host, {})
    bucket[key] = int(bucket.get(key, 0) or 0) + 1
    _save_hits()


def ranked_keys_for_domain(domain: str, limit: int = RANKED_PATTERN_LIMIT) -> list[str]:
    host = (domain or "").strip().lower()
    hits = _load_hits().get(host) or {}
    ranked = sorted(hits.items(), key=lambda item: (-item[1], item[0]))
    return [key for key, _ in ranked[: max(0, limit)]]


def candidates_for_person(full_name: str, domain: str) -> list[str]:
    return ordered_email_candidates(full_name, domain, ranked_keys_for_domain(domain))
