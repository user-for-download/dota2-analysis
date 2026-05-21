// Package domain provides shared typed-ID primitives for Dota 2 domain entities.
//
// These types replace raw int64/int16 at public API boundaries across
// all go-dota2-* projects. Use type aliases in downstream projects
// to adopt them without cascading breakage:
//
//	type HeroID = core.HeroID
package domain

// Typed ID primitives for domain entities.
type (
	HeroID    int16
	TeamID    int64
	AccountID int64
	PatchID   int32
	MatchID   int64
)

// HeroIDNone is the zero-value HeroID used to represent "no hero" (no_hero stub).
const HeroIDNone HeroID = 0
