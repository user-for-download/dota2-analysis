"""Evaluate trained models."""
import json
import pandas as pd
import lightgbm as lgb
import numpy as np
from trainer.config import Settings
from trainer.feature_specs import FEATURES


def run(settings: Settings):
    """Evaluate the imitation model.

    Computes Recall@5 per match: for each match, checks how many of the
    actually picked heroes (label=1.0) appear in the model's top 5
    predictions out of all candidates.  A match with 10 picks that gets
    7 of them in the top 5 has recall@5 = 0.7.
    """
    cand_path = settings.artifact_dir / "candidates.parquet"
    if not cand_path.exists():
        raise FileNotFoundError(f"{cand_path} not found. Run training first.")

    decisions = pd.read_parquet(cand_path)

    # Feature columns must match the model's training features.
    feature_cols = [f["name"] for f in FEATURES]
    missing = [c for c in feature_cols if c not in decisions.columns]
    if missing:
        raise RuntimeError(
            f"Candidates.parquet is missing feature columns {missing}. "
            f"Available: {list(decisions.columns)}"
        )

    model_path = settings.artifact_dir / "imitation" / "model.bin"
    booster = lgb.Booster(model_file=str(model_path))

    # Compute Recall@5 per decision — is the actual pick in the top 5?
    recall_at_5 = []
    
    # Sort to ensure predictions align with groups.
    # reset_index(drop=True) is critical: groupby(...).index is a pandas Index,
    # but predictions is a positional numpy array.  Without reset_index the
    # index labels may not align with positional indices after sorting, causing
    # preds[group.index] to fetch wrong rows.
    decisions = decisions.sort_values(["match_id", "slot"]).reset_index(drop=True)
    X = decisions[feature_cols].values.astype(float)
    predictions = booster.predict(X)

    # Free memory
    del X
    import gc
    gc.collect()

    for (match_id, slot), group in decisions.groupby(["match_id", "slot"], sort=False):
        preds = predictions[group.index]

        picked = group.loc[group["label"] == 1.0, "hero_id"].values
        if len(picked) == 0:
            continue  # Skip if no positive label exists

        # There should be exactly 1 positive label per decision slot
        actual_pick = picked[0]

        # Top 5 predicted heroes among candidates for this specific slot
        top5_idx = np.argsort(preds)[-5:]
        top5_heroes = set(group.iloc[top5_idx]["hero_id"].values)

        # Was the actual pick in the top 5?
        hit = 1.0 if actual_pick in top5_heroes else 0.0
        recall_at_5.append(hit)

    avg_recall = np.mean(recall_at_5) if recall_at_5 else 0.0
    print(f"Recall@5: {avg_recall:.4f}")

    # Update meta.json with computed metrics
    meta_path = settings.artifact_dir / "imitation" / "meta.json"
    with open(meta_path) as f:
        meta = json.load(f)
    meta["recall_at_5"] = float(avg_recall)
    with open(meta_path, "w") as f:
        json.dump(meta, f, indent=2)
