from __future__ import annotations

import argparse
import hashlib
import itertools
import json
import math
import sys
from pathlib import Path
from typing import Any


REPOSITORY_ROOT = Path(__file__).resolve().parents[2]
MODELTRACE_ROOT = REPOSITORY_ROOT / "ModelTrace"
sys.path.insert(0, str(MODELTRACE_ROOT))

from bank_builder import fit_robust_artifacts, read_rows  # noqa: E402
import fingerprint  # noqa: E402


DEFAULT_THRESHOLDS = (0.95, 0.90, 0.20)
DEFAULT_CANDIDATE_SIMILARITY_MARGIN = 0.10


def environment_disjoint_folds(environments: list[str]) -> list[dict[str, list[str]]]:
    ordered = sorted(set(environments))
    if len(ordered) < 4 or len(ordered) % 4 != 0:
        raise ValueError("environment calibration needs a multiple of four conditions")
    chunk_size = len(ordered) // 4
    chunks = [
        ordered[index * chunk_size : (index + 1) * chunk_size]
        for index in range(4)
    ]
    return [
        {
            "validation": chunks[rotation],
            "calibration": chunks[(rotation + 1) % 4],
            "training": chunks[(rotation + 2) % 4] + chunks[(rotation + 3) % 4],
        }
        for rotation in range(4)
    ]


def load_reference_rows() -> tuple[list[dict[str, Any]], dict[str, Any]]:
    sources = (
        ("gpt_reference.jsonl", "gpt", "GPT"),
        ("claude_reference.jsonl", "claude", "Claude"),
    )
    rows: list[dict[str, Any]] = []
    for filename, family_id, family_name in sources:
        path = MODELTRACE_ROOT / "data" / filename
        if not path.is_file():
            raise FileNotFoundError(f"ModelTrace reference data is missing: {path}")
        rows.extend(
            {**row, "family_id": family_id, "family_name": family_name}
            for row in read_rows(path)
        )
    bank_path = MODELTRACE_ROOT / "data" / "unified_bank.json"
    if not bank_path.is_file():
        raise FileNotFoundError(f"ModelTrace unified bank is missing: {bank_path}")
    return rows, json.loads(bank_path.read_text(encoding="utf-8"))


def make_fold_bank(
    rows: list[dict[str, Any]],
    full_bank: dict[str, Any],
) -> dict[str, Any]:
    model_ids = list(dict.fromkeys(row["source"] for row in rows))
    templates = {item["id"]: item for item in full_bank["models"]}
    models = []
    for model_id in model_ids:
        selected = [row for row in rows if row["source"] == model_id]
        entry = templates[model_id]
        models.append(
            {
                **entry,
                "response_count": len(selected),
                "valid_number_count": sum(len(row["numbers"]) for row in selected),
                "counts": [
                    sum(row["counts"][index] for row in selected)
                    for index in range(355)
                ],
            }
        )
    return {
        "models": models,
        "robust": fit_robust_artifacts(rows, model_ids),
        # Match production inference. The reference bank's grouped-CV
        # probability temperature is reused; it is not refit per holdout fold.
        "calibration": full_bank["calibration"],
    }


def score_holdout(bank: dict[str, Any], rows: list[dict[str, Any]]) -> dict[str, Any]:
    ordered = sorted(rows, key=lambda row: row["challenge_id"])
    analysis = fingerprint.analyze_global_outputs(
        [
            {"text": row["text"], "expected_count": row.get("requested_count", 0)}
            for row in ordered
        ],
        bank,
    )
    ranked = analysis["results"]
    top = ranked[0]
    second_probability = ranked[1]["probability"] if len(ranked) > 1 else 0.0
    return {
        "candidate": top["model"],
        "probability": top["probability"],
        "profile_similarity": top["profile_similarity"],
        "margin": top["probability"] - second_probability,
    }


def passes_gates(row: dict[str, Any], thresholds: tuple[float, float, float]) -> bool:
    minimum_probability, minimum_similarity, minimum_margin = thresholds
    return (
        row["probability"] >= minimum_probability
        and row["profile_similarity"] >= minimum_similarity
        and row["margin"] >= minimum_margin
    )


