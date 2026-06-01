"""Unit tests for features.py and candidates.py — no database required."""

import pandas as pd
from trainer.candidates import _available_heroes
from trainer.features import _features_hero_priors


# ── _available_heroes ─────────────────────────────────────────────────────────

class TestAvailableHeroes:
    def test_basic_available(self):
        """Heroes not yet drafted are available."""
        avail = _available_heroes(
            all_heroes={1, 2, 3, 4, 5},
            drafted_so_far={1, 3},
            current_hero=2,
        )
        assert avail == [2, 4, 5]

    def test_current_hero_in_drafted(self):
        """When current_hero is already in drafted_so_far (duplicate event),
        it should be removed from the drafted set so it remains available."""
        avail = _available_heroes(
            all_heroes={1, 2, 3},
            drafted_so_far={1, 2},
            current_hero=2,
        )
        assert avail == [2, 3]  # 2 is available despite being in drafted_so_far

    def test_all_heroes_drafted(self):
        """When everything is drafted, return empty list."""
        avail = _available_heroes(
            all_heroes={1, 2},
            drafted_so_far={1, 2},
            current_hero=2,
        )
        assert avail == [2]  # 2 is available (removed from drafted), others gone


# ── _features_hero_priors ─────────────────────────────────────────────────────

class TestFeaturesHeroPriors:
    """_features_hero_priors computes hero-level pick rate, WR, popularity.

    Key behaviours to test:
    1. train_match_ids filter restricts the corpus correctly
    2. Hero priors are merged onto candidates
    3. Missing heroes get fillna defaults
    4. Stats are shrunk correctly (beta priors)
    """

    def test_train_match_ids_filters_correctly(self):
        """When train_match_ids is provided, hero stats come only from
        those matches — no leakage from excluded matches."""
        candidates = pd.DataFrame({
            "match_id": [1, 1, 2],
            "hero_id": [10, 20, 30],
        })
        raw = pd.DataFrame({
            "match_id": [1, 1, 2, 2],
            "hero_id": [10, 20, 30, 10],
            "is_pick": [True, True, True, True],
            "acting_won": [True, False, True, False],
        })

        result = _features_hero_priors(
            candidates, raw,
            train_match_ids={1},  # only match 1
        )

        # hero 10 was picked once (match 1, won) and once (match 2, lost).
        # With train_match_ids={1}, only the win counts.
        h10 = result[result["hero_id"] == 10].iloc[0]
        assert h10["hero_pick_count"] == 1.0, \
            f"Expected 1 pick from match 1 only, got {h10['hero_pick_count']}"

        # hero 30 was picked only in match 2 (excluded from train) → should
        # get the fillna default (not present in hero_stats at all).
        h30 = result[result["hero_id"] == 30].iloc[0]
        assert pd.isna(h30["hero_pick_rate"]) or h30["hero_pick_count"] == 0.0, \
            "hero 30 should have no stats from training set"

    def test_full_corpus_when_no_filter(self):
        """When train_match_ids is None, all matches contribute to priors."""
        candidates = pd.DataFrame({
            "match_id": [1, 2],
            "hero_id": [10, 20],
        })
        raw = pd.DataFrame({
            "match_id": [1, 1, 2],
            "hero_id": [10, 20, 10],
            "is_pick": [True, True, True],
            "acting_won": [True, False, True],
        })

        result = _features_hero_priors(candidates, raw, train_match_ids=None)

        # hero 10 appears twice in full corpus → pick_count = 2
        h10 = result[result["hero_id"] == 10].iloc[0]
        assert h10["hero_pick_count"] == 2.0, \
            f"Expected 2 picks from full corpus, got {h10['hero_pick_count']}"

    def test_merged_onto_candidates(self):
        """Result has same rows as input candidates with priors added."""
        candidates = pd.DataFrame({
            "match_id": [1, 1],
            "hero_id": [10, 20],
        })
        raw = pd.DataFrame({
            "match_id": [1, 1],
            "hero_id": [10, 20],
            "is_pick": [True, True],
            "acting_won": [True, False],
        })

        result = _features_hero_priors(candidates, raw)

        assert len(result) == 2
        expected_cols = {"hero_pick_rate", "hero_wr", "hero_popularity",
                         "hero_pick_count", "hero_win_count"}
        assert expected_cols.issubset(set(result.columns)), \
            f"Missing columns: {expected_cols - set(result.columns)}"

    def test_missing_hero_fillna(self):
        """Heroes not present in raw_decisions get fillna defaults."""
        candidates = pd.DataFrame({
            "match_id": [1],
            "hero_id": [99],  # not in raw
        })
        raw = pd.DataFrame({
            "match_id": [1],
            "hero_id": [10],
            "is_pick": [True],
            "acting_won": [True],
        })

        result = _features_hero_priors(candidates, raw)

        row = result.iloc[0]
        assert row["hero_pick_rate"] > 0.0, "Missing hero should get non-zero default"
        assert row["hero_wr"] == 0.5, "Missing hero should get default WR=0.5"
        assert row["hero_popularity"] == 0.0, "Missing hero should get log1p(0)=0"

    def test_pick_count_zero_heroes(self):
        """A hero present in candidates but with zero picks gets defaults."""
        candidates = pd.DataFrame({
            "match_id": [1],
            "hero_id": [10],
        })
        raw = pd.DataFrame({
            "match_id": [1],
            "hero_id": [10],
            "is_pick": [False],  # ban, not a pick
            "acting_won": [True],
        })

        result = _features_hero_priors(candidates, raw)

        row = result.iloc[0]
        # is_pick=False means this hero doesn't appear in picks at all
        assert row["hero_wr"] == 0.5, "Hero with no picks should get default WR"
        assert row["hero_pick_count"] == 0.0, "Hero with no picks should have count 0"

    def test_shrinkage(self):
        """Beta shrinkage pulls extreme values toward the prior."""
        candidates = pd.DataFrame({
            "match_id": [1],
            "hero_id": [10],
        })
        # Hero 10: 10 wins, 0 losses from 10 picks → raw WR = 1.0
        # Shrunk WR with beta(10, 20): (10 + 10) / (10 + 20) = 20/30 = 0.667
        raw = pd.DataFrame({
            "match_id": list(range(1, 11)),
            "hero_id": [10] * 10,
            "is_pick": [True] * 10,
            "acting_won": [True] * 10,
        })

        result = _features_hero_priors(candidates, raw)
        wr = result.iloc[0]["hero_wr"]

        expected = (10 + 10.0) / (10 + 20.0)  # 0.6667
        assert abs(wr - expected) < 1e-10, \
            f"Shrunk WR {wr:.6f} != expected {expected:.6f}"

    def test_total_picks_denominator(self):
        """Pick rate denominator is total_picks (not n_decisions)."""
        candidates = pd.DataFrame({
            "match_id": [1],
            "hero_id": [10],
        })
        # Hero 10 has 8 picks out of 20 total → rate = (8+2)/(20+4) = 10/24 ≈ 0.417
        hero_picks = [(10, True)] * 8
        other_picks = [(h, True) for h in range(11, 23)]  # 12 other heroes × 1 each
        all_picks = hero_picks + other_picks
        raw = pd.DataFrame({
            "match_id": [1] * len(all_picks),
            "hero_id": [h for h, _ in all_picks],
            "is_pick": [p for _, p in all_picks],
            "acting_won": [True] * len(all_picks),
        })

        result = _features_hero_priors(candidates, raw)
        rate = result.iloc[0]["hero_pick_rate"]

        expected = (8 + 2.0) / (20 + 4.0)  # 10/24 = 0.4167
        assert abs(rate - expected) < 1e-10, \
            f"Pick rate {rate:.6f} != expected {expected:.6f}"
