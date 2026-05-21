package config

// TelemetryConfig holds OpenTelemetry exporter settings.
type TelemetryConfig struct {
	Endpoint   string
	SampleRate float64
}
