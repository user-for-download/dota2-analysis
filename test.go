package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type PickBan struct {
	IsPick bool  `json:"is_pick"`
	HeroID int   `json:"hero_id"`
	Team   int16 `json:"team"`
	Order  int   `json:"order"`
}

type MatchResponse struct {
	MatchID   int64     `json:"match_id"`
	GameMode  int       `json:"game_mode"`
	PicksBans []PickBan `json:"picks_bans"`
}

func computeSequenceHash(pbs []PickBan) string {
	h := sha256.New()
	for _, pb := range pbs {
		action := "ban"
		if pb.IsPick {
			action = "pick"
		}
		h.Write([]byte(fmt.Sprintf("%d:%s|", pb.Team, action)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func fetchMatch(matchID int64) (*MatchResponse, error) {
	url := fmt.Sprintf("https://api.opendota.com/api/matches/%d", matchID)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}

	var match MatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&match); err != nil {
		return nil, err
	}
	return &match, nil
}

func main() {
	matches := []struct {
		patch   string
		matchID int64
	}{
		{"7.41", 8829950213},
		{"7.40", 8744085947},
		{"7.39", 8607145322},
		{"7.38", 8303667672},
		{"7.37", 8184994140},
		{"7.36", 7886224634},
		{"7.35", 7750880089},
		{"7.34", 7506512389},
		{"7.33", 7276549518},
		{"7.32", 7116420936},
		{"7.31", 6721399295},
		{"7.30", 6446970363},
	}

	ctx := context.Background()
	_ = ctx

	fmt.Println("// Add these to knownDraftSchemaHashes:")
	fmt.Println()

	for _, m := range matches {
		fmt.Printf("// fetching %s (match %d)\n", m.patch, m.matchID)
		match, err := fetchMatch(m.matchID)
		if err != nil {
			fmt.Printf("// ERROR fetching %s (match %d): %v\n", m.patch, m.matchID, err)
			time.Sleep(1 * time.Second)
			continue
		}

		if match.GameMode != 2 {
			fmt.Printf("// SKIP %s (match %d): not Captain's Mode (game_mode=%d)\n",
				m.patch, m.matchID, match.GameMode)
			time.Sleep(1 * time.Second)
			continue
		}

		if len(match.PicksBans) == 0 {
			fmt.Printf("// SKIP %s (match %d): no picks_bans data\n", m.patch, m.matchID)
			time.Sleep(1 * time.Second)
			continue
		}

		hash := computeSequenceHash(match.PicksBans)
		fmt.Printf("\"cm_%s\": \"%s\", // match %d, %d picks/bans\n",
			m.patch[2:], hash, m.matchID, match.PicksBans)

		// Rate limit to avoid hitting OpenDota API limits
		time.Sleep(1 * time.Second)
	}
}