def candidate_similarity_thresholds(
    omitted_model_scores: list[dict[str, Any]],
    model_ids: list[str],
    *,
    minimum_probability: float,
    minimum_margin: float,
    similarity_margin: float,
    fallback_similarity: float = DEFAULT_THRESHOLDS[1],
) -> dict[str, float]:
    """Set a separate similarity floor for each candidate from omitted-model scores."""
    scores_by_candidate: dict[str, list[float]] = {model_id: [] for model_id in model_ids}
    for row in omitted_model_scores:
        candidate = row.get("candidate")
        if (
            candidate in scores_by_candidate
            and row.get("probability", 0.0) >= minimum_probability
            and row.get("margin", 0.0) >= minimum_margin
        ):
            similarity = row.get("profile_similarity")
            if isinstance(similarity, (int, float)) and math.isfinite(float(similarity)):
                scores_by_candidate[candidate].append(float(similarity))

    thresholds: dict[str, float] = {}
    for model_id, scores in scores_by_candidate.items():
        if not scores:
            thresholds[model_id] = fallback_similarity
            continue
        # A floor above 1 deliberately makes a candidate unidentifiable when the
        # reference bank has no separation from omitted-model simulations.
        thresholds[model_id] = max(scores) + similarity_margin
    return thresholds


def evaluate_environment_disjoint_calibration(
    rows: list[dict[str, Any]],
    full_bank: dict[str, Any],
    *,
    minimum_probability: float = DEFAULT_THRESHOLDS[0],
    minimum_margin: float = DEFAULT_THRESHOLDS[2],
    similarity_margin: float = DEFAULT_CANDIDATE_SIMILARITY_MARGIN,
) -> dict[str, Any]:
    """Validate candidate-specific floors across disjoint environment folds.

    Each rotation trains reference profiles on six environments, derives OOD
    floors from three calibration environments with the candidate model omitted,
    then measures known-model and omitted-model outcomes on three untouched
    environments. These are simulated unknowns, not genuine new-model samples.
    """
    models = [item["id"] for item in full_bank["models"]]
    environments = sorted({row["condition_id"] for row in rows})
    folds = environment_disjoint_folds(environments)
    known_total = 0
    known_top_correct = 0
    known_accepted = 0
    omitted_total = 0
    omitted_accepted = 0
    fold_summaries: list[dict[str, Any]] = []

    for fold in folds:
        validation_environments = fold["validation"]
        calibration_environments = fold["calibration"]
        train_environments = fold["training"]
        train_rows = [row for row in rows if row["condition_id"] in train_environments]
        calibration_rows = [row for row in rows if row["condition_id"] in calibration_environments]
        validation_rows = [row for row in rows if row["condition_id"] in validation_environments]
        known_bank = make_fold_bank(train_rows, full_bank)
        calibration_ood: list[dict[str, Any]] = []
        validation_ood: list[dict[str, Any]] = []

        for omitted_model in models:
            omitted_bank = make_fold_bank(
                [row for row in train_rows if row["source"] != omitted_model],
                full_bank,
            )
            for environment in calibration_environments:
                heldout = [
                    row for row in calibration_rows
                    if row["condition_id"] == environment and row["source"] == omitted_model
                ]
                if len(heldout) == 3:
                    calibration_ood.append({"truth": omitted_model, **score_holdout(omitted_bank, heldout)})
            for environment in validation_environments:
                heldout = [
                    row for row in validation_rows
                    if row["condition_id"] == environment and row["source"] == omitted_model
                ]
                if len(heldout) == 3:
                    validation_ood.append({"truth": omitted_model, **score_holdout(omitted_bank, heldout)})

        candidate_thresholds = candidate_similarity_thresholds(
            calibration_ood,
            models,
            minimum_probability=minimum_probability,
            minimum_margin=minimum_margin,
            similarity_margin=similarity_margin,
        )
        fold_known_total = 0
        fold_known_correct = 0
        fold_known_accepted = 0
        for model in models:
            for environment in validation_environments:
                heldout = [
                    row for row in validation_rows
                    if row["condition_id"] == environment and row["source"] == model
                ]
                if len(heldout) != 3:
                    continue
                score = score_holdout(known_bank, heldout)
                accepted = (
                    score["candidate"] == model
                    and score["probability"] >= minimum_probability
                    and score["margin"] >= minimum_margin
                    and score["profile_similarity"] >= candidate_thresholds[model]
                )
                fold_known_total += 1
                fold_known_correct += score["candidate"] == model
                fold_known_accepted += accepted

        fold_omitted_accepted = sum(
            score["probability"] >= minimum_probability
            and score["margin"] >= minimum_margin
            and score["profile_similarity"] >= candidate_thresholds.get(score["candidate"], DEFAULT_THRESHOLDS[1])
            for score in validation_ood
        )
        known_total += fold_known_total
        known_top_correct += fold_known_correct
        known_accepted += fold_known_accepted
        omitted_total += len(validation_ood)
        omitted_accepted += fold_omitted_accepted
        fold_summaries.append(
            {
                "training_environments": train_environments,
                "calibration_environments": calibration_environments,
                "validation_environments": validation_environments,
                "known_cases": fold_known_total,
                "known_top_candidate_correct": fold_known_correct,
                "known_correct_candidates_accepted": fold_known_accepted,
                "simulated_ood_cases": len(validation_ood),
                "simulated_ood_candidates_accepted": fold_omitted_accepted,
            }
        )

    return {
        "method": "four_rotating_environment_disjoint_folds; per-candidate max omitted-model similarity plus fixed margin",
        "minimum_probability": minimum_probability,
        "minimum_margin": minimum_margin,
        "similarity_margin": similarity_margin,
        "known_cases": known_total,
        "known_top_candidate_correct": known_top_correct,
        "known_correct_candidates_accepted": known_accepted,
        "simulated_ood_cases": omitted_total,
        "simulated_ood_candidates_accepted": omitted_accepted,
        "folds": fold_summaries,
        "warning": "Omitted reference models are only proxy unknowns; these results do not establish accuracy for genuine unseen models.",
    }


