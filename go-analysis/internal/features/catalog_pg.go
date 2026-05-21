package features

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/user-for-download/go-dota2-analysis/internal/domain"
)

// PGCatalog implements domain.HeroCatalog by loading all heroes from
// public.heroes at startup and serving reads from memory.
type PGCatalog struct {
	byID   map[domain.HeroID]domain.HeroInfo
	byName map[string]domain.HeroID
	all    []domain.HeroID
}

// NewPGCatalog queries all heroes from public.heroes and caches them.
func NewPGCatalog(ctx context.Context, db *pgxpool.Pool) (*PGCatalog, error) {
	rows, err := db.Query(ctx, `
		SELECT id, name, primary_attr, roles
		FROM public.heroes
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("query heroes: %w", err)
	}
	defer rows.Close()

	byID := make(map[domain.HeroID]domain.HeroInfo)
	byName := make(map[string]domain.HeroID)
	var all []domain.HeroID

	for rows.Next() {
		var (
			id          int16
			name        string
			primaryAttr *string
			roles       []string
		)
		if err := rows.Scan(&id, &name, &primaryAttr, &roles); err != nil {
			return nil, fmt.Errorf("scan hero %d: %w", id, err)
		}

		attr := ""
		if primaryAttr != nil {
			attr = *primaryAttr
		}

		info := domain.HeroInfo{
			ID:          domain.HeroID(id),
			Name:        name,
			PrimaryAttr: attr,
			Roles:       parseRoles(roles),
		}
		byID[info.ID] = info
		byName[name] = info.ID
		all = append(all, info.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate heroes: %w", err)
	}

	return &PGCatalog{
		byID:   byID,
		byName: byName,
		all:    all,
	}, nil
}

func (c *PGCatalog) Name(id domain.HeroID) string {
	if info, ok := c.byID[id]; ok {
		return info.Name
	}
	return ""
}

func (c *PGCatalog) Info(id domain.HeroID) (domain.HeroInfo, bool) {
	info, ok := c.byID[id]
	return info, ok
}

func (c *PGCatalog) Roles(id domain.HeroID) []domain.Role {
	if info, ok := c.byID[id]; ok {
		return slices.Clone(info.Roles)
	}
	return nil
}

func (c *PGCatalog) All() []domain.HeroID {
	return slices.Clone(c.all)
}

func (c *PGCatalog) EachHero(f func(domain.HeroID) bool) {
	for _, id := range c.all {
		if !f(id) {
			break
		}
	}
}

var roleMap = map[string]domain.Role{
	"Carry":     domain.RoleCarry,
	"Support":   domain.RoleSupport,
	"Nuker":     domain.RoleNuker,
	"Initiator": domain.RoleInitiator,
	"Disabler":  domain.RoleDisabler,
	"Escape":    domain.RoleEscape,
	"Durable":   domain.RoleDurable,
	"Pusher":    domain.RolePusher,
}

func parseRoles(raw []string) []domain.Role {
	roles := make([]domain.Role, 0, len(raw))
	for _, r := range raw {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if role, ok := roleMap[r]; ok {
			roles = append(roles, role)
		}
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i] < roles[j] })
	return roles
}
