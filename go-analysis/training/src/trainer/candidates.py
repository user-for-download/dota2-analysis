"""Candidate generation — must match Go implementation."""
import pandas as pd


def generate_candidates(decisions: pd.DataFrame, all_heroes: list[int]) -> pd.DataFrame:
    """Generate candidate heroes for each decision (exclude drafted).

    In the full implementation, for each pick decision this produces
    rows for every hero that was still available (not yet picked or
    banned) at that point in the draft. This ensures the imitation
    model learns to rank available heroes, not all 124.

    Must match the candidate generation logic in the Go recommender.
    """
    # TODO: Track drafted heroes per match and exclude them.
    # For now, pass through decisions as-is.
    return decisions