def build_candidate_calibration(
    rows: list[dict[str, Any]],
    full_bank: dict[str, Any],
    bank_sha256: str,
    fingerprint_sha256: str,
    *,
    minimum_probability: float = DEFAULT_THRESHOLDS[0],
    minimum_margin: float = DEFAULT_THRESHOLDS[2],
    similarity_margin: float = DEFAULT_CANDIDATE_SIMILARITY_MARGIN,
) -> dict[str, Any]:
    """Create deployment thresholds tied to the exact reference bank hash."""
    models = [item["id"] for item in full_bank["models"]]
    environments = sorted({row["condition_id"] for row in rows})
    omitted_scores: list[dict[str, Any]] = []
    for omitted_model in models:
        omitted_bank = make_fold_bank(
            [row for row in rows if row["source"] != omitted_model],
            full_bank,
        )
        for environment in environments:
            heldout = [
                row for row in rows
                if row["condition_id"] == environment and row["source"] == omitted_model
            ]
            if len(heldout) == 3:
                omitted_scores.append({"truth": omitted_model, **score_holdout(omitted_bank, heldout)})
    thresholds = candidate_similarity_thresholds(
        omitted_scores,
        models,
        minimum_probability=minimum_probability,
        minimum_margin=minimum_margin,
        similarity_margin=similarity_margin,
    )
    return {
        "schema_version": 1,
        "bank_sha256": bank_sha256,
        "fingerprint_sha256": fingerprint_sha256,
        "method": "per-candidate maximum similarity among omitted-reference-model simulations plus fixed margin",
        "minimum_probability": minimum_probability,
        "minimum_margin": minimum_margin,
        "similarity_margin": similarity_margin,
        "omitted_model_simulation_cases": len(omitted_scores),
        "candidate_minimum_profile_similarity": thresholds,
        "warning": "Thresholds are calibrated only against omitted reference models. Genuine unseen-model rejection remains unvalidated.",
    }


