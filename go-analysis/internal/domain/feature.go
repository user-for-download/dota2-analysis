package domain

import (
	"crypto/sha256"
	"fmt"
	"slices"
)

// FeatureDef describes a single feature in a feature spec.
type FeatureDef struct {
	Name       string `json:"name"`
	Dtype      string `json:"dtype"`
	SourceHash string `json:"source_hash"`
}

// FeatureSpec defines the schema for a feature vector.
type FeatureSpec struct {
	Version  string       `json:"version"`
	Features []FeatureDef `json:"features"`
}

// Equal reports whether two FeatureSpecs are identical.
func (s *FeatureSpec) Equal(other *FeatureSpec) bool {
	if s == nil || other == nil {
		return s == other
	}
	if s.Version != other.Version {
		return false
	}
	if len(s.Features) != len(other.Features) {
		return false
	}
	for i := range s.Features {
		if s.Features[i].Name != other.Features[i].Name || s.Features[i].Dtype != other.Features[i].Dtype {
			return false
		}
	}
	return true
}

// MustValidate panics if the spec is invalid.
func (s *FeatureSpec) MustValidate() {
	if s == nil {
		panic("feature spec is nil")
	}
	if s.Version == "" {
		panic("feature spec version is empty")
	}
	if len(s.Features) == 0 {
		panic("feature spec has no features")
	}
	for i, f := range s.Features {
		if f.Name == "" {
			panic(fmt.Sprintf("feature %d has empty name", i))
		}
		if f.Dtype == "" {
			panic(fmt.Sprintf("feature %d has empty dtype", i))
		}
	}
}

// Checksum returns a SHA-256 hex digest of the spec definition.
func (s *FeatureSpec) Checksum() string {
	h := sha256.New()
	h.Write([]byte(s.Version))
	for _, f := range s.Features {
		h.Write([]byte(f.Name))
		h.Write([]byte(f.Dtype))
		h.Write([]byte(f.SourceHash))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// FeatureVector is an immutable feature vector bound to a specific spec and hero.
// All fields are private — only Values() returns a copy.
type FeatureVector struct {
	spec   *FeatureSpec
	hero   HeroID
	values []float64
}

// NewFeatureVector creates a validated feature vector.
func NewFeatureVector(spec *FeatureSpec, hero HeroID, values []float64) *FeatureVector {
	spec.MustValidate()
	if len(values) != len(spec.Features) {
		panic("values length does not match spec features length")
	}
	return &FeatureVector{
		spec:   spec,
		hero:   hero,
		values: slices.Clone(values),
	}
}

// Hero returns the hero this vector was computed for.
func (v *FeatureVector) Hero() HeroID { return v.hero }

// Spec returns the feature spec this vector conforms to.
func (v *FeatureVector) Spec() *FeatureSpec { return v.spec }

// Len returns the number of feature values.
func (v *FeatureVector) Len() int { return len(v.values) }

// Values returns a copy of the feature values.
func (v *FeatureVector) Values() []float64 { return slices.Clone(v.values) }
