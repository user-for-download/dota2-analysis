package herostatstore

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PG struct {
	pool *pgxpool.Pool
}

func NewPG(p *pgxpool.Pool) *PG {
	return &PG{pool: p}
}

var _ Writer = (*PG)(nil)

const heroStatUpsertSQL = `
INSERT INTO hero_stats (
    id, base_health, base_mana, base_armor, base_mr,
    base_attack_min, base_attack_max, base_str, base_agi, base_int,
    str_gain, agi_gain, int_gain, attack_range, projectile_speed,
    attack_rate, move_speed, turn_rate, cm_enabled,
    turbo_picks, turbo_wins, pro_picks, pro_wins, pro_bans,
    pub_picks, pub_wins, pub_win_rate, pro_win_rate, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15, $16, $17, $18, $19,
    $20, $21, $22, $23, $24, $25, $26, $27, $28, now()
)
ON CONFLICT (id) DO UPDATE SET
    base_health = EXCLUDED.base_health,
    base_mana = EXCLUDED.base_mana,
    base_armor = EXCLUDED.base_armor,
    base_mr = EXCLUDED.base_mr,
    base_attack_min = EXCLUDED.base_attack_min,
    base_attack_max = EXCLUDED.base_attack_max,
    base_str = EXCLUDED.base_str,
    base_agi = EXCLUDED.base_agi,
    base_int = EXCLUDED.base_int,
    str_gain = EXCLUDED.str_gain,
    agi_gain = EXCLUDED.agi_gain,
    int_gain = EXCLUDED.int_gain,
    attack_range = EXCLUDED.attack_range,
    projectile_speed = EXCLUDED.projectile_speed,
    attack_rate = EXCLUDED.attack_rate,
    move_speed = EXCLUDED.move_speed,
    turn_rate = EXCLUDED.turn_rate,
    cm_enabled = EXCLUDED.cm_enabled,
    turbo_picks = EXCLUDED.turbo_picks,
    turbo_wins = EXCLUDED.turbo_wins,
    pro_picks = EXCLUDED.pro_picks,
    pro_wins = EXCLUDED.pro_wins,
    pro_bans = EXCLUDED.pro_bans,
    pub_picks = EXCLUDED.pub_picks,
    pub_wins = EXCLUDED.pub_wins,
    pub_win_rate = EXCLUDED.pub_win_rate,
    pro_win_rate = EXCLUDED.pro_win_rate,
    updated_at = now()
`

func (r *PG) Upsert(ctx context.Context, stats []HeroStat) (int, error) {
	if len(stats) == 0 {
		return 0, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var n int
	for _, s := range stats {
		var pubWinRate, proWinRate *float32
		if s.PubPick != nil && *s.PubPick > 0 && s.PubWin != nil {
			rate := float32(*s.PubWin) / float32(*s.PubPick)
			pubWinRate = &rate
		}
		if s.ProPick != nil && *s.ProPick > 0 && s.ProWin != nil {
			rate := float32(*s.ProWin) / float32(*s.ProPick)
			proWinRate = &rate
		}

		if _, err := tx.Exec(ctx, "SELECT ensure_hero_stubs(ARRAY[$1::smallint])", s.ID); err != nil {
			return n, fmt.Errorf("hero stub check failed: %w", err)
		}

		if _, err := tx.Exec(ctx, heroStatUpsertSQL,
			s.ID, s.BaseHealth, s.BaseMana, s.BaseArmor, s.BaseMR,
			s.BaseAttackMin, s.BaseAttackMax, s.BaseStr, s.BaseAgi, s.BaseInt,
			s.StrGain, s.AgiGain, s.IntGain, s.AttackRange, s.ProjectileSpeed,
			s.AttackRate, s.MoveSpeed, s.TurnRate, s.CmEnabled,
			s.TurboPicks, s.TurboWins, s.ProPick, s.ProWin, s.ProBan,
			s.PubPick, s.PubWin, pubWinRate, proWinRate,
		); err != nil {
			return n, fmt.Errorf("upsert herostat %d: %w", s.ID, err)
		}
		n++
	}
	if err := tx.Commit(ctx); err != nil {
		return n, fmt.Errorf("commit: %w", err)
	}
	return n, nil
}