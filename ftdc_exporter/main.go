package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/yourusername/my-ftdc-tool/ftdc"
	"github.com/yourusername/my-ftdc-tool/internal/config"
	"github.com/yourusername/my-ftdc-tool/internal/logging"
	"github.com/yourusername/my-ftdc-tool/internal/storage"
	"golang.org/x/sync/errgroup"
)

const (
	grafanaDateFormat   = "2006-01-02T15:04:05.000Z"
	grafanaDashboardURL = "http://localhost:3001/d/ddnw277huiv40ae/ftdc-dashboard"
)

type timeBounds struct {
	min atomic.Int64
	max atomic.Int64
}

func newTimeBounds() *timeBounds {
	tb := &timeBounds{}
	tb.min.Store(math.MaxInt64)
	tb.max.Store(math.MinInt64)
	return tb
}

func (tb *timeBounds) observe(ts int64) {
	for {
		current := tb.min.Load()
		if ts >= current {
			break
		}
		if tb.min.CompareAndSwap(current, ts) {
			break
		}
	}
	for {
		current := tb.max.Load()
		if ts <= current {
			break
		}
		if tb.max.CompareAndSwap(current, ts) {
			break
		}
	}
}

func (tb *timeBounds) rangeMillis() (int64, int64, bool) {
	min := tb.min.Load()
	max := tb.max.Load()
	if min == math.MaxInt64 || max == math.MinInt64 || max < min {
		return 0, 0, false
	}
	return min, max, true
}

func buildGrafanaURL(bounds *timeBounds) (string, error) {
	start, end, ok := bounds.rangeMillis()
	if !ok {
		return "", fmt.Errorf("no FTDC metrics were ingested; cannot build dashboard link")
	}
	from := time.UnixMilli(start).UTC().Format(grafanaDateFormat)
	to := time.UnixMilli(end).UTC().Format(grafanaDateFormat)
	return fmt.Sprintf("%s?from=%s&to=%s&timezone=UTC", grafanaDashboardURL, from, to), nil
}

func ingestFTDCFromFile(ctx context.Context, absInputPath string, writer storage.Writer, cfg *config.Config, counter *atomic.Int64, bounds *timeBounds, includePatterns map[string]struct{}) error {
	tags, err := ftdc.GetTags(ctx, absInputPath)
	if err != nil {
		return err
	}

	batches, errs := ftdc.StreamBatches(ctx, absInputPath, includePatterns, cfg.BatchSize, cfg.BatchBuffer)
	if cfg.Debug {
		logging.Info("Processing: %s", absInputPath)
	}
	for batch := range batches {
		points := make([]storage.Point, 0, len(batch.Items))
		for _, doc := range batch.Items {
			tsMillis, err := extractTimestamp(doc["start"])
			if err != nil {
				return err
			}
			points = append(points, storage.Point{
				Fields:    doc,
				Timestamp: time.UnixMilli(tsMillis),
			})
			bounds.observe(tsMillis)
		}

		if err := writer.Write(ctx, tags, points); err != nil {
			return err
		}

		counter.Add(int64(len(batch.Items)))
	}

	if err := <-errs; err != nil && err != io.EOF {
		fmt.Println("stream error:", err)
	}
	if cfg.Debug {
		logging.Info("Completed processing %s", absInputPath)
	}
	return nil
}

func extractTimestamp(v interface{}) (int64, error) {
	switch val := v.(type) {
	case int64:
		return val, nil
	case int32:
		return int64(val), nil
	case float64:
		return int64(val), nil
	default:
		return 0, fmt.Errorf("document missing start timestamp field")
	}
}

func main() {
	cfg := config.ParseFlags()

	var processed atomic.Int64
	bounds := newTimeBounds()

	time.Sleep(5 * time.Second)

	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				duration := time.Duration(processed.Load()) * time.Second
				timestamp := time.Now().Format("15:04:05")
				fmt.Printf("\r[%s] Ingested %-15s of diagnostics metrics", timestamp, duration)
			case <-done:
				return
			}
		}
	}()

	logging.PrintBanner()
	cfg.Print()
	absFTDCDirectory, err := filepath.Abs(cfg.InputDir)
	if err != nil {
		log.Fatalf("Failed to get absolute path of output file: %v", err)
	}
	var files []string
	err = filepath.WalkDir(absFTDCDirectory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	includePatterns, err := ftdc.ParseIncludeFile(cfg.MetricsIncludeFile)
	if err != nil {
		log.Fatalf("Failed to parse metrics include file: %v", err)
	}

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(cfg.Parallel)

	writerPool := make([]storage.Writer, cfg.Parallel)
	for i := range writerPool {
		w, err := storage.NewWriter(ctx, cfg.StorageOptions())
		if err != nil {
			log.Fatalf("Failed to create metrics writer: %v", err)
		}
		defer w.Close()
		writerPool[i] = w
	}

	var writerIdx atomic.Int64

	sort.Strings(files)

	logging.Info("%d files queued for processing", len(files))
	for _, f := range files {
		file := f
		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				idx := int(writerIdx.Add(1)-1) % len(writerPool)
				if err := ingestFTDCFromFile(ctx, filepath.Clean(file), writerPool[idx], cfg, &processed, bounds, includePatterns); err != nil {
					if errors.Is(err, ftdc.ErrInvalidFormat) {
						logging.Info("failed to ingest file %s: %v", file, err)
						return nil
					}
					return err
				}
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		fmt.Println("failed:", err)
	}

	close(done)

	url, err := buildGrafanaURL(bounds)
	if err != nil {
		log.Fatal(err)
	}

	logging.Info("Metrics available for analysis on:\n\n %s\n", url)
	if cfg.WaitForever {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		fmt.Println("Press Ctrl+C to exit.")
		<-ctx.Done()
		fmt.Println("Received shutdown signal.")
	}
}
