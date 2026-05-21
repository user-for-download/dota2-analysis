package domain

import (
	"testing"
)

func TestFeatureSpecEqual(t *testing.T) {
	t.Run("identical", func(t *testing.T) {
		a := &FeatureSpec{Version: "v1", Features: []FeatureDef{{Name: "x", Dtype: "f32"}}}
		b := &FeatureSpec{Version: "v1", Features: []FeatureDef{{Name: "x", Dtype: "f32"}}}
		if !a.Equal(b) {
			t.Error("expected equal specs")
		}
	})

	t.Run("different version", func(t *testing.T) {
		a := &FeatureSpec{Version: "v1"}
		b := &FeatureSpec{Version: "v2"}
		if a.Equal(b) {
			t.Error("expected not equal")
		}
	})

	t.Run("different feature count", func(t *testing.T) {
		a := &FeatureSpec{Version: "v1", Features: []FeatureDef{{Name: "x", Dtype: "f32"}}}
		b := &FeatureSpec{Version: "v1", Features: []FeatureDef{
			{Name: "x", Dtype: "f32"},
			{Name: "y", Dtype: "f64"},
		}}
		if a.Equal(b) {
			t.Error("expected not equal")
		}
	})

	t.Run("different feature content", func(t *testing.T) {
		a := &FeatureSpec{Version: "v1", Features: []FeatureDef{{Name: "x", Dtype: "f32"}}}
		b := &FeatureSpec{Version: "v1", Features: []FeatureDef{{Name: "y", Dtype: "f32"}}}
		if a.Equal(b) {
			t.Error("expected not equal")
		}
	})

	t.Run("both nil", func(t *testing.T) {
		if !(*FeatureSpec)(nil).Equal(nil) {
			t.Error("nil specs should be equal")
		}
	})

	t.Run("one nil", func(t *testing.T) {
		a := &FeatureSpec{Version: "v1"}
		if a.Equal(nil) {
			t.Error("one nil should not be equal")
		}
	})
}

func TestFeatureSpecMustValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		s := &FeatureSpec{Version: "v1", Features: []FeatureDef{{Name: "x", Dtype: "f32"}}}
		// Should not panic.
		s.MustValidate()
	})

	t.Run("nil spec", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for nil spec")
			}
		}()
		(*FeatureSpec)(nil).MustValidate()
	})

	t.Run("empty version", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for empty version")
			}
		}()
		(&FeatureSpec{Version: "", Features: []FeatureDef{{Name: "x", Dtype: "f32"}}}).MustValidate()
	})

	t.Run("no features", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for no features")
			}
		}()
		(&FeatureSpec{Version: "v1", Features: nil}).MustValidate()
	})

	t.Run("empty feature name", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for empty feature name")
			}
		}()
		(&FeatureSpec{Version: "v1", Features: []FeatureDef{{Name: "", Dtype: "f32"}}}).MustValidate()
	})

	t.Run("empty feature dtype", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for empty dtype")
			}
		}()
		(&FeatureSpec{Version: "v1", Features: []FeatureDef{{Name: "x", Dtype: ""}}}).MustValidate()
	})
}

func TestFeatureSpecChecksum(t *testing.T) {
	s := &FeatureSpec{Version: "v1", Features: []FeatureDef{{Name: "x", Dtype: "f32", SourceHash: "abc"}}}
	c1 := s.Checksum()
	c2 := s.Checksum()
	if c1 != c2 {
		t.Error("checksums should be deterministic")
	}
	if c1 == "" {
		t.Error("checksum should not be empty")
	}

	t.Run("different specs have different checksums", func(t *testing.T) {
		s2 := &FeatureSpec{Version: "v2", Features: []FeatureDef{{Name: "x", Dtype: "f32", SourceHash: "abc"}}}
		if s.Checksum() == s2.Checksum() {
			t.Error("different specs should have different checksums")
		}
	})
}

func TestNewFeatureVector(t *testing.T) {
	spec := &FeatureSpec{Version: "v1", Features: []FeatureDef{
		{Name: "a", Dtype: "f32"},
		{Name: "b", Dtype: "f32"},
	}}

	t.Run("success", func(t *testing.T) {
		v := NewFeatureVector(spec, HeroID(1), []float64{0.5, 0.8})
		if v.Hero() != HeroID(1) {
			t.Errorf("Hero() = %d, want 1", v.Hero())
		}
		if v.Len() != 2 {
			t.Errorf("Len() = %d, want 2", v.Len())
		}
	})

	t.Run("panics on length mismatch", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for length mismatch")
			}
		}()
		NewFeatureVector(spec, HeroID(1), []float64{0.5})
	})
}

func TestFeatureVectorImmutability(t *testing.T) {
	spec := &FeatureSpec{Version: "v1", Features: []FeatureDef{{Name: "a", Dtype: "f32"}}}
	original := []float64{0.5}
	v := NewFeatureVector(spec, HeroID(1), original)

	// Modify original after construction — should not affect the vector.
	original[0] = 999
	if vals := v.Values(); vals[0] != 0.5 {
		t.Errorf("vector was mutated: got %f, want 0.5", vals[0])
	}

	// Modify returned copy — should not affect the vector.
	vals := v.Values()
	vals[0] = 999
	if vals2 := v.Values(); vals2[0] != 0.5 {
		t.Errorf("returned copy was not independent: got %f, want 0.5", vals2[0])
	}
}

func TestFeatureVectorSpec(t *testing.T) {
	spec := &FeatureSpec{Version: "v1", Features: []FeatureDef{{Name: "a", Dtype: "f32"}}}
	v := NewFeatureVector(spec, HeroID(1), []float64{0.5})
	if v.Spec() != spec {
		t.Error("Spec() should return the same pointer")
	}
}
