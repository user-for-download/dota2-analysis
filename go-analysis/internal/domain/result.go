package domain

// Result wraps recommendation output with runtime metadata about how it was produced.
type Result struct {
	Recommendations []Recommendation
	UsedValueModel  bool
	Warnings        []string
}
