package domain

import core "github.com/user-for-download/go-dota2-core/domain"

// Typed ID primitives re-exported from go-dota2-core.
// Type aliases (=) make these identical types, not just convertible,
// so existing code using domain.HeroID compiles without changes.
type (
	HeroID    = core.HeroID
	TeamID    = core.TeamID
	AccountID = core.AccountID
	PatchID   = core.PatchID
	MatchID   = core.MatchID
)

// HeroIDNone is the zero-value HeroID used to represent "no hero" (no_hero stub).
const HeroIDNone = core.HeroIDNone
