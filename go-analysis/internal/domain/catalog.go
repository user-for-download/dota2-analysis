package domain

// Role represents a hero role classification.
type Role int

const (
	RoleCarry Role = iota + 1
	RoleSupport
	RoleNuker
	RoleDisabler
	RoleDurable
	RoleEscape
	RolePusher
	RoleInitiator
)

// HeroInfo contains metadata about a hero.
type HeroInfo struct {
	ID          HeroID
	Name        string
	Roles       []Role
	PrimaryAttr string // "str", "agi", "int", "all"
}

// HeroCatalog provides read-only access to hero metadata.
type HeroCatalog interface {
	// Name returns the hero's name (e.g. "npc_dota_hero_axe").
	Name(HeroID) string
	// Info returns full hero metadata, or false if not found.
	Info(HeroID) (HeroInfo, bool)
	// Roles returns the roles assigned to a hero.
	Roles(HeroID) []Role
	// All returns all hero IDs in the catalog.
	All() []HeroID
	// EachHero iterates over all hero IDs; return false to stop early.
	EachHero(f func(HeroID) bool)
}
