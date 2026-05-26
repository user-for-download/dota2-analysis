"""Evaluate trained models.
"""
import json
import pandas as pd
import lightgbm as lgb
import numpy as np
from trainer.config import Settings


def run(settings: Settings):
    """Evaluate the imitation model.

    Computes Recall@5 per match: checks whether the actually chosen
    hero (label=1.0) appears in the model's top 5 predicted heroes
    out of all available candidates.
    """
    cand_path = settings.artifact_dir / "candidates.parquet"
    if not cand_path.exists():
        raise FileNotFoundError(f"{cand_path} not found. Run training first.")

    decisions = pd.read_parquet(cand_path)

    model_path = settings.artifact_dir / "imitation" / "model.bin"
    booster = lgb.Booster(model_file=str(model_path))

    X = decisions[["hero_id"]].values.astype(float)
    predictions = booster.predict(X)

    # Compute Recall@5 per match
    recall_at_5 = []
    for match_id, group in decisions.groupby("match_id"):
        preds = predictions[group.index]

        # Find the one hero that was actually chosen (label=1.0)
        chosen_heroes = group.loc[group["label"] == 1.0, "hero_id"].values
        if len(chosen_heroes) == 0:
            continue  # Skip if no positive label exists
        chosen_hero = chosen_heroes[0]

        # Get top 5 predicted heroes from the candidates
        top5_idx = np.argsort(preds)[-5:]
        top5_heroes = group.iloc[top5_idx]["hero_id"].values

        # Check if the chosen hero is in the top 5
        hit = chosen_hero in top5_heroes
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
