package matchpg

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/user-for-download/go-dota2/internal/storage/matchstore"
)

func (s *Store) upsertPlayers(ctx context.Context, tx pgx.Tx, m matchstore.Match) error {
	var rows [][]any
	for _, p := range m.Players {
		rows = append(rows, []any{
			m.MatchID, p.PlayerSlot, m.StartTime, nullIf0_64(p.AccountID),
			p.HeroID, p.HeroVariant, p.IsRadiant, p.Win, m.Duration,
			nullIf0_32(p.PatchID), p.LobbyType, p.GameMode, p.RankTier,
			p.Kills, p.Deaths, p.Assists, p.Level, p.NetWorth,
			p.Gold, p.GoldSpent, p.GoldPerMin, p.XPPerMin,
			p.LastHits, p.Denies, p.HeroDamage, p.TowerDamage, p.HeroHealing,
			p.Item0, p.Item1, p.Item2, p.Item3, p.Item4, p.Item5, p.ItemNeutral,
			p.Backpack0, p.Backpack1, p.Backpack2, p.Backpack3,
			p.Lane, p.LaneRole, p.IsRoaming, nullIf0_32(p.PartyID), p.PartySize,
			nullIf0_f32(p.Stuns), p.ObsPlaced, p.SenPlaced, p.CreepsStacked, p.CampsStacked,
			p.RunePickups, p.FirstbloodClaimed, nullIf0_f32(p.TeamfightParticipation),
			p.TowersKilled, p.RoshansKilled, p.ObserversPlaced, p.LeaverStatus,
			p.GoldT, p.XPT, p.LHT, p.DNT, p.Times,
			p.ThrowGold, p.ComebackGold, p.LossGold, p.WinGold,
		})
	}

	cols := []string{
		"match_id", "player_slot", "start_time", "account_id",
		"hero_id", "hero_variant", "is_radiant", "win", "duration",
		"patch_id", "lobby_type", "game_mode", "rank_tier",
		"kills", "deaths", "assists", "level", "net_worth",
		"gold", "gold_spent", "gold_per_min", "xp_per_min",
		"last_hits", "denies", "hero_damage", "tower_damage", "hero_healing",
		"item_0", "item_1", "item_2", "item_3", "item_4", "item_5", "item_neutral",
		"backpack_0", "backpack_1", "backpack_2", "backpack_3",
		"lane", "lane_role", "is_roaming", "party_id", "party_size",
		"stuns", "obs_placed", "sen_placed", "creeps_stacked", "camps_stacked",
		"rune_pickups", "firstblood_claimed", "teamfight_participation",
		"towers_killed", "roshans_killed", "observers_placed", "leaver_status",
		"gold_t", "xp_t", "lh_t", "dn_t", "times",
		"throw_gold", "comeback_gold", "loss_gold", "win_gold",
	}

	return bulkUpsert(ctx, tx, "_stage_players", "player_matches", cols, "ON CONFLICT (match_id, player_slot, start_time) DO NOTHING", rows)
}

func (s *Store) upsertPlayerDetails(ctx context.Context, tx pgx.Tx, m matchstore.Match) error {
	var rows [][]any
	for _, pd := range m.Details {
		rows = append(rows, []any{
			m.MatchID, pd.PlayerSlot,
			jsonbOrNull(pd.Damage), jsonbOrNull(pd.DamageTaken), jsonbOrNull(pd.DamageInflictor), jsonbOrNull(pd.DamageInflictorReceived),
			jsonbOrNull(pd.DamageTargets), jsonbOrNull(pd.HeroHits), jsonbOrNull(pd.MaxHeroHit),
			jsonbOrNull(pd.AbilityUses), jsonbOrNull(pd.AbilityTargets), jsonbOrNull(pd.AbilityUpgradesArr),
			jsonbOrNull(pd.ItemUses), jsonbOrNull(pd.GoldReasons), jsonbOrNull(pd.XPReasons), jsonbOrNull(pd.Killed), jsonbOrNull(pd.KilledBy),
			jsonbOrNull(pd.KillStreaks), jsonbOrNull(pd.MultiKills), jsonbOrNull(pd.LifeState), jsonbOrNull(pd.LanePos), jsonbOrNull(pd.Obs), jsonbOrNull(pd.Sen),
			jsonbOrNull(pd.Actions), jsonbOrNull(pd.Pings), jsonbOrNull(pd.Runes), jsonbOrNull(pd.Purchase),
			jsonbOrNull(pd.ObsLog), jsonbOrNull(pd.SenLog), jsonbOrNull(pd.ObsLeftLog), jsonbOrNull(pd.SenLeftLog),
			jsonbOrNull(pd.PurchaseLog), jsonbOrNull(pd.KillsLog), jsonbOrNull(pd.BuybackLog), jsonbOrNull(pd.RunesLog),
			jsonbOrNull(pd.ConnectionLog), jsonbOrNull(pd.PermanentBuffs), jsonbOrNull(pd.NeutralTokensLog),
			jsonbOrNull(pd.NeutralItemHistory), jsonbOrNull(pd.AdditionalUnits), jsonbOrNull(pd.Cosmetics),
			jsonbOrNull(pd.Benchmarks), jsonbOrNull(pd.AllWordCounts), jsonbOrNull(pd.MyWordCounts),
		})
	}

	cols := []string{
		"match_id", "player_slot",
		"damage", "damage_taken", "damage_inflictor", "damage_inflictor_received",
		"damage_targets", "hero_hits", "max_hero_hit",
		"ability_uses", "ability_targets", "ability_upgrades_arr",
		"item_uses", "gold_reasons", "xp_reasons", "killed", "killed_by",
		"kill_streaks", "multi_kills", "life_state", "lane_pos", "obs", "sen",
		"actions", "pings", "runes", "purchase",
		"obs_log", "sen_log", "obs_left_log", "sen_left_log",
		"purchase_log", "kills_log", "buyback_log", "runes_log",
		"connection_log", "permanent_buffs", "neutral_tokens_log",
		"neutral_item_history", "additional_units", "cosmetics",
		"benchmarks", "all_word_counts", "my_word_counts",
	}

	return bulkUpsert(ctx, tx, "_stage_player_details", "player_match_details", cols, "ON CONFLICT (match_id, player_slot) DO NOTHING", rows)
}

func (s *Store) upsertTimeseries(ctx context.Context, tx pgx.Tx, m matchstore.Match) error {
	var rows [][]any
	for _, ts := range m.Timeseries {
		rows = append(rows, []any{
			m.MatchID, ts.PlayerSlot, m.StartTime, ts.Minute, ts.HeroID, nullIf0_64(ts.AccountID), nullIf0_32(ts.PatchID),
			ts.Gold, ts.XP, ts.LH, ts.DN,
		})
	}

	cols := []string{
		"match_id", "player_slot", "start_time", "minute", "hero_id", "account_id", "patch_id",
		"gold", "xp", "lh", "dn",
	}

	return bulkUpsert(ctx, tx, "_stage_timeseries", "player_timeseries", cols, "ON CONFLICT (match_id, player_slot, minute, start_time) DO NOTHING", rows)
}