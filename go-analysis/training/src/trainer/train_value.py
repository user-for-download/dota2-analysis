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
    given a draft decision.  Uses the full 24-feature spec (same as
    the imitation model) so the Go side can feed feature vectors
    from the same builder to both scorers without a spec mismatch.

    The decisions Parquet has one row per actual pick (no negative
    candidates).  We copy it, add label=1.0, and run the standard
    feature computation pipeline.  The value model is a binary
    classifier, not a ranker — no negative samples are needed.
    """
    decisions = pd.read_parquet(settings.artifact_dir / "decisions.parquet")

    # Use actual pick decisions as the candidate set with label=1.0.
    # compute_features needs the label column for its internal merge
    # logic (distinguishing actual picks from undrafted heroes).
    candidates = decisions.copy()
    candidates["label"] = 1.0

    # Compute the full 24-feature spec — must match the imitation
    # model's feature set so the Go scorer accepts the vectors.
    print("Computing features for value model...")
    candidates = compute_features(candidates, settings, raw_decisions=decisions)

    # Feature columns must match FEATURES in feature_specs.py.
    feature_cols = [f["name"] for f in FEATURES]
    missing = [c for c in feature_cols if c not in candidates.columns]
    if missing:
        raise RuntimeError(f"Missing feature columns for value model: {missing}")

    # Split by match_id (rows within a match are correlated).
    match_ids = candidates["match_id"].unique()
    np.random.seed(42)
    np.random.shuffle(match_ids)
    split_idx = int(len(match_ids) * 0.8)

    train_df = candidates[candidates["match_id"].isin(match_ids[:split_idx])]
    val_df = candidates[candidates["match_id"].isin(match_ids[split_idx:])]

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
    spec = {
        "version": FEATURE_SPEC_VERSION,
        "features": FEATURES,
    }
    with open(out_dir / "spec.json", "w") as f:
        json.dump(spec, f, indent=2)
    print(f"Saved spec.json with {len(FEATURES)} features (matching imitation spec)")

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
