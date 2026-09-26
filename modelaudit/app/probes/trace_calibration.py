from __future__ import annotations

import json
import math
from pathlib import Path
from typing import Any


CALIBRATION_PATH = (
    Path(__file__).resolve().parents[2]
    / "vendor"
    / "modeltrace"
    / "candidate_thresholds.json"
)


def load_candidate_calibration(
    bank_sha256: str,
    fingerprint_sha256: str,
) -> tuple[dict[str, Any] | None, str]:
    """Load only calibration built for the exact bank and classifier source."""
    try:
        value = json.loads(CALIBRATION_PATH.read_text(encoding="utf-8"))
    except (OSError, UnicodeDecodeError, json.JSONDecodeError):
        return None, "calibration_file_unavailable"
    if not isinstance(value, dict) or value.get("schema_version") != 1:
        return None, "calibration_file_invalid"
    if value.get("bank_sha256") != bank_sha256:
        return None, "calibration_bank_hash_mismatch"
    if value.get("fingerprint_sha256") != fingerprint_sha256:
        return None, "calibration_fingerprint_hash_mismatch"

    thresholds = value.get("candidate_minimum_profile_similarity")
    if not isinstance(thresholds, dict) or not thresholds:
        return None, "candidate_thresholds_missing"
    checked: dict[str, float] = {}
    for model, threshold in thresholds.items():
        if (
            not isinstance(model, str)
            or not model
            or isinstance(threshold, bool)
            or not isinstance(threshold, (int, float))
            or not math.isfinite(float(threshold))
            or threshold < 0
            or threshold > 2
        ):
            return None, "candidate_thresholds_invalid"
        checked[model] = float(threshold)

    minimum_probability = value.get("minimum_probability")
    minimum_margin = value.get("minimum_margin")
    similarity_margin = value.get("similarity_margin")
    numbers = (minimum_probability, minimum_margin, similarity_margin)
    if any(
        isinstance(number, bool)
        or not isinstance(number, (int, float))
        or not math.isfinite(float(number))
        or number < 0
        or number > 1
        for number in numbers
    ):
        return None, "calibration_thresholds_invalid"

    return {**value, "candidate_minimum_profile_similarity": checked}, "verified"
