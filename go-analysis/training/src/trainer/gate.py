"""Regression gate before publishing."""
import json
from trainer.config import Settings


def run(settings: Settings):
    """Compare new model metrics against deployed model.

    Prevents publishing a model that is significantly worse than the
    currently deployed one. Allows a small tolerance (0.01) for noise.
    """
    current_meta = json.loads(
        (settings.artifact_dir / "imitation" / "meta.json").read_text()
    )

    deployed_path = settings.model_dir / "imitation" / "current" / "meta.json"
    if deployed_path.exists():
        deployed_meta = json.loads(deployed_path.read_text())
        if current_meta["recall_at_5"] < deployed_meta["recall_at_5"] - 0.01:
            raise SystemExit(
                f"Regression: recall@5 {current_meta['recall_at_5']:.4f} "
                f"< {deployed_meta['recall_at_5']:.4f} - 0.01"
            )
        print(
            f"Gate passed: {current_meta['recall_at_5']:.4f} >= "
            f"{deployed_meta['recall_at_5']:.4f} - 0.01"
        )
    else:
        print("No deployed model — gate skipped.")
