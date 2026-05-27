"""Train imitation model using LightGBM lambdarank."""
import json
import lightgbm as lgb
import numpy as np
import pandas as pd
from datetime import datetime, timezone
from sqlalchemy import text
from trainer.config import Settings
from trainer.feature_specs import FEATURE_SPEC_VERSION, FEATURES
from trainer.candidates import generate_candidates
from trainer.features import compute_features
from trainer.db import get_engine


def run(settings: Settings):
    """Train the imitation model.

    Uses LightGBM's lambdarank objective to learn a ranking over heroes
    that mimics professional draft decisions. The model is trained per-match
    (groups) so that NDCG is computed within each draft context.
    """
    raw_decisions = pd.read_parquet(settings.artifact_dir / "decisions.parquet")

    # Fetch all known hero IDs for candidate generation.
    engine = get_engine(settings)
    hero_df = pd.read_sql(
        text("SELECT DISTINCT hero_id FROM public.picks_bans ORDER BY hero_id"),
        engine,
    )
    all_heroes = hero_df["hero_id"].tolist()

    # Generate negative samples (unpicked heroes with label=0).
    print("Generating candidates...")
    candidates = generate_candidates(raw_decisions, all_heroes)

    # Compute all features (MV-dependent + MV-independent).
    print("Computing features...")
    candidates = compute_features(candidates, settings, raw_decisions=raw_decisions)

    # Save full candidate dataset (features + labels) for evaluate.py.
    cand_path = settings.artifact_dir / "candidates.parquet"
    candidates.to_parquet(cand_path, index=False)
    print(f"Saved {len(candidates)} feature-rich candidates to {cand_path}")

    # Feature column names must match FEATURES order in feature_specs.py.
    feature_cols = [f["name"] for f in FEATURES]
    print(f"Training with {len(feature_cols)} features: {feature_cols}")

    # Sanity check: verify all feature columns exist.
    missing = [c for c in feature_cols if c not in candidates.columns]
    if missing:
        raise RuntimeError(f"Missing feature columns: {missing}")

    # Sanity check: verify no NaN or inf values (breaks LightGBM training).
    for col in feature_cols:
        if candidates[col].isna().any():
            n_na = candidates[col].isna().sum()
            raise RuntimeError(f"Feature column '{col}' has {n_na} NaN values")
        if np.isinf(candidates[col]).any():
            n_inf = np.isinf(candidates[col].values).sum()
            raise RuntimeError(f"Feature column '{col}' has {n_inf} inf values")

    # Split by match_id (critical for ranking: never split inside a match).
    match_ids = candidates["match_id"].unique()
    np.random.seed(42)
    np.random.shuffle(match_ids)
    split_idx = int(len(match_ids) * 0.8)

    train_df = candidates[candidates["match_id"].isin(match_ids[:split_idx])]
    val_df = candidates[candidates["match_id"].isin(match_ids[split_idx:])]

    # Create a unique decision ID for grouping (match_id + slot)
    # LambdaMART must rank candidates WITHIN a single decision context,
    # not across the entire match.
    train_df = train_df.sort_values(["match_id", "slot"])
    val_df = val_df.sort_values(["match_id", "slot"])

    X_train = train_df[feature_cols].values.astype(float)
    y_train = train_df["label"].values
    groups_train = train_df.groupby(["match_id", "slot"], sort=False).size().values

    X_val = val_df[feature_cols].values.astype(float)
    y_val = val_df["label"].values
    groups_val = val_df.groupby(["match_id", "slot"], sort=False).size().values

    # Free memory
    import gc
    del train_df
    del val_df
    del candidates
    gc.collect()

    train_data = lgb.Dataset(X_train, label=y_train, group=groups_train)
    val_data = lgb.Dataset(X_val, label=y_val, group=groups_val, reference=train_data)

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

    booster = lgb.train(
        params, train_data,
        valid_sets=[val_data],
        callbacks=[lgb.early_stopping(settings.early_stopping_rounds)],
    )

    # Save model
    out_dir = settings.artifact_dir / "imitation"
    out_dir.mkdir(parents=True, exist_ok=True)

    model_path = out_dir / "model.bin"
    booster.save_model(str(model_path))

    # Save feature spec — must match FEATURES in feature_specs.py.
    spec = {
        "version": FEATURE_SPEC_VERSION,
        "features": FEATURES,
    }
    with open(out_dir / "spec.json", "w") as f:
        json.dump(spec, f, indent=2)
    print(f"Saved spec.json with {len(FEATURES)} features")

    # Save metadata
    dir_ts = datetime.now(timezone.utc).strftime("%Y%m%d-%H%M%S")
    iso_ts = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    meta = {
        "version": f"imitation-v{settings.patch_id}-{dir_ts}",
        "trained_at": iso_ts,
        "recall_at_5": 0.0,  # placeholder — evaluate separately
        "ndcg_at_10": 0.0,
        "best_iter": booster.best_iteration,
        "patch_id": settings.patch_id,
    }
    with open(out_dir / "meta.json", "w") as f:
        json.dump(meta, f, indent=2)

    print(f"Imitation model saved to {out_dir}")
