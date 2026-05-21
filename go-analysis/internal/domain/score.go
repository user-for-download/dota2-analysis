package domain

// Score is a numeric evaluation of a hero in a given context.
type Score struct {
	Hero  HeroID
	Value float64
}

// Reason explains a contributing factor to a recommendation score.
type Reason struct {
	Factor string
	Note   string
	Delta  float64
}

// Recommendation is a scored hero suggestion with explanatory context.
type Recommendation struct {
	Hero    HeroID
	Name    string
	Score   float64
	Rank    int
	Reasons []Reason
	Risks   []string
}
