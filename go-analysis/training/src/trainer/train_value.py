"""Train value model using LightGBM binary classification."""
import json
import lightgbm as lgb
import numpy as np
import pandas as pd
from datetime import datetime, timezone
from trainer.config import Settings
from trainer.feature_specs import FEATURE_SPEC_VERSION, FEATURES
from trainer.features import compute_features


def run(settings: Settings):
    """Train the value model.

    Binary classification: predicts whether the acting team wins
    given a draft decision. Uses the full feature set defined in
    feature_specs.py (team picks, win rates, synergies, counters,
    meta attributes, player comfort, star threat).

    Used to complement the imitation model in the combined
    recommendation scorer.
    """
    decisions = pd.read_parquet(settings.artifact_dir / "decisions.parquet")

    # Compute features if not already present (extract.py may pass through).
    decisions = compute_features(decisions, settings)

    # Build feature matrix from the canonical feature spec
    feature_cols = [f["name"] for f in FEATURES]

    # Split by match_id (rows within a match are correlated).
    match_ids = decisions["match_id"].unique()
    np.random.seed(42)
    np.random.shuffle(match_ids)
    split_idx = int(len(match_ids) * 0.8)

    train_df = decisions[decisions["match_id"].isin(match_ids[:split_idx])]
    val_df = decisions[decisions["match_id"].isin(match_ids[split_idx:])]

    X_train = train_df[feature_cols].values
    y_train = train_df["acting_won"].astype(int).values
    X_val = val_df[feature_cols].values
    y_val = val_df["acting_won"].astype(int).values

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

    # Save feature spec so inference can validate input shape
    spec = {
        "version": FEATURE_SPEC_VERSION,
        "features": FEATURES,
    }
    with open(out_dir / "spec.json", "w") as f:
        json.dump(spec, f, indent=2)

    timestamp = datetime.now(timezone.utc).strftime("%Y%m%d-%H%M%S")
    meta = {
        "version": f"value-v{settings.patch_id}-{timestamp}",
        "trained_at": timestamp,
        "auc": 0.0,
        "best_iter": booster.best_iteration,
        "patch_id": settings.patch_id,
    }
    with open(out_dir / "meta.json", "w") as f:
        json.dump(meta, f, indent=2)

    print(f"Value model saved to {out_dir}")
