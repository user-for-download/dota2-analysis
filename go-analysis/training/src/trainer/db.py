"""Database connection helper."""
from sqlalchemy import create_engine
from trainer.config import Settings


def get_engine(settings: Settings):
    """Create a SQLAlchemy engine from settings.

    Uses pool_pre_ping to detect stale connections automatically.
    """
    return create_engine(settings.postgres_dsn, pool_pre_ping=True)
