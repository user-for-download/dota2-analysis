"""Evaluate trained models."""
import json
import pandas as pd
import lightgbm as lgb
import numpy as np
from trainer.config import Settings


def run(settings: Settings):
    """Evaluate the imitation model.

    Computes Recall@K per match: for each match, checks whether the
    actually chosen hero appears in the model's top-K predictions.
    """
    decisions = pd.read_parquet(settings.artifact_dir / "decisions.parquet")

    model_path = settings.artifact_dir / "imitation" / "model.bin"
    booster = lgb.Booster(model_file=str(model_path))

    X = decisions[["hero_id"]].values.astype(float)
    predictions = booster.predict(X)

    # Compute Recall@5 per match
    recall_at_5 = []
    for match_id, group in decisions.groupby("match_id"):
        preds = predictions[group.index]
        chosen = group["chosen_hero"].values
        top5 = group.iloc[np.argsort(preds)[-5:]]["hero_id"].values
        hit = any(h in top5 for h in chosen)
        recall_at_5.append(int(hit))

    avg_recall = np.mean(recall_at_5) if recall_at_5 else 0.0
    print(f"Recall@5: {avg_recall:.4f}")

    # Update meta.json with computed metrics
    meta_path = settings.artifact_dir / "imitation" / "meta.json"
    with open(meta_path) as f:
        meta = json.load(f)
    meta["recall_at_5"] = float(avg_recall)
    with open(meta_path, "w") as f:
        json.dump(meta, f, indent=2)
