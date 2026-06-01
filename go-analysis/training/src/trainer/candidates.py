"""Candidate generation — must match Go implementation."""
import warnings

import pandas as pd
import numpy as np


def _available_heroes(
    all_heroes: set[int],
    drafted_so_far: set[int],
    current_hero: int,
) -> set[int]:
    """Return the set of heroes still available to be picked/banned.

    Args:
        all_heroes: Complete hero pool for the patch.
        drafted_so_far: Heroes already picked/banned before this decision.
        current_hero: The hero being chosen at this decision point
                      (not yet in drafted_so_far, so still available).

    Returns:
        Set of hero IDs that haven't been drafted yet.
    """
    available = all_heroes - drafted_so_far
    if current_hero not in available:
        warnings.warn(
            f"Hero {current_hero} picked but not in available pool "
            f"(drafted={drafted_so_far})",
            stacklevel=2,
        )
    return available


def generate_candidates(
    decisions: pd.DataFrame,
    all_heroes: list[int],
    max_negatives: int = 30,
) -> pd.DataFrame:
    """Generate candidate heroes per decision slot.

    For each pick in a match (processed in slot order), produces one
    positive sample (the actual pick, label=1.0) and ~50 negative samples
    (undrafted heroes still available at that point in the draft, label=0.0).

    This gives lambdarank a realistic ranking pool — dozens of candidates
    per group — instead of the trivial 2-3 candidates from shared sampling.

    Must match the candidate generation logic in the Go recommender.
    """
    rows: list[dict] = []
    rng = np.random.default_rng(42)

    for match_id, group in decisions.groupby("match_id"):
        group = group.sort_values("slot")
        drafted_so_far: set[int] = set()

        for _, row in group.iterrows():
            hero: int = row["hero_id"]
            is_ban: bool = not row["is_pick"]

            # Track this hero as unavailable for future slots (picks AND bans).
            drafted_so_far.add(hero)

            # Only generate training samples for picks — bans are not decisions to recommend.
            if is_ban:
                continue

            # Positive sample: the actual pick at this slot.
            r = row.to_dict()
            r["label"] = 1.0
            rows.append(r)

            # Negative samples: heroes not yet drafted (or banned) at this point.
            # When max_negatives >= len(available), all available heroes are used
            # (full-pool evaluation). When smaller, a random subset is sampled
            # (training mode — keeps group sizes manageable for LambdaMART).
            available = _available_heroes(set(all_heroes), drafted_so_far, hero)
            n_neg = min(max_negatives, len(available))
            if n_neg > 0:
                neg_heroes = rng.choice(available, size=n_neg, replace=False)
                for neg_id in neg_heroes:
                    r = row.to_dict()
                    r["hero_id"] = neg_id
                    r["label"] = 0.0
                    rows.append(r)

    return pd.DataFrame(rows)
