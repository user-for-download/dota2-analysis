"""Data quality checks before training."""
import pandas as pd
from trainer.config import Settings


def run(settings: Settings):
    """Run data quality gates.

    Asserts minimum thresholds for training data volume and diversity.
    Fails fast if data is insufficient, preventing wasted training cycles.
    """
    decisions = pd.read_parquet(settings.artifact_dir / "decisions.parquet")

    checks = {
        "total_decisions": len(decisions),
        "unique_matches": decisions["match_id"].nunique(),
        "unique_heroes": decisions["hero_id"].nunique(),
        "missing_values": decisions.isnull().sum().to_dict(),
    }

    print("Data quality report:")
    for k, v in checks.items():
        print(f"  {k}: {v}")

    # Assert minimum data quality
    assert len(decisions) > 100, f"Too few decisions: {len(decisions)}"
    assert decisions["match_id"].nunique() > 10, f"Too few matches: {decisions['match_id'].nunique()}"
    print("Quality checks passed.")
