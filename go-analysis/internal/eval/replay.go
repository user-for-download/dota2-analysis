package eval

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// Baseline scores a list of candidate heroes.
type Baseline interface {
	Score(candidates []domain.HeroID) []domain.Score
}

// PlayerAwareBaseline scores candidates using player roster context.
type PlayerAwareBaseline interface {
	ScoreWithRoster(ctx context.Context, roster []domain.AccountID, candidates []domain.HeroID) ([]domain.Score, error)
}

// ReplayConfig controls the backtest parameters.
type ReplayConfig struct {
	PatchID int32
	Limit   int // max matches to replay (0 = all)
}

// Replay runs the backtest against historical drafts.
// It queries matches + picks_bans for the given patch, reconstructs each
// pick decision, scores with the baseline, and aggregates metrics per phase.
func Replay(
	ctx context.Context,
	db *pgxpool.Pool,
	catalog domain.HeroCatalog,
	baseline Baseline,
	cfg ReplayConfig,
) (*BacktestResult, error) {
	// Load rosters upfront from player_matches for all matches in this patch.
	type sideRoster struct {
		radiant []domain.AccountID
		dire    []domain.AccountID
	}
	rosters := make(map[int64]*sideRoster)

	rosterRows, err := db.Query(ctx, `
		SELECT match_id, is_radiant, array_agg(account_id ORDER BY player_slot) as players
		FROM public.player_matches
		WHERE patch_id = $1
		GROUP BY match_id, is_radiant
	`, cfg.PatchID)
	if err != nil {
		// Roster data is optional — continue without it.
		// Log the failure so operator knows player-comfort baselines will be degraded.
		slog.Default().Warn("roster query failed; player comfort disabled", "err", err)
		rosterRows = nil
	}
	if rosterRows != nil {
		defer rosterRows.Close()
		for rosterRows.Next() {
			var matchID int64
			var isRadiant bool
			// Use *int64 to handle NULL account_ids (anonymous players).
			var accountPtrs []*int64
			if err := rosterRows.Scan(&matchID, &isRadiant, &accountPtrs); err != nil {
				continue
			}
			sr, ok := rosters[matchID]
			if !ok {
				sr = &sideRoster{}
				rosters[matchID] = sr
			}
			ids := make([]domain.AccountID, 0, len(accountPtrs))
			for _, ptr := range accountPtrs {
				if ptr != nil {
					ids = append(ids, domain.AccountID(*ptr))
				}
			}
			if isRadiant {
				sr.radiant = ids
			} else {
				sr.dire = ids
			}
		}
		_ = rosterRows.Err() // best effort
	}

	// Check if baseline supports player-aware scoring.
	playerBaseline, _ := baseline.(PlayerAwareBaseline)

	rows, err := db.Query(ctx, `
		SELECT pb.match_id, pb.ord, pb.is_pick, pb.hero_id, pb.team,
		       m.radiant_team_id, m.dire_team_id
		FROM public.picks_bans pb
		JOIN public.matches m ON m.match_id = pb.match_id
		WHERE m.patch_id = $1
		      AND m.leagueid > 0
		      AND m.radiant_team_id IS NOT NULL
		      AND m.dire_team_id IS NOT NULL
		ORDER BY pb.match_id, pb.ord
	`, cfg.PatchID)
	if err != nil {
		return nil, fmt.Errorf("query picks_bans: %w", err)
	}
	defer rows.Close()

	// Per-phase accumulators.
	type phaseAcc struct {
		total   int
		correct int
		sumR1   float64
		sumR3   float64
		sumR5   float64
		sumN10  float64
	}
	phaseMap := make(map[string]*phaseAcc)

	// Match-level state.
	var (
		currentMatchID   int64
		radiantTeamID    int64
		direTeamID       int64
		radiantPicks     []domain.HeroID
		direPicks        []domain.HeroID
		radiantBans      []domain.HeroID
		direBans         []domain.HeroID
		matchCount       int
		processedMatches = make(map[int64]bool)
	)

	phases := domain.CMPhaseTable()

	for rows.Next() {
		var (
			matchID int64
			ord     int16
			isPick  bool
			heroID  int16
			team    int16
			rTeamID int64
			dTeamID int64
		)
		if err := rows.Scan(&matchID, &ord, &isPick, &heroID, &team, &rTeamID, &dTeamID); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		// New match boundary.
		if matchID != currentMatchID {
			currentMatchID = matchID
			if !processedMatches[matchID] {
				processedMatches[matchID] = true
				matchCount++
			}
			if cfg.Limit > 0 && matchCount > cfg.Limit {
				break
			}
			radiantTeamID = rTeamID
			direTeamID = dTeamID
			radiantPicks = nil
			direPicks = nil
			radiantBans = nil
			direBans = nil
		}

		slot := int(ord)
		if slot >= phases.Len() {
			// Skip slots beyond the standard phase table.
			continue
		}
		phase, _ := phases.At(slot)

		if !isPick {
			// Record the ban and continue.
			if team == 0 {
				radiantBans = append(radiantBans, domain.HeroID(heroID))
			} else {
				direBans = append(direBans, domain.HeroID(heroID))
			}
			continue
		}

		// It's a pick — evaluate the baseline.
		// Determine the user's perspective: "us" is the team making the pick.
		userTeam := domain.SideUs
		if domain.DraftTeam(team) == domain.DraftDire {
			userTeam = domain.SideThem
		}

		// Build DraftState with all picks/bans BEFORE this slot.
		// Look up player rosters from preloaded data (best effort).
		var radiantRoster, direRoster []domain.AccountID
		if sr, ok := rosters[matchID]; ok {
			radiantRoster = sr.radiant
			direRoster = sr.dire
		}
		ds := domain.NewDraftState(
			domain.PatchID(cfg.PatchID),
			userTeam,
			domain.CMPhaseTable(),
			domain.TeamID(radiantTeamID), domain.TeamID(direTeamID),
			radiantRoster, direRoster,
			radiantPicks, direPicks,
			radiantBans, direBans,
			slot,
		)

		legal := ds.LegalHeroes(catalog)
		if len(legal) == 0 {
			continue
		}

		var scores []domain.Score
		if playerBaseline != nil {
			// Use player-aware scoring with roster context.
			roster := ds.Roster()
			s, err := playerBaseline.ScoreWithRoster(ctx, roster, legal)
			if err != nil {
				// Fallback to uniform scores on error.
				scores = uniformScores(legal)
			} else {
				scores = s
			}
		} else {
			scores = baseline.Score(legal)
		}
		sort.Slice(scores, func(i, j int) bool {
			return scores[i].Value > scores[j].Value
		})

		// Find the rank of the actual picked hero.
		actualHero := domain.HeroID(heroID)
		rank := -1
		for i, s := range scores {
			if s.Hero == actualHero {
				rank = i + 1 // 1-based rank
				break
			}
		}

		// Record the pick for subsequent slots (must happen regardless of rank).
		if team == 0 {
			radiantPicks = append(radiantPicks, actualHero)
		} else {
			direPicks = append(direPicks, actualHero)
		}

		// If the hero wasn't in the scored list (shouldn't happen if legal), skip.
		if rank < 0 {
			continue
		}

		// Accumulate metrics for this phase.
		acc, ok := phaseMap[phase.Name]
		if !ok {
			acc = &phaseAcc{}
			phaseMap[phase.Name] = acc
		}
		acc.total++
		if rank == 1 {
			acc.correct++
		}
		acc.sumR1 += computeRecall(rank, 1)
		acc.sumR3 += computeRecall(rank, 3)
		acc.sumR5 += computeRecall(rank, 5)
		acc.sumN10 += computeNDCG10(rank)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	// Build per-phase results.
	phaseNames := make([]string, 0, len(phaseMap))
	for name := range phaseMap {
		phaseNames = append(phaseNames, name)
	}
	sort.Strings(phaseNames)

	perPhase := make([]PhaseMetrics, 0, len(phaseNames))
	for _, name := range phaseNames {
		acc := phaseMap[name]
		n := float64(acc.total)
		pm := PhaseMetrics{
			Phase:   name,
			Total:   acc.total,
			Correct: acc.correct,
		}
		if n > 0 {
			pm.Recall1 = acc.sumR1 / n
			pm.Recall3 = acc.sumR3 / n
			pm.Recall5 = acc.sumR5 / n
			pm.NDCG10 = acc.sumN10 / n
		}
		perPhase = append(perPhase, pm)
	}

	overall := aggregateOverall(perPhase)

	return &BacktestResult{
		PerPhase: perPhase,
		Overall:  overall,
	}, nil
}
