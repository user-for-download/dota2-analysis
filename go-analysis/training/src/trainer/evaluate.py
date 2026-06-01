"""Evaluate trained models — restricted set + full pool + outcome predictor.

CANDIDATE POOL ASYMMETRY (train vs. eval):
  Training:  ~31 candidates/slot  (1 positive + 30 sampled negatives)
  Full-pool: ~127 candidates/slot (1 positive + all undrafted heroes)

This gap means restricted-set recall@5 is NOT comparable to full-pool recall@5.
The full pool is inherently harder (model must rank 1/127 instead of 1/31).

The gate.py applies a 0.5x scaling factor to account for this, but this is
a rough heuristic — expect a natural gap.  If needed, you can train with
`max_negatives=len(all_heroes)` to match production, at the cost of speed.
"""
import json
import gc
import pandas as pd
import lightgbm as lgb
import numpy as np
from sqlalchemy import text
from sklearn.metrics import roc_auc_score, accuracy_score
from trainer.config import Settings
from trainer.feature_specs import FEATURES
from trainer.candidates import generate_candidates
from trainer.features import compute_features
from trainer.db import get_engine


def _metrics_for_imitation(
    booster: lgb.Booster,
    candidates: pd.DataFrame,
    feature_cols: list[str],
    label: str = "Evaluation",
) -> dict[str, float]:
    """Compute recall@5 and ndcg@10 per decision slot.

    For each (match_id, slot) group, finds the rank of the actual pick
    (label=1.0) among all candidates sorted by model score, then computes:

      recall@5  = 1.0 if rank <= 5  else 0.0  (averaged across slots)
      ndcg@10   = 1/log2(rank+1)    if rank <= 10 else 0.0

    Binary relevance: exactly 1 positive per slot, all others negative.
    """
    df = candidates.sort_values(["match_id", "slot"]).reset_index(drop=True)
    X = df[feature_cols].values.astype(float)
    predictions = booster.predict(X)

    recall_at_5 = []
    ndcg_at_10 = []

    for (_match_id, _slot), group in df.groupby(["match_id", "slot"], sort=False):
        preds = predictions[group.index]

        picked = group.loc[group["label"] == 1.0, "hero_id"].values
        if len(picked) == 0:
            continue

        actual_pick = picked[0]

        # Sort descending by prediction score
        sorted_idx = np.argsort(preds)[::-1]
        ranked_heroes = group.iloc[sorted_idx]["hero_id"].values

        matches = np.where(ranked_heroes == actual_pick)[0]
        if len(matches) == 0:
            continue  # edge case: actual pick not in candidate set
        rank = matches[0] + 1  # 1-indexed

        recall_at_5.append(1.0 if rank <= 5 else 0.0)

        if rank <= 10:
            ndcg_at_10.append(1.0 / np.log2(rank + 1))
        else:
            ndcg_at_10.append(0.0)

    avg_recall = float(np.mean(recall_at_5)) if recall_at_5 else 0.0
    avg_ndcg = float(np.mean(ndcg_at_10)) if ndcg_at_10 else 0.0

    print(f"{label}: recall@5={avg_recall:.4f}  ndcg@10={avg_ndcg:.4f}  "
          f"(n={len(recall_at_5)} decisions)")

    return {"recall_at_5": avg_recall, "ndcg_at_10": avg_ndcg}


def _update_meta(artifact_name: str, settings: Settings, updates: dict) -> None:
    """Merge *updates* into meta.json for *artifact_name*, preserving existing fields."""
    meta_path = settings.artifact_dir / artifact_name / "meta.json"
    with open(meta_path) as f:
        meta = json.load(f)
    meta.update(updates)
    with open(meta_path, "w") as f:
        json.dump(meta, f, indent=2)


