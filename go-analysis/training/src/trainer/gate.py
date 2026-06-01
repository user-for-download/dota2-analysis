"""Regression gate before publishing."""
import json
from trainer.config import Settings


def _get_meta(name: str, settings) -> dict:
    return json.loads(
        (settings.artifact_dir / name / "meta.json").read_text()
    )


def run(settings: Settings):
    """Compare new model metrics against minimum thresholds and deployed model.

    Checks:
      1. Absolute floor (restricted set): recall@5 must exceed
         settings.recall_threshold.
      2. Absolute floor (full pool):      recall_at_5_full must exceed
         settings.recall_threshold * 0.5 (full-pool is harder).
      3. Regression (restricted set):     new recall@5 must not be
         > settings.recall_threshold below the deployed version.
    """
    meta = _get_meta("imitation", settings)

    # ── Sentinel check: if evaluate.py hasn't run, values are -1.0 ──────
    recall = meta.get("recall_at_5", -1.0)
    recall_full = meta.get("recall_at_5_full", -1.0)
    ndcg = meta.get("ndcg_at_10", -1.0)

    if recall < 0 or recall_full < 0 or ndcg < 0:
        raise SystemExit(
            f"Gate failed: evaluation metrics not computed yet. "
            f"recall@5={recall:.1f}  recall_at_5_full={recall_full:.1f}  ndcg@10={ndcg:.1f}  "
            f"(run 'evaluate' before 'gate')"
        )

    tolerance = 0.01

    # ── Absolute floor check (restricted set, ~31 candidates/slot) ──────
    if recall < settings.recall_threshold:
        raise SystemExit(
            f"Gate failed: recall@5 {recall:.4f} "
            f"< minimum threshold {settings.recall_threshold}"
        )

    # ── Absolute floor check (full pool, ~127 candidates/slot) ──────────
    full_threshold = settings.recall_threshold * 0.5
    if recall_full < full_threshold:
        raise SystemExit(
            f"Gate failed: recall_at_5_full {recall_full:.4f} "
            f"< full-pool threshold {full_threshold:.4f} "
            f"(={settings.recall_threshold} × 0.5)"
        )

    # ── Regression check against deployed model ─────────────────────────
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
        elif recall < deployed_recall - tolerance:
            raise SystemExit(
                f"Regression: recall@5 {recall:.4f} "
                f"< deployed {deployed_recall:.4f} - {tolerance}"
            )
        else:
            print(
                f"Gate passed: {recall:.4f} >= "
                f"max({settings.recall_threshold}, {deployed_recall:.4f} - {tolerance})"
            )
    else:
        print(f"Gate passed (restricted): {recall:.4f} >= {settings.recall_threshold}")
        print(f"Gate passed (full pool):  {recall_full:.4f} >= {full_threshold:.4f}")
