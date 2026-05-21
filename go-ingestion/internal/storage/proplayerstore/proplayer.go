package proplayerstore

import "context"
import "time"

type ProPlayer struct {
	AccountID       int64      `json:"account_id"`
	SteamID         *string    `json:"steamid"`
	Personaname     *string    `json:"personaname"`
	Name            *string    `json:"name"`
	CountryCode     *string    `json:"country_code"`
	FantasyRole     *int       `json:"fantasy_role"`
	TeamID          *int64     `json:"team_id"`
	TeamName        *string    `json:"team_name"`
	TeamTag         *string    `json:"team_tag"`
	IsPro           *bool      `json:"is_pro"`
	IsLocked        *bool      `json:"is_locked"`
	Avatar          *string    `json:"avatar"`
	LastMatchTime   *time.Time `json:"last_match_time"`
	LastLogin       *time.Time `json:"last_login"`
	FullHistoryTime *time.Time `json:"full_history_time"`
	Cheese          *int       `json:"cheese"`
	FhUnavailable   *bool      `json:"fh_unavailable"`
	LocCountryCode  *string    `json:"loccountrycode"`
	Plus            *bool      `json:"plus"`
}

type Writer interface {
	Upsert(ctx context.Context, players []ProPlayer) (int, error)
}