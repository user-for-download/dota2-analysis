"""Train imitation model using LightGBM lambdarank."""
import json
import lightgbm as lgb
import numpy as np
import pandas as pd
from datetime import datetime, timezone
from trainer.config import Settings
from trainer.feature_specs import FEATURE_SPEC_VERSION, FEATURES


def run(settings: Settings):
    """Train the imitation model.

    Uses LightGBM's lambdarank objective to learn a ranking over heroes
    that mimics professional draft decisions. The model is trained per-match
    (groups) so that NDCG is computed within each draft context.
    """
    decisions = pd.read_parquet(settings.artifact_dir / "decisions.parquet")

    # Simplified: use hero_id as the only feature for now.
    # Full implementation uses the 8-feature vector from feature_specs.py.
    X = decisions[["hero_id"]].values.astype(float)
    y = decisions["label"].values

    # Group by match_id for lambdarank — each match is one ranking problem.
    groups = decisions.groupby("match_id").size().values

    train_data = lgb.Dataset(X, label=y, group=groups)

    params = {
        "objective": "lambdarank",
        "metric": "ndcg",
        "ndcg_eval_at": [1, 3, 5, 10],
        "num_leaves": settings.num_leaves,
        "learning_rate": settings.learning_rate,
        "num_iterations": settings.n_estimators,
        "min_child_samples": settings.min_child_samples,
        "verbose": -1,
    }

    booster = lgb.train(params, train_data)

    # Save model
    out_dir = settings.artifact_dir / "imitation"
    out_dir.mkdir(parents=True, exist_ok=True)

    model_path = out_dir / "model.bin"
    booster.save_model(str(model_path))

    # Save feature spec — Go inference validates this at boot.
    spec = {
        "version": FEATURE_SPEC_VERSION,
        "features": FEATURES,
    }
    with open(out_dir / "spec.json", "w") as f:
        json.dump(spec, f, indent=2)

    # Save metadata
    timestamp = datetime.now(timezone.utc).strftime("%Y%m%d-%H%M%S")
    meta = {
        "version": f"imitation-v{settings.patch_id}-{timestamp}",
        "trained_at": timestamp,
        "recall_at_5": 0.0,  # placeholder — evaluate separately
        "ndcg_at_10": 0.0,
        "best_iter": booster.best_iteration,
        "patch_id": settings.patch_id,
    }
    with open(out_dir / "meta.json", "w") as f:
        json.dump(meta, f, indent=2)

    print(f"Imitation model saved to {out_dir}")
