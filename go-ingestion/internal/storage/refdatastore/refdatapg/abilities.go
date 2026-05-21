package refdatapg

import (
	"context"
	"fmt"

	"github.com/user-for-download/go-dota2/internal/storage/refdatastore"
)

func (s *Store) UpsertAbilities(ctx context.Context, a []refdatastore.AbilityRef) error {
	if len(a) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO abilities (key, dname, behavior, target_team, description, img, mana_cost, cooldown, attrib, is_talent, updated_at)
		VALUES ($1, $2, $3::jsonb, $4, $5, $6, $7, $8, $9::jsonb, $10, NOW())
		ON CONFLICT (key) DO UPDATE SET
			dname = EXCLUDED.dname, behavior = EXCLUDED.behavior, target_team = EXCLUDED.target_team,
			description = EXCLUDED.description, img = EXCLUDED.img, mana_cost = EXCLUDED.mana_cost,
			cooldown = EXCLUDED.cooldown, attrib = EXCLUDED.attrib, is_talent = EXCLUDED.is_talent, updated_at = NOW()`

	for _, ab := range a {
		if ab.Name == "" {
			continue
		}
		if _, err := tx.Exec(ctx, q,
			ab.Name, ab.Localized, jsonbOrNull(ab.Behavior), ab.TargetTeam, ab.Description,
			ab.Img, ab.ManaCost, ab.Cooldown, jsonbOrNull(ab.Attrib), ab.IsTalent,
		); err != nil {
			return fmt.Errorf("upsert ability %q: %w", ab.Name, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	s.log.Info("abilities upserted", "count", len(a))
	return nil
}

func (s *Store) UpsertAbilityIDs(ctx context.Context, ai []refdatastore.AbilityIDRef) error {
	if len(ai) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO abilities (key, id, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (key) DO UPDATE SET id = EXCLUDED.id, updated_at = NOW()`

	for _, idRef := range ai {
		if idRef.Name == "" || idRef.ID <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx, q, idRef.Name, idRef.ID); err != nil {
			return fmt.Errorf("upsert ability id %d (%s): %w", idRef.ID, idRef.Name, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	s.log.Info("ability ids upserted", "count", len(ai))
	return nil
}

func (s *Store) UpsertHeroAbilities(ctx context.Context, ha []refdatastore.HeroAbilityRef) error {
	if len(ha) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO hero_abilities (hero_name, slot, ability)
		VALUES ($1, $2, $3)
		ON CONFLICT (hero_name, slot) DO UPDATE SET ability = EXCLUDED.ability`

	for _, h := range ha {
		if h.HeroName == "" || h.Ability == "" {
			continue
		}
		if _, err := tx.Exec(ctx, q, h.HeroName, h.Slot, h.Ability); err != nil {
			return fmt.Errorf("upsert hero ability %s/%d: %w", h.HeroName, h.Slot, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	s.log.Info("hero_abilities upserted", "count", len(ha))
	return nil
}

func (s *Store) UpsertHeroTalents(ctx context.Context, ht []refdatastore.HeroTalentRef) error {
	if len(ht) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO hero_talents (hero_name, ability, level)
		VALUES ($1, $2, $3)
		ON CONFLICT (hero_name, ability) DO UPDATE SET level = EXCLUDED.level`

	for _, t := range ht {
		if t.HeroName == "" || t.Ability == "" || t.Level < 1 || t.Level > 4 {
			continue
		}
		if _, err := tx.Exec(ctx, q, t.HeroName, t.Ability, t.Level); err != nil {
			return fmt.Errorf("upsert hero talent %s/%s: %w", t.HeroName, t.Ability, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	s.log.Info("hero_talents upserted", "count", len(ht))
	return nil
}

func (s *Store) UpsertHeroFacets(ctx context.Context, hf []refdatastore.HeroFacetRef) error {
	if len(hf) == 0 {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		INSERT INTO hero_facets (hero_name, slot, name, title, description, icon, color, gradient_id, deprecated)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (hero_name, slot) DO UPDATE SET
			name = EXCLUDED.name, title = EXCLUDED.title, description = EXCLUDED.description,
			icon = EXCLUDED.icon, color = EXCLUDED.color, gradient_id = EXCLUDED.gradient_id, deprecated = EXCLUDED.deprecated`

	for _, f := range hf {
		if f.HeroName == "" || f.Name == "" {
			continue
		}
		if _, err := tx.Exec(ctx, q,
			f.HeroName, f.Slot, f.Name, f.Title, f.Description,
			f.Icon, f.Color, f.GradientID, f.Deprecated,
		); err != nil {
			return fmt.Errorf("upsert hero facet %s/%d: %w", f.HeroName, f.Slot, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	s.log.Info("hero_facets upserted", "count", len(hf))
	return nil
}