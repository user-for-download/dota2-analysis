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

	// Query patch date range to enable partition pruning on player_matches
	// (partitioned by start_time).  Without a start_time filter, PG probes
	// every quarterly partition — a full sequential scan of millions of rows.
	var patchMinTS, patchMaxTS int64
	if err := db.QueryRow(ctx, `
		SELECT COALESCE(EXTRACT(EPOCH FROM release_at)::bigint, 0) AS min_ts,
		       COALESCE(EXTRACT(EPOCH FROM LEAD(release_at) OVER (ORDER BY release_at))::bigint - 1, 2147483647) AS max_ts
		FROM patches WHERE id = $1
	`, cfg.PatchID).Scan(&patchMinTS, &patchMaxTS); err != nil {
		slog.Default().Warn("patch range query failed; partition pruning disabled", "err", err)
	}

	rosterRows, err := db.Query(ctx, `
		SELECT match_id, is_radiant,
		       COALESCE(array_remove(array_agg(account_id ORDER BY player_slot), NULL), ARRAY[]::bigint[]) AS players
		FROM public.player_matches
		WHERE patch_id = $1
		  AND start_time BETWEEN $2 AND $3  -- partition pruning
		GROUP BY match_id, is_radiant
	`, cfg.PatchID, patchMinTS, patchMaxTS)
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
			// array_remove filters out NULL account_ids (anonymous players), so
			// a plain []int64 is safe — no nil pointers to worry about.
			// COALESCE(..., ARRAY[]::bigint[]) handles the edge case where the
			// entire array is NULL (all account_ids in the group were NULL).
			var accountIDs []int64
			if err := rosterRows.Scan(&matchID, &isRadiant, &accountIDs); err != nil {
				continue
			}
			sr, ok := rosters[matchID]
			if !ok {
				sr = &sideRoster{}
				rosters[matchID] = sr
			}
			ids := make([]domain.AccountID, len(accountIDs))
			for i, id := range accountIDs {
				ids[i] = domain.AccountID(id)
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
	type matchState struct {
		radiantTeamID int64
		direTeamID    int64
		radiantPicks  []domain.HeroID
		direPicks     []domain.HeroID
		radiantBans   []domain.HeroID
		direBans      []domain.HeroID
		phaseTable    domain.PhaseTable
		phases        []struct {
			ord    int16
			isPick bool
			heroID int16
			team   int16
		}
	}
	var (
		matchStateMap = make(map[int64]*matchState)
		matchOrder    []int64
		matchCount    int
	)

	// First pass: collect all rows per match so we can derive the phase table
	// from the is_pick sequence rather than assuming ord maps 1:1 to CMPhaseTable.
	// When bans are declined, ord values shift — using raw ord corrupts the
	// backtest by looking up the wrong phase (e.g. a pick at ord=15 may actually
	// be a Radiant pick, but CMPhaseTable.At(15) expects a Dire ban).
	type rawRow struct {
		matchID int64
		ord     int16
		isPick  bool
		heroID  int16
		team    int16
		rTeamID int64
		dTeamID int64
	}
	var allRows []rawRow

	for rows.Next() {
		var r rawRow
		if err := rows.Scan(&r.matchID, &r.ord, &r.isPick, &r.heroID, &r.team, &r.rTeamID, &r.dTeamID); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		allRows = append(allRows, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	// Build per-match state from collected rows.
	for _, r := range allRows {
		if cfg.Limit > 0 && matchCount > cfg.Limit {
			break
		}

		ms, ok := matchStateMap[r.matchID]
		if !ok {
			matchCount++
			ms = &matchState{
				radiantTeamID: r.rTeamID,
				direTeamID:    r.dTeamID,
			}
			matchStateMap[r.matchID] = ms
			matchOrder = append(matchOrder, r.matchID)
		}
		ms.phases = append(ms.phases, struct {
			ord    int16
			isPick bool
			heroID int16
			team   int16
		}{ord: r.ord, isPick: r.isPick, heroID: r.heroID, team: r.team})
	}

	// Derive phase tables from isPick sequences (handles declined bans).
	for _, ms := range matchStateMap {
		isPickSeq := make([]bool, len(ms.phases))
		for i, p := range ms.phases {
			isPickSeq[i] = p.isPick
		}
		ms.phaseTable = domain.DerivePhaseTable(isPickSeq)
	}

	// Second pass: process each match's rows using the derived phase table.
	for _, matchID := range matchOrder {
		ms := matchStateMap[matchID]

		radiantPicks := ms.radiantPicks
		direPicks := ms.direPicks
		radiantBans := ms.radiantBans
		direBans := ms.direBans

		for localIdx, p := range ms.phases {
			phase, ok := ms.phaseTable.At(localIdx)
			if !ok {
				continue
			}

			if !p.isPick {
				// Record the ban and continue.
				if p.team == 0 {
					radiantBans = append(radiantBans, domain.HeroID(p.heroID))
				} else {
					direBans = append(direBans, domain.HeroID(p.heroID))
				}
				continue
			}

			// It's a pick — evaluate the baseline.
			userTeam := domain.SideUs
			if domain.DraftTeam(p.team) == domain.DraftDire {
				userTeam = domain.SideThem
			}

			var radiantRoster, direRoster []domain.AccountID
			if sr, ok := rosters[matchID]; ok {
				radiantRoster = sr.radiant
				direRoster = sr.dire
			}
			ds := domain.NewDraftState(
				domain.PatchID(cfg.PatchID),
				userTeam,
				ms.phaseTable,
				domain.TeamID(ms.radiantTeamID), domain.TeamID(ms.direTeamID),
				radiantRoster, direRoster,
				radiantPicks, direPicks,
				radiantBans, direBans,
				localIdx,
			)

			legal := ds.LegalHeroes(catalog)
			if len(legal) == 0 {
				continue
			}

			var scores []domain.Score
			if playerBaseline != nil {
				roster := ds.Roster()
				s, err := playerBaseline.ScoreWithRoster(ctx, roster, legal)
				if err != nil {
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

			actualHero := domain.HeroID(p.heroID)
			rank := -1
			for i, s := range scores {
				if s.Hero == actualHero {
					rank = i + 1
					break
				}
			}

			if p.team == 0 {
				radiantPicks = append(radiantPicks, actualHero)
			} else {
				direPicks = append(direPicks, actualHero)
			}

			if rank < 0 {
				continue
			}

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
