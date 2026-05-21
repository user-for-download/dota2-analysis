"""Configuration for the trainer."""
from pydantic_settings import BaseSettings
from pathlib import Path


class Settings(BaseSettings):
    """Trainer configuration from environment variables.

    All settings can be overridden via TRAINER_* environment variables.
    The postgres_dsn must match the DSN used by the Go analysis service
    (same Postgres instance, same credentials).
    """

    # Database — must match Go service DSN format
    postgres_dsn: str = "postgresql://dota2:dota2@localhost:5432/dota2"

    # Patch ID to train on
    patch_id: int = 72

    # Paths
    artifact_dir: Path = Path("/app/artifacts")
    model_dir: Path = Path("/app/deploy/models")

    # Training hyperparameters
    n_estimators: int = 1000
    learning_rate: float = 0.05
    num_leaves: int = 31
    min_child_samples: int = 20
    early_stopping_rounds: int = 50

    # Evaluation thresholds
    recall_threshold: float = 0.3
    ndcg_threshold: float = 0.4

    model_config = {"env_prefix": "TRAINER_"}
