"""Evaluate trained models."""
import json
import pandas as pd
import lightgbm as lgb
import numpy as np
from trainer.config import Settings


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

    model_path = settings.artifact_dir / "imitation" / "model.bin"
    booster = lgb.Booster(model_file=str(model_path))

    X = decisions[["hero_id"]].values.astype(float)
    predictions = booster.predict(X)

    # Compute Recall@5 per match — fraction of actually-picked heroes
    # that appear in the model's top 5 predictions.
    recall_at_5 = []
    for match_id, group in decisions.groupby("match_id"):
        preds = predictions[group.index]

        picked = group.loc[group["label"] == 1.0, "hero_id"].values
        if len(picked) == 0:
            continue  # Skip if no positive label exists

        n_picked = len(picked)

        # Top 5 predicted heroes among all candidates for this match.
        top5_idx = np.argsort(preds)[-5:]
        top5_heroes = set(group.iloc[top5_idx]["hero_id"].values)

        # How many of the actually-picked heroes are in the top 5?
        hits = sum(1 for h in picked if h in top5_heroes)
        recall_at_5.append(hits / n_picked)

    avg_recall = np.mean(recall_at_5) if recall_at_5 else 0.0
    print(f"Recall@5: {avg_recall:.4f}")

    # Update meta.json with computed metrics
    meta_path = settings.artifact_dir / "imitation" / "meta.json"
    with open(meta_path) as f:
        meta = json.load(f)
    meta["recall_at_5"] = float(avg_recall)
    with open(meta_path, "w") as f:
        json.dump(meta, f, indent=2)
