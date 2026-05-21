package domain

import (
	"testing"
)

func TestHeroIDNone(t *testing.T) {
	if HeroIDNone != 0 {
		t.Errorf("HeroIDNone = %d, want 0", HeroIDNone)
	}
}

func TestHeroIDConversions(t *testing.T) {
	// Verify typed IDs behave as expected at boundary.
	h := HeroID(1)
	if int16(h) != 1 {
		t.Errorf("HeroID(1) converted to int16 = %d, want 1", int16(h))
	}
}

func TestMatchID(t *testing.T) {
	m := MatchID(12345)
	if int64(m) != 12345 {
		t.Errorf("MatchID(12345) = %d, want 12345", int64(m))
	}
}

func TestTeamID(t *testing.T) {
	team := TeamID(999)
	if int64(team) != 999 {
		t.Errorf("TeamID(999) = %d, want 999", int64(team))
	}
}

func TestPatchID(t *testing.T) {
	p := PatchID(42)
	if int32(p) != 42 {
		t.Errorf("PatchID(42) = %d, want 42", int32(p))
	}
}

func TestAccountID(t *testing.T) {
	a := AccountID(76561198000000000)
	if int64(a) != 76561198000000000 {
		t.Errorf("AccountID = %d, want %d", int64(a), 76561198000000000)
	}
}
