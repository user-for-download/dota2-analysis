"""Shared fixtures for trainer tests.

All tests use synthetic DataFrames — no database connection needed.
"""
import pandas as pd
import pytest


@pytest.fixture
def sample_decisions() -> pd.DataFrame:
    """10 synthetic draft decisions across 2 matches.

    Each match has 5 picks for radiant (team=0) then 5 for dire (team=1).
    acting_won alternates to give a 50/50 class balance.
    """
    rows = []
    for match_id in (1001, 1002):
        for slot, hero_id in enumerate(
            [1, 2, 3, 4, 5, 6, 7, 8, 9, 10], start=1
        ):
            is_radiant = slot <= 5
            rows.append({
                "match_id": match_id,
                "slot": slot,
                "is_pick": True,
                "hero_id": hero_id,
                "team": 0 if is_radiant else 1,
                "acting_team": 101 if is_radiant else 102,
                "opp_team": 102 if is_radiant else 101,
                "acting_won": (slot % 2 == 0),  # alternating win/loss
            })
    return pd.DataFrame(rows)


@pytest.fixture
def sample_candidates(sample_decisions) -> pd.DataFrame:
    """Candidates with 1 positive + 3 negatives per slot for match 1001."""
    rows = []
    for (match_id, slot), group in sample_decisions.query(
        "match_id == 1001"
    ).groupby(["match_id", "slot"]):
        actual = group.iloc[0]
        # Positive sample
        rows.append({
            "match_id": match_id,
            "slot": slot,
            "hero_id": actual["hero_id"],
            "label": 1.0,
        })
        # Negative samples (fictional undrafted heroes)
        for neg_id in [11, 12, 13]:
            rows.append({
                "match_id": match_id,
                "slot": slot,
                "hero_id": neg_id,
                "label": 0.0,
            })
    return pd.DataFrame(rows)


@pytest.fixture
def all_heroes() -> list[int]:
    """Small hero pool for candidate generation."""
    return list(range(1, 21))
