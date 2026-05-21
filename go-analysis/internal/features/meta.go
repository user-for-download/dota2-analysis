package features

import "github.com/user-for-download/go-dota2-analysis/internal/domain"

// attrEncoding maps primary_attr strings to numeric codes for ML features.
var attrEncoding = map[string]float64{"str": 1, "agi": 2, "int": 3, "all": 4}

// HeroMetaFeatures computes hero metadata features that do not require a DB call.
// Returns a map from HeroID to [primary_attr_encoded, role_count].
// primary_attr encoding: str=1, agi=2, int=3, all=4 (0 if unknown).
func HeroMetaFeatures(catalog domain.HeroCatalog, heroes []domain.HeroID) (map[domain.HeroID][]float64, error) {
	result := make(map[domain.HeroID][]float64, len(heroes))
	for _, h := range heroes {
		info, ok := catalog.Info(h)
		if !ok {
			result[h] = []float64{0, 0}
			continue
		}
		result[h] = []float64{attrEncoding[info.PrimaryAttr], float64(len(info.Roles))}
	}
	return result, nil
}
