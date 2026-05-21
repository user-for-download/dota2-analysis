package features

import (
	"testing"

	"github.com/user-for-download/go-dota2-analysis/internal/domain"
)

// inMemoryCatalog implements domain.HeroCatalog for tests.
type inMemoryCatalog struct {
	heroes map[domain.HeroID]domain.HeroInfo
}

func (c *inMemoryCatalog) Name(id domain.HeroID) string {
	if h, ok := c.heroes[id]; ok {
		return h.Name
	}
	return ""
}

func (c *inMemoryCatalog) Info(id domain.HeroID) (domain.HeroInfo, bool) {
	h, ok := c.heroes[id]
	return h, ok
}

func (c *inMemoryCatalog) Roles(id domain.HeroID) []domain.Role {
	if h, ok := c.heroes[id]; ok {
		return h.Roles
	}
	return nil
}

func (c *inMemoryCatalog) All() []domain.HeroID {
	out := make([]domain.HeroID, 0, len(c.heroes))
	for id := range c.heroes {
		out = append(out, id)
	}
	return out
}

func (c *inMemoryCatalog) EachHero(f func(domain.HeroID) bool) {
	for id := range c.heroes {
		if !f(id) {
			return
		}
	}
}

func TestHeroMetaFeatures(t *testing.T) {
	cat := &inMemoryCatalog{
		heroes: map[domain.HeroID]domain.HeroInfo{
			1: {Name: "str_hero", PrimaryAttr: "str", Roles: []domain.Role{domain.RoleCarry, domain.RoleDurable}},
			2: {Name: "agi_hero", PrimaryAttr: "agi", Roles: []domain.Role{domain.RoleCarry}},
			3: {Name: "int_hero", PrimaryAttr: "int", Roles: []domain.Role{domain.RoleSupport, domain.RoleDisabler, domain.RoleNuker}},
			4: {Name: "all_hero", PrimaryAttr: "all", Roles: nil},
		},
	}

	tests := []struct {
		name    string
		heroID  domain.HeroID
		wantAttr float64
		wantRoles float64
	}{
		{"str hero", 1, 1, 2},
		{"agi hero", 2, 2, 1},
		{"int hero", 3, 3, 3},
		{"all hero", 4, 4, 0},
	}

	result, err := HeroMetaFeatures(cat, []domain.HeroID{1, 2, 3, 4})
	if err != nil {
		t.Fatalf("HeroMetaFeatures failed: %v", err)
	}

	for _, tt := range tests {
		got, ok := result[tt.heroID]
		if !ok {
			t.Errorf("%s: missing from result", tt.name)
			continue
		}
		if len(got) != 2 {
			t.Errorf("%s: got %d values, want 2", tt.name, len(got))
			continue
		}
		if got[0] != tt.wantAttr {
			t.Errorf("%s: attr = %f, want %f", tt.name, got[0], tt.wantAttr)
		}
		if got[1] != tt.wantRoles {
			t.Errorf("%s: role count = %f, want %f", tt.name, got[1], tt.wantRoles)
		}
	}
}

func TestHeroMetaFeaturesUnknown(t *testing.T) {
	cat := &inMemoryCatalog{heroes: make(map[domain.HeroID]domain.HeroInfo)}
	result, err := HeroMetaFeatures(cat, []domain.HeroID{99})
	if err != nil {
		t.Fatalf("HeroMetaFeatures failed: %v", err)
	}
	got, ok := result[99]
	if !ok {
		t.Fatal("missing result for unknown hero")
	}
	if got[0] != 0 || got[1] != 0 {
		t.Errorf("unknown hero: got [%f %f], want [0 0]", got[0], got[1])
	}
}

func TestHeroMetaFeaturesEmpty(t *testing.T) {
	cat := &inMemoryCatalog{heroes: make(map[domain.HeroID]domain.HeroInfo)}
	result, err := HeroMetaFeatures(cat, nil)
	if err != nil {
		t.Fatalf("HeroMetaFeatures failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d entries", len(result))
	}
}
