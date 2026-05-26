"""Candidate generation — must match Go implementation."""
import pandas as pd
import numpy as np


def generate_candidates(decisions: pd.DataFrame, all_heroes: list[int]) -> pd.DataFrame:
    """Generate candidate heroes for each decision (exclude already drafted).

    For each match, produces positive samples (actually-picked heroes, label=1.0)
    and negative samples (undrafted available heroes, label=0.0). This gives
    lambdarank meaningful items to rank rather than all-positive groups.

    Must match the candidate generation logic in the Go recommender.
    """
    rows = []

    # Get all drafted heroes per match to exclude from candidates
    drafted_per_match = decisions.groupby("match_id")["hero_id"].apply(set).to_dict()

    for match_id, group in decisions.groupby("match_id"):
        drafted = drafted_per_match[match_id]
        available = [h for h in all_heroes if h not in drafted]

        # Positive samples: the actual picks
        for _, row in group.iterrows():
            r = row.to_dict()
            r["label"] = 1.0
            rows.append(r)

        # Negative samples: unpicked available heroes.
        # Limit to ~20 per match to keep dataset balanced and memory reasonable.
        n_neg = min(20, len(available))
        neg_heroes = np.random.choice(available, size=n_neg, replace=False)

        # Use the first pick's row as a template for context columns (team, opponent, etc.)
        template = group.iloc[0].to_dict()
        for hero_id in neg_heroes:
            r = template.copy()
            r["hero_id"] = hero_id
            r["label"] = 0.0
            rows.append(r)

    return pd.DataFrame(rows)
