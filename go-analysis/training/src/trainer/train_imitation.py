"""Train imitation model using LightGBM lambdarank."""
import json
import lightgbm as lgb
import numpy as np
import pandas as pd
from datetime import datetime, timezone
from sqlalchemy import text
from trainer.config import Settings
from trainer.feature_specs import FEATURE_SPEC_VERSION
from trainer.candidates import generate_candidates
from trainer.db import get_engine


def run(settings: Settings):
    """Train the imitation model.

    Uses LightGBM's lambdarank objective to learn a ranking over heroes
    that mimics professional draft decisions. The model is trained per-match
    (groups) so that NDCG is computed within each draft context.
    """
    decisions = pd.read_parquet(settings.artifact_dir / "decisions.parquet")

    # Fetch all known hero IDs for candidate generation.
    engine = get_engine(settings)
    hero_df = pd.read_sql(
        text("SELECT DISTINCT hero_id FROM public.picks_bans ORDER BY hero_id"),
        engine,
    )
    all_heroes = hero_df["hero_id"].tolist()

    # Generate negative samples (unpicked heroes with label=0).
    print("Generating candidates...")
    decisions = generate_candidates(decisions, all_heroes)

    # Simplified: use hero_id as the only feature for now.
    # Full implementation uses the 8-feature vector from feature_specs.py.

    # Split by match_id (critical for ranking: never split inside a match).
    match_ids = decisions["match_id"].unique()
    np.random.seed(42)
    np.random.shuffle(match_ids)
    split_idx = int(len(match_ids) * 0.8)

    train_df = decisions[decisions["match_id"].isin(match_ids[:split_idx])]
    val_df = decisions[decisions["match_id"].isin(match_ids[split_idx:])]

    X_train = train_df[["hero_id"]].values.astype(float)
    y_train = train_df["label"].values
    groups_train = train_df.groupby("match_id").size().values

    X_val = val_df[["hero_id"]].values.astype(float)
    y_val = val_df["label"].values
    groups_val = val_df.groupby("match_id").size().values

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

    # Save feature spec — Must match the actual features used for training!
    simplified_features = [{"name": "hero_id", "dtype": "f32"}]
    spec = {
        "version": FEATURE_SPEC_VERSION,
        "features": simplified_features,
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
