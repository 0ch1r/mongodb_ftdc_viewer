package storage

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Backend string

const (
	BackendInflux      Backend = "influx"
	BackendVictoria    Backend = "victoriametrics"
	defaultMeasurement         = "ftdc"
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
	Backend     Backend
	Measurement string
	Influx      InfluxConfig
	Victoria    VictoriaConfig
}

type InfluxConfig struct {
	Org     string
	Bucket  string
	URL     string
	Token   string
	UseGzip bool
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
	switch cfg.Backend {
	case BackendInflux:
		return newInfluxWriter(ctx, cfg.measurement(), cfg.Influx)
	case BackendVictoria:
		return newVictoriaWriter(cfg.measurement(), cfg.Victoria)
	default:
		return nil, fmt.Errorf("unsupported storage backend: %s", cfg.Backend)
	}
}
