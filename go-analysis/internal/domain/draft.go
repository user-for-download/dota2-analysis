package domain

import "slices"

// Side represents the user's perspective in a match.
type Side int8

const (
	SideUs Side = iota
	SideThem
)

func (s Side) String() string {
	switch s {
	case SideUs:
		return "us"
	case SideThem:
		return "them"
	default:
		return "unknown"
	}
}

// DraftTeam identifies which side of the draft is acting.
type DraftTeam int8

const (
	DraftRadiant DraftTeam = 0
	DraftDire    DraftTeam = 1
)

func (t DraftTeam) String() string {
	switch t {
	case DraftRadiant:
		return "radiant"
	case DraftDire:
		return "dire"
	default:
		return "unknown"
	}
}

// Phase describes a single step in the Captain's Mode draft order.
type Phase struct {
	Name       string
	IsBan      bool
	ActingTeam DraftTeam
}

// teamSlot holds the state for one side of the draft (opaque within DraftState).
type teamSlot struct {
	TeamID TeamID
	Roster []AccountID
	Picks  []HeroID
	Bans   []HeroID
}

// DraftState captures the state of a CM draft at a specific slot.
// It is opaque — construct via NewDraftState, read via methods only.
type DraftState struct {
	patch    PatchID
	userTeam Side
	phases   PhaseTable
	us       teamSlot
	them     teamSlot
	slot     int
	drafted  map[HeroID]bool
}

// NewDraftState creates an immutable snapshot of a draft at the given slot.
func NewDraftState(
	patch PatchID,
	userTeam Side,
	phases PhaseTable,
	radiantTeamID, direTeamID TeamID,
	radiantRoster, direRoster []AccountID,
	radiantPicks, direPicks, radiantBans, direBans []HeroID,
	slot int,
) *DraftState {
	ds := &DraftState{
		patch:    patch,
		userTeam: userTeam,
		phases:   phases,
		slot:     slot,
		drafted:  make(map[HeroID]bool),
	}

	if userTeam == SideUs {
		ds.us = teamSlot{
			TeamID: radiantTeamID,
			Roster: slices.Clone(radiantRoster),
			Picks:  slices.Clone(radiantPicks),
			Bans:   slices.Clone(radiantBans),
		}
		ds.them = teamSlot{
			TeamID: direTeamID,
			Roster: slices.Clone(direRoster),
			Picks:  slices.Clone(direPicks),
			Bans:   slices.Clone(direBans),
		}
	} else {
		ds.us = teamSlot{
			TeamID: direTeamID,
			Roster: slices.Clone(direRoster),
			Picks:  slices.Clone(direPicks),
			Bans:   slices.Clone(direBans),
		}
		ds.them = teamSlot{
			TeamID: radiantTeamID,
			Roster: slices.Clone(radiantRoster),
			Picks:  slices.Clone(radiantPicks),
			Bans:   slices.Clone(radiantBans),
		}
	}

	// Build the drafted set from all picks and bans.
	for _, h := range ds.us.Picks {
		ds.drafted[h] = true
	}
	for _, h := range ds.them.Picks {
		ds.drafted[h] = true
	}
	for _, h := range ds.us.Bans {
		ds.drafted[h] = true
	}
	for _, h := range ds.them.Bans {
		ds.drafted[h] = true
	}

	return ds
}

// Patch returns the patch ID for this draft.
func (d *DraftState) Patch() PatchID { return d.patch }

// Slot returns the current phase slot index.
func (d *DraftState) Slot() int { return d.slot }

// TeamID returns the user's team ID.
func (d *DraftState) TeamID() TeamID { return d.us.TeamID }

// ThemTeamID returns the opposing team's ID.
func (d *DraftState) ThemTeamID() TeamID { return d.them.TeamID }

// Phase returns the current draft phase, or a zero-value Phase if complete.
func (d *DraftState) Phase() Phase {
	p, ok := d.phases.At(d.slot)
	if !ok {
		return Phase{}
	}
	return p
}

// NextActor returns the team that acts in the current phase.
func (d *DraftState) NextActor() DraftTeam { return d.Phase().ActingTeam }

// IsBanPhase reports whether the current phase is a ban.
func (d *DraftState) IsBanPhase() bool { return d.Phase().IsBan }

// AllyPicks returns a copy of the user's team's picked heroes.
func (d *DraftState) AllyPicks() []HeroID { return slices.Clone(d.us.Picks) }

// EnemyPicks returns a copy of the opposing team's picked heroes.
func (d *DraftState) EnemyPicks() []HeroID { return slices.Clone(d.them.Picks) }

// AllyBans returns a copy of the user's team's banned heroes.
func (d *DraftState) AllyBans() []HeroID { return slices.Clone(d.us.Bans) }

// Roster returns a copy of the user's team's player roster (account IDs).
func (d *DraftState) Roster() []AccountID { return slices.Clone(d.us.Roster) }

// IsDraftComplete reports whether all draft slots have been processed.
func (d *DraftState) IsDraftComplete() bool { return d.slot >= d.phases.Len() }

// LegalHeroes returns all heroes from the catalog that have not been drafted.
func (d *DraftState) LegalHeroes(cat HeroCatalog) []HeroID {
	var legal []HeroID
	cat.EachHero(func(id HeroID) bool {
		if id != HeroIDNone && !d.drafted[id] { // Exclude no_hero stub
			legal = append(legal, id)
		}
		return true
	})
	return legal
}
