"""Integration tests for the training pipeline.

These tests require a live Postgres database with Dota 2 match data.
They are skipped by default — run with:

    TRAINER_POSTGRES_DSN=postgresql://user:pass@host/db pytest -x tests/test_integration.py -v

Or set the env var in your .env file.
"""

import os
import pytest
import pandas as pd
import numpy as np
from pathlib import Path
from sqlalchemy import create_engine, text

from trainer.config import Settings
from trainer.extract import run as extract_run
from trainer.candidates import generate_candidates
from trainer.features import compute_features
from trainer.feature_specs import FEATURES
from trainer.train_imitation import run as train_imitation_run
from trainer.evaluate import _metrics_for_imitation


def _db_available(settings: Settings) -> bool:
    """Check if the Postgres database is reachable."""
    try:
        engine = create_engine(settings.postgres_dsn)
        with engine.connect() as conn:
            conn.execute(text("SELECT 1"))
        return True
    except Exception:
        return False


# Skip all tests in this module if no database is available.
pytestmark = pytest.mark.skipif(
    not _db_available(Settings()),
    reason="No Postgres database available (set TRAINER_POSTGRES_DSN)",
)


@pytest.fixture(scope="module")
def temp_settings(tmp_path_factory) -> Settings:
    """Settings pointing to a temporary artifact directory."""
    tmp = tmp_path_factory.mktemp("trainer_integration")
    return Settings(
        artifact_dir=tmp,
        model_dir=tmp / "deploy",
        n_estimators=50,  # small model for speed
        learning_rate=0.1,
        num_leaves=15,
        min_child_samples=5,
        early_stopping_rounds=10,
        recall_threshold=0.1,  # lower bar for smoke test
    )


class TestExtract:
    """Verify data extraction produces well-formed output."""

    def test_extract_creates_parquet(self, temp_settings):
        """extract.py produces a non-empty decisions.parquet."""
        extract_run(temp_settings)
        parquet_path = temp_settings.artifact_dir / "decisions.parquet"
        assert parquet_path.exists(), f"{parquet_path} not created"
        df = pd.read_parquet(parquet_path)
        assert len(df) > 0, "Decisions parquet is empty"
        required = {"match_id", "hero_id", "slot", "is_pick", "acting_won", "value_label"}
        assert required.issubset(set(df.columns)), \
            f"Missing columns: {required - set(df.columns)}"

    def test_extract_has_positive_and_negative_outcomes(self, temp_settings):
        """value_label has both 0 and 1 (wins and losses)."""
        df = pd.read_parquet(temp_settings.artifact_dir / "decisions.parquet")
        unique_labels = set(df["value_label"].unique())
        assert 0 in unique_labels, "No loss samples (value_label=0)"
        assert 1 in unique_labels, "No win samples (value_label=1)"


class TestTrainImitation:
    """Train a small imitation model on extracted data."""

    @pytest.fixture(autouse=True)
    def _run_extract_first(self, temp_settings):
        """Ensure extract has run before any training test."""
        extract_run(temp_settings)

    def test_training_completes(self, temp_settings):
        """train_imitation produces a model.bin and meta.json."""
        train_imitation_run(temp_settings)
        model_dir = temp_settings.artifact_dir / "imitation"
        assert (model_dir / "model.bin").exists(), "model.bin not created"
        assert (model_dir / "meta.json").exists(), "meta.json not created"
        assert (model_dir / "spec.json").exists(), "spec.json not created"

    def test_model_has_all_features(self, temp_settings):
        """Trained model's feature spec matches FEATURES."""
        import json
        spec_path = temp_settings.artifact_dir / "imitation" / "spec.json"
        spec = json.loads(spec_path.read_text())
        spec_names = [f["name"] for f in spec["features"]]
        expected = [f["name"] for f in FEATURES]
        assert spec_names == expected, \
            f"Feature mismatch: {set(spec_names) ^ set(expected)}"

    def test_metrics_are_plausible(self, temp_settings):
        """Full-pool recall@5 > 0 (better than random 5/127 ≈ 0.04)."""
        import lightgbm as lgb
        from trainer.db import get_engine
        from sqlalchemy import text

        # Load decisions and all heroes.
        decisions = pd.read_parquet(temp_settings.artifact_dir / "decisions.parquet")
        engine = get_engine(temp_settings)
        hero_df = pd.read_sql(
            text("SELECT id FROM public.heroes ORDER BY id"), engine,
        )
        all_heroes = hero_df["id"].tolist()

        # Generate full-pool candidates.
        candidates = generate_candidates(decisions, all_heroes,
                                          max_negatives=len(all_heroes))
        candidates = compute_features(candidates, temp_settings,
                                       raw_decisions=decisions,
                                       refresh_mvs=False)

        # Load the trained booster.
        booster = lgb.Booster(
            model_file=str(temp_settings.artifact_dir / "imitation" / "model.bin")
        )
        feature_cols = [f["name"] for f in FEATURES]
        metrics = _metrics_for_imitation(
            booster, candidates, feature_cols, label="Integration test",
        )

        random_baseline = 5.0 / len(all_heroes)  # ~0.039 for 127 heroes
        assert metrics["recall_at_5"] > random_baseline, (
            f"recall@5 ({metrics['recall_at_5']:.4f}) not better than "
            f"random ({random_baseline:.4f})"
        )


class TestNoDataCorruption:
    """Verify features don't contain NaN/inf after pipeline runs."""

    @pytest.fixture(autouse=True)
    def _run_pipeline(self, temp_settings):
        extract_run(temp_settings)
        train_imitation_run(temp_settings)

    def test_candidates_parquet_no_nan(self, temp_settings):
        """candidates.parquet has no NaN or inf in feature columns."""
        candidates = pd.read_parquet(temp_settings.artifact_dir / "candidates.parquet")
        feature_cols = [f["name"] for f in FEATURES]
        for col in feature_cols:
            n_nan = candidates[col].isna().sum()
            assert n_nan == 0, f"Feature '{col}' has {n_nan} NaN values"
            n_inf = np.isinf(candidates[col].values).sum()
            assert n_inf == 0, f"Feature '{col}' has {n_inf} inf values"

    def test_split_column_present(self, temp_settings):
        """candidates.parquet has 'split' column (train/val)."""
        candidates = pd.read_parquet(temp_settings.artifact_dir / "candidates.parquet")
        assert "split" in candidates.columns, "Missing 'split' column"
        splits = set(candidates["split"].unique())
        assert splits == {"train", "val"}, f"Unexpected split values: {splits}"
        n_train = (candidates["split"] == "train").sum()
        n_val = (candidates["split"] == "val").sum()
        assert n_train > 0, "No training samples"
        assert n_val > 0, "No validation samples"
