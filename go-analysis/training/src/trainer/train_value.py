"""Train value model using LightGBM binary classification."""
import json
import lightgbm as lgb
import numpy as np
import pandas as pd
from datetime import datetime, timezone
from trainer.config import Settings
from trainer.feature_specs import FEATURE_SPEC_VERSION


def run(settings: Settings):
    """Train the value model.

    Binary classification: predicts whether the acting team wins
    given a draft decision. Uses hero_id as a simplified feature
    for now.

    TODO: Replace with full 8-feature vector from feature_specs.py
    once features.compute_features() is implemented.

    Used to complement the imitation model in the combined
    recommendation scorer.
    """
    decisions = pd.read_parquet(settings.artifact_dir / "decisions.parquet")

    # Simplified: use hero_id as the only feature for now.
    # TODO: Replace with full 8-feature vector from feature_specs.py.
    feature_cols = ["hero_id"]

    # Split by match_id (rows within a match are correlated).
    match_ids = decisions["match_id"].unique()
    np.random.seed(42)
    np.random.shuffle(match_ids)
    split_idx = int(len(match_ids) * 0.8)

    train_df = decisions[decisions["match_id"].isin(match_ids[:split_idx])]
    val_df = decisions[decisions["match_id"].isin(match_ids[split_idx:])]

    X_train = train_df[feature_cols].values.astype(float)
    y_train = train_df["value_label"].values
    X_val = val_df[feature_cols].values.astype(float)
    y_val = val_df["value_label"].values

    train_data = lgb.Dataset(X_train, label=y_train, feature_name=feature_cols)
    val_data = lgb.Dataset(X_val, label=y_val, feature_name=feature_cols, reference=train_data)

    params = {
        "objective": "binary",
        "metric": "auc",
        "num_leaves": settings.num_leaves,
        "learning_rate": settings.learning_rate,
        "num_iterations": settings.n_estimators,
        "min_child_samples": settings.min_child_samples,
        "verbose": -1,
    }

    booster = lgb.train(
        params, train_data, valid_sets=[val_data],
        callbacks=[lgb.early_stopping(settings.early_stopping_rounds)],
    )

    out_dir = settings.artifact_dir / "value"
    out_dir.mkdir(parents=True, exist_ok=True)

    booster.save_model(str(out_dir / "model.bin"))

    # Save feature spec — Must match the actual features used for training!
    simplified_features = [{"name": col, "dtype": "f32"} for col in feature_cols]
    spec = {
        "version": FEATURE_SPEC_VERSION,
        "features": simplified_features,
    }
    with open(out_dir / "spec.json", "w") as f:
        json.dump(spec, f, indent=2)

    # Save metadata
    dir_ts = datetime.now(timezone.utc).strftime("%Y%m%d-%H%M%S")
    iso_ts = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    auc = booster.best_score.get("valid_0", {}).get("auc", 0.0)
    meta = {
        "version": f"value-v{settings.patch_id}-{dir_ts}",
        "trained_at": iso_ts,
        "auc": auc,
        "best_iter": booster.best_iteration,
        "patch_id": settings.patch_id,
    }
    with open(out_dir / "meta.json", "w") as f:
        json.dump(meta, f, indent=2)

    print(f"Value model saved to {out_dir}")
