package herostatstore

import "context"

type HeroStat struct {
	ID              int16    `json:"id"`
	BaseHealth      *int     `json:"base_health"`
	BaseMana        *int     `json:"base_mana"`
	BaseArmor       *float32 `json:"base_armor"`
	BaseMR          *float32 `json:"base_mr"`
	BaseAttackMin   *int16   `json:"base_attack_min"`
	BaseAttackMax   *int16   `json:"base_attack_max"`
	BaseStr         *int16   `json:"base_str"`
	BaseAgi         *int16   `json:"base_agi"`
	BaseInt         *int16   `json:"base_int"`
	StrGain         *float32 `json:"str_gain"`
	AgiGain         *float32 `json:"agi_gain"`
	IntGain         *float32 `json:"int_gain"`
	AttackRange     *int16   `json:"attack_range"`
	ProjectileSpeed *int16   `json:"projectile_speed"`
	AttackRate      *float32 `json:"attack_rate"`
	MoveSpeed       *int16   `json:"move_speed"`
	TurnRate        *float32 `json:"turn_rate"`
	CmEnabled       *bool    `json:"cm_enabled"`
	TurboPicks      *int     `json:"turbo_picks"`
	TurboWins       *int     `json:"turbo_wins"`
	ProPick         *int     `json:"pro_pick"`
	ProWin          *int     `json:"pro_win"`
	ProBan          *int     `json:"pro_ban"`
	PubPick         *int     `json:"pub_pick"`
	PubWin          *int     `json:"pub_win"`
}

type Writer interface {
	Upsert(ctx context.Context, stats []HeroStat) (int, error)
}