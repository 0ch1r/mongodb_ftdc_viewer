package storage

import (
	"context"
	"strings"
	"time"
)

type Backend string

const (
	BackendVictoria Backend = "victoriametrics"
	defaultMeasurement       = "ftdc"
)

type Point struct {
	Fields    map[string]interface{}
	Timestamp time.Time
}

type Writer interface {
	Write(ctx context.Context, tags map[string]string, points []Point) error
	Close()
}

type Config struct {
	Measurement string
	Victoria    VictoriaConfig
}

type VictoriaConfig struct {
	URL      string
	Bucket   string
	Tenant   string
	Token    string
	Username string
	Password string
	UseGzip  bool
}

func (cfg Config) measurement() string {
	if strings.TrimSpace(cfg.Measurement) == "" {
		return defaultMeasurement
	}
	return cfg.Measurement
}

func NewWriter(ctx context.Context, cfg Config) (Writer, error) {
	_ = ctx
	return newVictoriaWriter(cfg.measurement(), cfg.Victoria)
}