def evaluate(
    rows: list[dict[str, Any]],
    full_bank: dict[str, Any],
    thresholds: tuple[float, float, float],
) -> dict[str, Any]:
    models = [item["id"] for item in full_bank["models"]]
    environments = sorted({row["condition_id"] for row in rows})
    known: list[dict[str, Any]] = []
    for environment in environments:
        train = [row for row in rows if row["condition_id"] != environment]
        bank = make_fold_bank(train, full_bank)
        for model in models:
            heldout = [
                row
                for row in rows
                if row["condition_id"] == environment and row["source"] == model
            ]
            if len(heldout) == 3:
                known.append({"truth": model, **score_holdout(bank, heldout)})

    omitted_model: list[dict[str, Any]] = []
    for model in models:
        train = [row for row in rows if row["source"] != model]
        bank = make_fold_bank(train, full_bank)
        for environment in environments:
            heldout = [
                row
                for row in rows
                if row["condition_id"] == environment and row["source"] == model
            ]
            if len(heldout) == 3:
                omitted_model.append({"truth": model, **score_holdout(bank, heldout)})

    default_known_passes = sum(
        row["candidate"] == row["truth"] and passes_gates(row, thresholds)
        for row in known
    )
    default_omitted_passes = sum(passes_gates(row, thresholds) for row in omitted_model)

    grid = [step / 20 for step in range(21)]
    best_zero_omitted_accepts: list[tuple[int, float, float, float]] = []
    for candidate_thresholds in itertools.product(grid, repeat=3):
        if any(passes_gates(row, candidate_thresholds) for row in omitted_model):
            continue
        covered = sum(
            row["candidate"] == row["truth"] and passes_gates(row, candidate_thresholds)
            for row in known
        )
        best_zero_omitted_accepts.append((covered, *candidate_thresholds))

    return {
        "purpose": "exploratory threshold review; not real out-of-bank validation",
        "known_holdout_method": "hold out one complete challenge environment from robust feature fitting",
        "omitted_model_method": "remove each model from the reference rows and test its 12 held-out environments",
        "probability_temperature": "reused from the checked-in unified bank grouped cross-validation",
        "thresholds": {
            "minimum_probability": thresholds[0],
            "minimum_profile_similarity": thresholds[1],
            "minimum_margin": thresholds[2],
        },
        "known_holdout": {
            "cases": len(known),
            "top_candidate_correct": sum(row["candidate"] == row["truth"] for row in known),
            "correct_candidates_passing_gates": default_known_passes,
        },
        "omitted_model_simulation": {
            "cases": len(omitted_model),
            "candidates_passing_gates": default_omitted_passes,
        },
        "best_zero_accept_grid_thresholds": [
            {
                "correct_known_cases": covered,
                "minimum_probability": probability,
                "minimum_profile_similarity": similarity,
                "minimum_margin": margin,
            }
            for covered, probability, similarity, margin in sorted(
                best_zero_omitted_accepts, reverse=True
            )[:10]
        ],
        "warning": "Omitted reference models are not genuine unseen models. Do not describe this result as open-set accuracy or use it to claim calibrated unknown detection.",
    }


def main() -> None:
    parser = argparse.ArgumentParser(description="Offline ModelTrace threshold review")
    parser.add_argument("--min-probability", type=float, default=DEFAULT_THRESHOLDS[0])
    parser.add_argument("--min-profile-similarity", type=float, default=DEFAULT_THRESHOLDS[1])
    parser.add_argument("--min-margin", type=float, default=DEFAULT_THRESHOLDS[2])
    parser.add_argument(
        "--candidate-similarity-margin",
        type=float,
        default=DEFAULT_CANDIDATE_SIMILARITY_MARGIN,
        help="extra similarity above each candidate's maximum omitted-model simulation score",
    )
    parser.add_argument(
        "--write-calibration",
        type=Path,
        help="write a bank-hash-bound candidate threshold profile to this path",
    )
    args = parser.parse_args()
    thresholds = (args.min_probability, args.min_profile_similarity, args.min_margin)
    if any(value < 0 or value > 1 for value in thresholds) or not 0 <= args.candidate_similarity_margin <= 1:
        parser.error("all thresholds must be between 0 and 1")
    rows, bank = load_reference_rows()
    bank_path = MODELTRACE_ROOT / "data" / "unified_bank.json"
    fingerprint_path = MODELTRACE_ROOT / "fingerprint.py"
    bank_sha256 = hashlib.sha256(bank_path.read_bytes()).hexdigest()
    fingerprint_sha256 = hashlib.sha256(fingerprint_path.read_bytes()).hexdigest()
    report = evaluate(rows, bank, thresholds)
    report["environment_disjoint_candidate_calibration"] = evaluate_environment_disjoint_calibration(
        rows,
        bank,
        minimum_probability=args.min_probability,
        minimum_margin=args.min_margin,
        similarity_margin=args.candidate_similarity_margin,
    )
    if args.write_calibration is not None:
        calibration = build_candidate_calibration(
            rows,
            bank,
            bank_sha256,
            fingerprint_sha256,
            minimum_probability=args.min_probability,
            minimum_margin=args.min_margin,
            similarity_margin=args.candidate_similarity_margin,
        )
        args.write_calibration.parent.mkdir(parents=True, exist_ok=True)
        args.write_calibration.write_text(
            json.dumps(calibration, ensure_ascii=False, indent=2) + "\n",
            encoding="utf-8",
        )
        report["calibration_written_to"] = str(args.write_calibration)
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
