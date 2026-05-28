"""Candidate generation — must match Go implementation."""
import pandas as pd
import numpy as np

# Number of negative (unpicked) candidates per decision slot.
# Higher = harder ranking task = better learning signal.
# With ~127 total heroes and ~10 already drafted per match,
# late-slot decisions will have fewer available heroes — min() handles this.
_NEGATIVES_PER_SLOT = 30


def _available_heroes(
    all_heroes: list[int], drafted_so_far: set[int], exclude: int
) -> list[int]:
    """Return all heroes that haven't been drafted yet, minus *exclude*."""
    return [h for h in all_heroes if h not in drafted_so_far and h != exclude]


def generate_candidates(decisions: pd.DataFrame, all_heroes: list[int]) -> pd.DataFrame:
    """Generate candidate heroes per decision slot.

    For each pick in a match (processed in slot order), produces one
    positive sample (the actual pick, label=1.0) and ~50 negative samples
    (undrafted heroes still available at that point in the draft, label=0.0).

    This gives lambdarank a realistic ranking pool — dozens of candidates
    per group — instead of the trivial 2-3 candidates from shared sampling.

    Must match the candidate generation logic in the Go recommender.
    """
    rows: list[dict] = []

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
            available = _available_heroes(all_heroes, drafted_so_far, hero)
            n_neg = min(_NEGATIVES_PER_SLOT, len(available))
            if n_neg > 0:
                neg_heroes = np.random.choice(available, size=n_neg, replace=False)
                for neg_id in neg_heroes:
                    r = row.to_dict()
                    r["hero_id"] = neg_id
                    r["label"] = 0.0
                    rows.append(r)

    return pd.DataFrame(rows)
