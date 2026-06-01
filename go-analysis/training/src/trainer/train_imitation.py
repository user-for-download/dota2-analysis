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

    Design decision — split matches BEFORE feature computation:
    Hero-prior features (pick rate, win rate) are computed from the training
    set ONLY to prevent validation match leakage.  Validation uses the full
    corpus (simulating production where priors reflect all available data).
    """
    raw_decisions = pd.read_parquet(settings.artifact_dir / "decisions.parquet")

    # Fetch all known hero IDs for candidate generation.
    engine = get_engine(settings)
    hero_df = pd.read_sql(
        text("SELECT DISTINCT hero_id FROM public.picks_bans ORDER BY hero_id"),
        engine,
    )
    all_heroes = hero_df["hero_id"].tolist()

    # ── Split match IDs FIRST (prevents hero prior leakage into val) ──────
    match_ids = raw_decisions["match_id"].unique()
    np.random.seed(42)
    np.random.shuffle(match_ids)
    split_idx = int(len(match_ids) * 0.8)
    train_match_ids = set(match_ids[:split_idx])
    val_match_ids = set(match_ids[split_idx:])

    train_decisions = raw_decisions[raw_decisions["match_id"].isin(train_match_ids)]
    val_decisions = raw_decisions[raw_decisions["match_id"].isin(val_match_ids)]

    print(f"Train matches: {len(train_match_ids)}, Val matches: {len(val_match_ids)}")

    # ── Training candidates — features computed with train-only priors ────
    print("Generating training candidates...")
    train_candidates = generate_candidates(train_decisions, all_heroes)
    print("Computing training features (train-only hero priors, fresh MVs)...")
    train_candidates = compute_features(
        train_candidates, settings, raw_decisions=raw_decisions,
        train_match_ids=train_match_ids,
        refresh_mvs=True,  # first call — refresh MVs
    )

    # ── Validation candidates — features computed with full-corpus priors ──
    # Using full-corpus priors simulates production: in inference, the model
    # will query hero priors from all historical data, not just a held-out set.
    print("Generating validation candidates...")
    val_candidates = generate_candidates(val_decisions, all_heroes)
    print("Computing validation features (full-corpus hero priors)...")
    val_candidates = compute_features(
        val_candidates, settings, raw_decisions=raw_decisions,
        train_match_ids=None,  # full corpus
        refresh_mvs=False,  # MVs already refreshed above
    )

    # ── Combine for persistence (evaluate.py reads candidates.parquet) ────
    train_candidates["split"] = "train"
    val_candidates["split"] = "val"
    candidates = pd.concat([train_candidates, val_candidates], ignore_index=True)
    cand_path = settings.artifact_dir / "candidates.parquet"
    candidates.to_parquet(cand_path, index=False)
    print(f"Saved {len(candidates)} feature-rich candidates to {cand_path}")

    # Feature column names must match FEATURES order in feature_specs.py.
    feature_cols = [f["name"] for f in FEATURES]
    print(f"Training with {len(feature_cols)} features: {feature_cols}")

    # Sanity checks: verify all feature columns exist with no NaN/inf.
    missing = [c for c in feature_cols if c not in candidates.columns]
    if missing:
        raise RuntimeError(f"Missing feature columns: {missing}")
    for col in feature_cols:
        for subset, name in [(train_candidates, "train"), (val_candidates, "val")]:
            if subset[col].isna().any():
                raise RuntimeError(f"{name} feature '{col}' has {subset[col].isna().sum()} NaN values")
            if np.isinf(subset[col].values).any():
                raise RuntimeError(f"{name} feature '{col}' has inf values")

    # Warn about constant features (zero ranking signal for LambdaMART).
    for col in feature_cols:
        n_unique = train_candidates[col].nunique()
        if n_unique == 1:
            val = train_candidates[col].iloc[0]
            print(f"  WARNING: Feature '{col}' is CONSTANT (value={val:.4f}) in training set")
            print(f"    → Zero ranking signal. Likely an empty MV — check MVs are populated.")
        elif n_unique <= 5:
            print(f"  NOTE: Feature '{col}' has only {n_unique} unique values (low cardinality)")

    # Create a unique decision ID for grouping (match_id + slot)
    # LambdaMART must rank candidates WITHIN a single decision context,
    # not across the entire match.
    train_candidates = train_candidates.sort_values(["match_id", "slot"])
    val_candidates = val_candidates.sort_values(["match_id", "slot"])

    X_train = train_candidates[feature_cols].values.astype(float)
    y_train = train_candidates["label"].values
    groups_train = train_candidates.groupby(["match_id", "slot"], sort=False).size().values

    X_val = val_candidates[feature_cols].values.astype(float)
    y_val = val_candidates["label"].values
    groups_val = val_candidates.groupby(["match_id", "slot"], sort=False).size().values

    # Free memory
    import gc
    del train_candidates
    del val_candidates
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

    # HACK: The Go inference library (dmitryikh/leaves) does not support the
    # 'lambdarank' objective. Because lambdarank outputs a raw score just like
    # regression, we can safely overwrite the objective string in the saved
    # model file to 'regression' to bypass the parser's strict validation check.
    model_text = model_path.read_text(encoding="utf-8")
    model_text = model_text.replace("objective=lambdarank", "objective=regression")
    model_path.write_text(model_text, encoding="utf-8")

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

    # Feature importance (gain = improvement when this feature is used in splits).
    importance = booster.feature_importance(importance_type="gain")
    feat_importance = {
        name: float(gain)  # numpy → native Python for JSON serialisation
        for name, gain in sorted(
            zip(feature_cols, importance), key=lambda x: x[1], reverse=True
        )
    }

    meta = {
        "version": f"imitation-v{settings.patch_id}-{dir_ts}",
        "trained_at": iso_ts,
        "recall_at_5": -1.0,   # placeholder — evaluate.py computes this; -1.0 = not yet computed
        "ndcg_at_10": -1.0,    # placeholder — evaluate.py computes this; -1.0 = not yet computed
        "best_iter": booster.best_iteration,
        "patch_id": settings.patch_id,
        "feature_importance_gain": feat_importance,
        "train_matches": len(train_match_ids),
        "val_matches": len(val_match_ids),
    }
    with open(out_dir / "meta.json", "w") as f:
        json.dump(meta, f, indent=2)

    print(f"Imitation model saved to {out_dir}")
