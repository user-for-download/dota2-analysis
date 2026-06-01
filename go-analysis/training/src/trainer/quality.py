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

    # Minimum data quality — explicit raise (not assert, which is disabled under python -O)
    if len(decisions) <= 100:
        raise RuntimeError(f"Too few decisions: {len(decisions)}")
    if decisions["match_id"].nunique() <= 10:
        raise RuntimeError(f"Too few matches: {decisions['match_id'].nunique()}")
    print("Quality checks passed.")
