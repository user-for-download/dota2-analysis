package parser

import (
	"testing"
)

func TestDecodeAll(t *testing.T) {
	payload := []byte(`{
		"match_id": 123,
		"start_time": 1704067200,
		"duration": 1800,
		"game_mode": 22,
		"picks_bans": [
			{"is_pick": true, "hero_id": 1, "team": 0, "order": 1, "ord": 1}
		],
		"draft_timings": [
			{"order": 1, "pick": true, "active_team": 0, "hero_id": 1, "player_slot": 1, "extra_time": 10, "total_time_taken": 20}
		],
		"objectives": [
			{"time": 100, "type": "CHAT_MESSAGE_FIRSTBLOOD", "slot": 2, "key": "3", "player_slot": 2}
		],
		"chat": [
			{"time": 200, "type": "chat", "key": "gg", "slot": 1, "player_slot": 1}
		],
		"radiant_gold_adv": [0, 100, 200],
		"radiant_xp_adv": [0, 50, 100],
		"teamfights": [
			{"start": 300, "end": 350, "last_death": 340, "deaths": 2, "players": [
				{"deaths_pos": {"1": 10}, "ability_uses": {"antimage_blink": 2}, "item_uses": {"tango": 1}, "killed": {"npc_dota_hero_puck": 1}, "deaths": 1, "buybacks": 0, "damage": 1000, "healing": 0, "gold_delta": 50, "xp_delta": 100, "xp_start": 0, "xp_end": 100}
			]}
		],
		"players": [
			{
				"player_slot": 1,
				"hero_id": 1,
				"account_id": 12345,
				"isRadiant": true,
				"kills": 5,
				"deaths": 2,
				"assists": 10,
				"last_hits": 100,
				"denies": 5,
				"gold_per_min": 500,
				"xp_per_min": 600,
				"level": 15,
				"hero_damage": 10000,
				"tower_damage": 2000,
				"hero_healing": 500,
				"gold": 1500,
				"gold_spent": 8000,
				"item_0": 1,
				"item_1": 2,
				"item_2": 3,
				"item_3": 4,
				"item_4": 5,
				"item_5": 6,
				"item_neutral": 7,
				"ability_upgrades_arr": [1, 2, 3],
				"times": [0, 60, 120],
				"gold_t": [0, 100, 200],
				"lh_t": [0, 2, 5],
				"dn_t": [0, 0, 1],
				"xp_t": [0, 100, 300],
				"killed": {"npc_dota_hero_puck": 2},
				"killed_by": {"npc_dota_hero_lina": 1},
				"multi_kills": {"2": 1},
				"kill_streaks": {"3": 1},
				"lane_pos": {"1": 10},
				"obs_log": [{"time": 10, "x": 100, "y": 100}],
				"sen_log": [{"time": 20, "x": 110, "y": 110}],
				"runes_log": [{"time": 30, "key": 2}],
				"kills_log": [{"time": 40, "key": "npc_dota_hero_puck"}],
				"buyback_log": [{"time": 50, "slot": 1}],
				"purchase_log": [{"time": 60, "key": "tango"}],
				"connection_log": [{"time": 70, "event": "connected"}],
				"obs_left_log": [{"time": 80, "x": 100, "y": 100}],
				"sen_left_log": [{"time": 90, "x": 110, "y": 110}],
				"damage_targets": {"npc_dota_hero_puck": {"attack": 500}}
			}
		]
	}`)
	match, err := decodeMatch(123, payload)
	if err != nil {
		t.Fatalf("decodeMatch: %v", err)
	}
	if len(match.PicksBans) != 1 {
		t.Errorf("PicksBans = %d, want 1", len(match.PicksBans))
	}
	if len(match.DraftTimings) != 1 {
		t.Errorf("DraftTimings = %d, want 1", len(match.DraftTimings))
	}
	if len(match.Objectives) != 1 {
		t.Errorf("Objectives = %d, want 1", len(match.Objectives))
	}
	if len(match.Chat) != 1 {
		t.Errorf("Chat = %d, want 1", len(match.Chat))
	}
	if len(match.Teamfights) != 1 {
		t.Errorf("Teamfights = %d, want 1", len(match.Teamfights))
	}
	if len(match.Players) != 1 {
		t.Errorf("Players = %d, want 1", len(match.Players))
	}
	if match.Advantages.RadiantGoldAdv[2] != 200 {
		t.Errorf("RadiantGoldAdv = %v", match.Advantages.RadiantGoldAdv)
	}
}

func TestMismatchedTimeLengths(t *testing.T) {
	payload := []byte(`{
		"match_id": 124,
		"players": [
			{
				"player_slot": 1,
				"times": [0, 60],
				"gold_t": [0, 100, 200]
			}
		]
	}`)
	_, err := decodeMatch(124, payload)
	if err == nil {
		t.Fatalf("expected error on mismatched time lengths, got nil")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		wantErr bool
	}{
		{"invalid match_id", `{"match_id": 0}`, true},
		{"missing start_time", `{"match_id": 123}`, true},
		{"invalid player_slot", `{"match_id": 123, "start_time": 1704067200, "players": [{"player_slot": 99}]}`, true},
		{"valid", `{"match_id": 123, "start_time": 1704067200, "players": [{"player_slot": 1}]}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, err := decodeMatch(123, []byte(c.payload))
			if err == nil {
				err = validate(m)
			}
			if (err != nil) != c.wantErr {
				t.Errorf("expected err=%v, got %v", c.wantErr, err)
			}
		})
	}
}
