package recommend

import (
	"fmt"
	"strings"

	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// FormatReasons returns a human-readable string for a recommendation's reasons.
func FormatReasons(rec domain.Recommendation) string {
	if len(rec.Reasons) == 0 {
		return "No specific reasons available"
	}

	parts := make([]string, 0, len(rec.Reasons))
	for _, r := range rec.Reasons {
		parts = append(parts, fmt.Sprintf("%s (%s)", r.Factor, r.Note))
	}
	return fmt.Sprintf("✓ %s", strings.Join(parts, ", "))
}

// FormatRisks returns a human-readable string for a recommendation's risks.
func FormatRisks(rec domain.Recommendation) string {
	if len(rec.Risks) == 0 {
		return ""
	}
	return fmt.Sprintf("⚠ %s", strings.Join(rec.Risks, ", "))
}
