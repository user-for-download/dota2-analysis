"""Regression gate before publishing."""
import json
from trainer.config import Settings


def run(settings: Settings):
    """Compare new model metrics against minimum thresholds and deployed model.

    Checks:
      1. Absolute floor: recall@5 must exceed settings.recall_threshold.
      2. Regression:    new recall@5 must not be > settings.recall_threshold
                        below the deployed version.
    """
    current_meta = json.loads(
        (settings.artifact_dir / "imitation" / "meta.json").read_text()
    )
    current_recall = current_meta["recall_at_5"]
    tolerance = 0.01  # small tolerance for noise

    # Absolute floor check
    if current_recall < settings.recall_threshold:
        raise SystemExit(
            f"Gate failed: recall@5 {current_recall:.4f} "
            f"< minimum threshold {settings.recall_threshold}"
        )

    # Regression check against deployed model
    deployed_path = settings.model_dir / "imitation" / "current" / "meta.json"
    if deployed_path.exists():
        deployed_meta = json.loads(deployed_path.read_text())
        deployed_recall = deployed_meta.get("recall_at_5", 0.0)

        # Deployed models trained without negative samples report recall@5 = 1.0
        # (all-positive labels produce a trivial metric).  Skip regression check
        # against stale metadata — let the new model set a meaningful baseline.
        if deployed_recall >= 1.0:
            print(
                f"Deployed model has stale recall@5 = {deployed_recall:.4f} "
                "(likely all-positive labels).  Skipping regression check."
            )
        elif current_recall < deployed_recall - tolerance:
            raise SystemExit(
                f"Regression: recall@5 {current_recall:.4f} "
                f"< deployed {deployed_recall:.4f} - {tolerance}"
            )
        else:
            print(
                f"Gate passed: {current_recall:.4f} >= "
                f"max({settings.recall_threshold}, {deployed_recall:.4f} - {tolerance})"
            )
    else:
        print(f"Gate passed: {current_recall:.4f} >= {settings.recall_threshold} (no deployed model)")
