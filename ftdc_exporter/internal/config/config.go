package config

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/yourusername/my-ftdc-tool/internal/storage"
)

type Config struct {
	InputDir           string
	Bucket             string
	MetricsIncludeFile string
	Parallel           int
	BatchSize          int
	BatchBuffer        int
	Debug              bool
	Measurement        string
	WaitForever        bool
	VictoriaURL        string
	VictoriaToken      string
	VictoriaTenant     string
	VictoriaUsername   string
	VictoriaPassword   string
	VictoriaUseGZip    bool
}

// ParseFlags reads and validates CLI flags, returning a Config instance.
func ParseFlags() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.InputDir, "input-dir", "", "Path to the directory containing FTDC files (required)")
	flag.StringVar(&cfg.Bucket, "bucket", "bucket", "Logical namespace/bucket tag for VictoriaMetrics writes")
	flag.StringVar(&cfg.Measurement, "measurement", "ftdc", "Metric measurement prefix")
	flag.IntVar(&cfg.Parallel, "parallel", 4, "Number of files to process in parallel")
	flag.IntVar(&cfg.BatchSize, "batch-size", 1000, "Number of FTDC metrics per batch")
	flag.IntVar(&cfg.BatchBuffer, "batch-buffer", 1, "Number of batches to queue before blocking")
	flag.StringVar(&cfg.MetricsIncludeFile, "metrics-include-file", "", "Number of batches to queue before blocking")
	flag.BoolVar(&cfg.Debug, "debug", false, "Enable debug logging")
	flag.BoolVar(&cfg.WaitForever, "wait-forever", true, "Wait indefinitely")
	flag.StringVar(&cfg.VictoriaURL, "victoria-url", "http://victoriametrics:8428", "VictoriaMetrics base URL (e.g., http://localhost:8428)")
	flag.StringVar(&cfg.VictoriaToken, "victoria-token", "", "VictoriaMetrics token for writes")
	flag.StringVar(&cfg.VictoriaTenant, "victoria-tenant", "", "VictoriaMetrics tenant (X-Scope-OrgID header)")
	flag.StringVar(&cfg.VictoriaUsername, "victoria-username", "", "VictoriaMetrics basic auth username")
	flag.StringVar(&cfg.VictoriaPassword, "victoria-password", "", "VictoriaMetrics basic auth password")
	flag.BoolVar(&cfg.VictoriaUseGZip, "victoria-gzip", true, "Enable gzip compression for VictoriaMetrics writes")

	flag.Parse()

	validateOrExit(cfg)
	resolvePaths(cfg)

	return cfg
}

func (cfg *Config) Print() {
	fmt.Println("------------------------------------------------------------")
	fmt.Println("Configuration")
	fmt.Println("------------------------------------------------------------")
	fmt.Printf("%-20s : %s\n", "Input Directory", cfg.InputDir)
	fmt.Printf("%-20s : %s\n", "Metrics filter list", cfg.MetricsIncludeFile)
	fmt.Printf("%-20s : %s\n", "Bucket", cfg.Bucket)
	fmt.Printf("%-20s : %s\n", "Measurement", cfg.Measurement)
	fmt.Printf("%-20s : %s\n", "Victoria URL", cfg.VictoriaURL)
	fmt.Printf("%-20s : %t\n", "Victoria Gzip", cfg.VictoriaUseGZip)
	if cfg.VictoriaTenant != "" {
		fmt.Printf("%-20s : %s\n", "Victoria Tenant", cfg.VictoriaTenant)
	}
	fmt.Printf("%-20s : %d\n", "Parallel Files", cfg.Parallel)
	fmt.Printf("%-20s : %d\n", "Batch Size", cfg.BatchSize)
	fmt.Printf("%-20s : %d\n", "Batch Buffer", cfg.BatchBuffer)
	fmt.Printf("%-20s : %t\n", "Wait Forever", cfg.WaitForever)
	fmt.Printf("%-20s : %t\n", "Debug Mode", cfg.Debug)
	fmt.Println("------------------------------------------------------------")

}

func validateOrExit(cfg *Config) {
	missing := []string{}
	if cfg.InputDir == "" {
		missing = append(missing, "--input-dir")
	}
	if cfg.MetricsIncludeFile == "" {
		missing = append(missing, "--metrics-include-file")
	}
	if cfg.VictoriaURL == "" {
		missing = append(missing, "--victoria-url")
	}

	if len(missing) > 0 {
		fmt.Printf("Missing required flags: %v\n\n", missing)
		flag.Usage()
		os.Exit(1)
	}
}

func resolvePaths(cfg *Config) {
	absPath, err := filepath.Abs(cfg.InputDir)
	if err != nil {
		log.Fatalf("Failed to resolve input path: %v", err)
	}
	cfg.InputDir = absPath
}

func (cfg *Config) StorageOptions() storage.Config {
	return storage.Config{
		Measurement: cfg.Measurement,
		Victoria: storage.VictoriaConfig{
			URL:      cfg.VictoriaURL,
			Bucket:   cfg.Bucket,
			Tenant:   cfg.VictoriaTenant,
			Token:    cfg.VictoriaToken,
			Username: cfg.VictoriaUsername,
			Password: cfg.VictoriaPassword,
			UseGzip:  cfg.VictoriaUseGZip,
		},
	}
}
