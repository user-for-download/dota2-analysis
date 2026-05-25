package lgbm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dmitryikh/leaves"
	"github.com/user-for-download/dota2-analysis/go-analysis/internal/domain"
)

// ModelMeta contains metadata about a trained LightGBM model.
type ModelMeta struct {
	Version   string    `json:"version"`
	TrainedAt time.Time `json:"trained_at"`
	RecallAt5 float64   `json:"recall_at_5"`
	NDCGAt10  float64   `json:"ndcg_at_10"`
	BestIter  int       `json:"best_iter"`
	PatchID   int32     `json:"patch_id"`
}

// LoadModel loads a model from a directory containing model.bin, spec.json, meta.json.
func LoadModel(modelDir string) (*Scorer, error) {
	binPath := filepath.Join(modelDir, "model.bin")
	specPath := filepath.Join(modelDir, "spec.json")
	metaPath := filepath.Join(modelDir, "meta.json")

	// Read spec
	specData, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("read spec.json: %w", err)
	}
	var spec domain.FeatureSpec
	if err := json.Unmarshal(specData, &spec); err != nil {
		return nil, fmt.Errorf("parse spec.json: %w", err)
	}

	// Read meta
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("read meta.json: %w", err)
	}
	var meta ModelMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return nil, fmt.Errorf("parse meta.json: %w", err)
	}

	// Load LightGBM model binary
	ens, err := loadEnsemble(binPath)
	if err != nil {
		return nil, fmt.Errorf("load model.bin: %w", err)
	}

	if meta.PatchID != 0 {
		// PatchID embedded in the model; log it so operators can verify
		// the loaded model matches the expected patch.
		fmt.Printf("info: model trained for patch %d\n", meta.PatchID)
	} else {
		fmt.Printf("info: model loaded (no patch metadata)\n")
	}

	return &Scorer{
		ens:  ens,
		spec: &spec,
		meta: meta,
		dir:  modelDir,
	}, nil
}

// loadEnsemble loads a LightGBM ensemble from a binary file.
// Uses github.com/dmitryikh/leaves for inference.
func loadEnsemble(path string) (*leaves.Ensemble, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	// LGEnsembleFromReader requires a *bufio.Reader
	reader := bufio.NewReader(f)
	ens, err := leaves.LGEnsembleFromReader(reader, true)
	if err != nil {
		return nil, fmt.Errorf("parse LightGBM model: %w", err)
	}
	return ens, nil
}
