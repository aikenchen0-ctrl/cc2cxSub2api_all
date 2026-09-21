"""Pull legal-form company names out of page text.

Python rewrite of spiderfoot `modules/sfp_company.py`
(MIT, Copyright (c) Steve Micallef 2018). Logic ported, SpiderFoot
runtime was not copied.
"""

from __future__ import annotations

import re

_SUFFIXES = (
    r"有限公司",
    r"股份有限公司",
    r"集团有限公司",
    r"Pty\.?\s+Ltd\.?",
    r"Pvt\.?\s+Ltd\.?",
    r"Pte\.?\s+Ltd\.?",
    r"L\.L\.C\.?",
    r"LLC",
    r"Ltd\.?",
    r"Limited",
    r"Incorporated",
    r"Inc\.?",
    r"Corporation",
    r"Corp\.?",
    r"GmbH",
    r"S\.A\.R\.L\.?",
    r"SARL",
    r"S\.A\.?",
    r"PLC",
    r"B\.V\.?",
    r"N\.V\.?",
    r"A\.G\.?",
    r"AG",
    r"SIA",
)

_NAME_RE = re.compile(
    r"([A-Z0-9\u4e00-\u9fff][\w\u4e00-\u9fff&.,'’\- ]{1,60}?)\s+(" + "|".join(_SUFFIXES) + r")\b",
    re.I,
)

_NOISE = re.compile(r"^(copyright|all rights reserved|\d{4})$", re.I)


def extract_company_names(text: str, *, limit: int = 8) -> list[str]:
    """Return unique legal-form company names found in `text`."""
    found: list[str] = []
    seen: set[str] = set()
    for match in _NAME_RE.finditer(text or ""):
        name = re.sub(r"\s+", " ", f"{match.group(1)} {match.group(2)}").strip(" ,.;:-")
        if _NOISE.match(name) or len(name) < 4:
            continue
        key = name.lower()
        if key in seen:
            continue
        seen.add(key)
        found.append(name)
        if len(found) >= limit:
            break
    return found
