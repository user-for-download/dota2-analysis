package lgbm

// Model handles LightGBM batch inference.
// Extracted as an interface so tests can use a mock instead of a real .bin file.
type Model interface {
	PredictDense(values []float64, nRows, nCols int, out []float64, startIter, numIter int) error
}