def _evaluate_imitation(settings: Settings, feature_cols: list[str]) -> dict:
    """Evaluate the imitation model on both restricted and full-pool candidate sets."""
    cand_path = settings.artifact_dir / "candidates.parquet"
    if not cand_path.exists():
        raise FileNotFoundError(f"{cand_path} not found. Run training first.")

    candidates = pd.read_parquet(cand_path)
    missing = [c for c in feature_cols if c not in candidates.columns]
    if missing:
        raise RuntimeError(
            f"Candidates.parquet is missing feature columns {missing}. "
            f"Available: {list(candidates.columns)}"
        )

    model_path = settings.artifact_dir / "imitation" / "model.bin"
    booster = lgb.Booster(model_file=str(model_path))

    # ── 1. Restricted set: metrics on the training candidate pool (~31/slot) ───
    restricted = _metrics_for_imitation(booster, candidates, feature_cols,
                                        label="Restricted set (~31 cand/slot)")

    # ── 2. Full pool: generate ALL available heroes per slot, compute features,
    #       and evaluate.  This gives a metric comparable to production inference
    #       where the model ranks all ~127 heroes. ──────────────────────────────
    print("Computing full-pool evaluation (~127 heroes/slot)...")
    raw_decisions = pd.read_parquet(settings.artifact_dir / "decisions.parquet")

    engine = get_engine(settings)
    hero_df = pd.read_sql(
        text("SELECT id FROM public.heroes ORDER BY id"),
        engine,
    )
    all_heroes: list[int] = hero_df["id"].tolist()

    full_candidates = generate_candidates(
        raw_decisions, all_heroes, max_negatives=len(all_heroes),
    )
    full_candidates = compute_features(full_candidates, settings,
                                        raw_decisions=raw_decisions,
                                        refresh_mvs=False)

    full = _metrics_for_imitation(booster, full_candidates, feature_cols,
                                  label="Full pool (~127 cand/slot)")

    # ── 3. Write all metrics to meta.json ─────────────────────────────────────
    all_metrics = {
        "recall_at_5": restricted["recall_at_5"],
        "ndcg_at_10": restricted["ndcg_at_10"],
        "recall_at_5_full": full["recall_at_5"],
        "ndcg_at_10_full": full["ndcg_at_10"],
    }
    _update_meta("imitation", settings, all_metrics)
    print("Wrote imitation metrics (restricted + full) to meta.json")

    # Return full-pool metrics for gate.py
    return full


def _evaluate_value(settings: Settings, feature_cols: list[str]) -> None:
    """Evaluate the value model (binary classifier) on a validation split."""
    model_path = settings.artifact_dir / "value" / "model.bin"
    if not model_path.exists():
        print("No value model found — skipping value evaluation")
        return

    print("Evaluating value model...")

    # Replicate the train_value.py pipeline to get a validation set.
    decisions = pd.read_parquet(settings.artifact_dir / "decisions.parquet")
    val_candidates = decisions.copy()
    val_candidates["label"] = 1.0

    # Compute features (uses same compute_features as training).
    # refresh_mvs=False: MVs already refreshed during training.
    val_candidates = compute_features(val_candidates, settings,
                                       raw_decisions=decisions,
                                       refresh_mvs=False)

    missing = [c for c in feature_cols if c not in val_candidates.columns]
    if missing:
        raise RuntimeError(f"Missing feature columns for value eval: {missing}")

    # Split by match_id — same 80/20 split as train_value.py.
    match_ids = val_candidates["match_id"].unique()
    rng = np.random.default_rng(42)
    rng.shuffle(match_ids)
    split_idx = int(len(match_ids) * 0.8)
    val_df = val_candidates[~val_candidates["match_id"].isin(match_ids[:split_idx])]

    X_val = val_df[feature_cols].values.astype(float)
    y_val = val_df["value_label"].values

    booster = lgb.Booster(model_file=str(model_path))
    preds = booster.predict(X_val)

    auc = float(roc_auc_score(y_val, preds))
    acc = float(accuracy_score(y_val, (preds >= 0.5).astype(float)))

    print(f"Value model:  AUC={auc:.4f}  accuracy={acc:.4f}  "
          f"(n={len(y_val)} decisions)")

    _update_meta("value", settings, {"auc": auc, "accuracy": acc})
    print("Wrote value metrics to meta.json")


def run(settings: Settings) -> None:
    """Evaluate both imitation and value models."""
    feature_cols = [f["name"] for f in FEATURES]

    # ── Imitation model ────────────────────────────────────────────────────────
    full_pool_metrics = _evaluate_imitation(settings, feature_cols)

    # ── Value model ────────────────────────────────────────────────────────────
    _evaluate_value(settings, feature_cols)

    # ── Gate check on full-pool metric ─────────────────────────────────────────
    # If the full-pool recall@5 is far below the restricted-set value, warn.
    restricted_recall = full_pool_metrics.get("recall_at_5", 0.0)
    # (full_pool_metrics is actually full-pool here; we can't compare without
    #  loading both — the gate.py does this properly.)
    print(f"\nEvaluation complete. Full-pool recall@5 = {full_pool_metrics['recall_at_5']:.4f}")
