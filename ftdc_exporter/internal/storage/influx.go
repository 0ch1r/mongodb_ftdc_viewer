package storage

import (
	"context"
	"fmt"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

type influxWriter struct {
	api         api.WriteAPIBlocking
	client      influxdb2.Client
	ctx         context.Context
	measurement string
}

func newInfluxWriter(ctx context.Context, measurement string, cfg InfluxConfig) (Writer, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("influx URL must be provided")
	}
	opts := influxdb2.DefaultOptions().
		SetPrecision(time.Second).
		SetUseGZip(cfg.UseGzip).
		SetMaxRetries(5).
		SetMaxRetryInterval(uint((10 * time.Second) / time.Millisecond))

	client := influxdb2.NewClientWithOptions(cfg.URL, cfg.Token, opts)

	return &influxWriter{
		api:         client.WriteAPIBlocking(cfg.Org, cfg.Bucket),
		client:      client,
		ctx:         ctx,
		measurement: measurement,
	}, nil
}

func (w *influxWriter) Write(ctx context.Context, tags map[string]string, points []Point) error {
	if len(points) == 0 {
		return nil
	}
	influxPoints := make([]*write.Point, 0, len(points))
	for _, p := range points {
		influxPoints = append(influxPoints, influxdb2.NewPoint(w.measurement, tags, p.Fields, p.Timestamp))
	}

	return w.api.WritePoint(w.ctx, influxPoints...)
}

func (w *influxWriter) Close() {
	if w.client != nil {
		w.client.Close()
	}
}
