package config

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/yourusername/my-ftdc-tool/internal/storage"
)

type Config struct {
	InputDir           string
	InfluxURL          string
	InfluxToken        string
	InfluxOrg          string
	InfluxUseGZip      bool
	InfluxBucket       string
	MetricsIncludeFile string
	Parallel           int
	BatchSize          int
	BatchBuffer        int
	Debug              bool
	InfluxMeasurement  string
	WaitForever        bool
	StorageBackend     string
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
	flag.StringVar(&cfg.StorageBackend, "storage-backend", "victoriametrics", "Metrics backend to use (victoriametrics|influx)")
	flag.StringVar(&cfg.InfluxURL, "influx-url", "", "InfluxDB server URL (e.g., http://localhost:8086) (required for --storage-backend=influx)")
	flag.BoolVar(&cfg.InfluxUseGZip, "influx-gzip", true, "InfluxDB client gzip compression flag")
	flag.StringVar(&cfg.InfluxToken, "influx-token", "ftdc", "InfluxDB authentication token")
	flag.StringVar(&cfg.InfluxOrg, "influx-org", "my-org", "InfluxDB organization")
	flag.StringVar(&cfg.InfluxMeasurement, "influx-measurement", "ftdc", "InfluxDB measurement")
	flag.StringVar(&cfg.InfluxBucket, "influx-bucket", "my-bucket", "InfluxDB bucket name")
	flag.IntVar(&cfg.Parallel, "parallel", 4, "Number of files to process in parallel")
	flag.IntVar(&cfg.BatchSize, "batch-size", 1000, "Number of FTDC metrics per batch")
	flag.IntVar(&cfg.BatchBuffer, "batch-buffer", 1, "Number of batches to queue before blocking")
	flag.StringVar(&cfg.MetricsIncludeFile, "metrics-include-file", "", "Number of batches to queue before blocking")
	flag.BoolVar(&cfg.Debug, "debug", false, "Enable debug logging")
	flag.BoolVar(&cfg.WaitForever, "wait-forever", true, "Wait indefinitely")
	flag.StringVar(&cfg.VictoriaURL, "victoria-url", "http://victoriametrics:8428", "VictoriaMetrics base URL (e.g., http://localhost:8428)")
	flag.StringVar(&cfg.VictoriaToken, "victoria-token", "", "VictoriaMetrics token for Influx-compatible writes")
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
	fmt.Printf("%-20s : %s\n", "Influx URL", cfg.InfluxURL)
	fmt.Printf("%-20s : %t\n", "Influx Gzip", cfg.InfluxUseGZip)
	fmt.Printf("%-20s : %s\n", "Influx Token", cfg.InfluxToken)
	fmt.Printf("%-20s : %s\n", "Influx Org", cfg.InfluxOrg)
	fmt.Printf("%-20s : %s\n", "Influx Bucket", cfg.InfluxBucket)
	fmt.Printf("%-20s : %s\n", "Influx Measurement", cfg.InfluxMeasurement)
	fmt.Printf("%-20s : %s\n", "Storage Backend", cfg.StorageBackend)
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

	cfg.StorageBackend = strings.ToLower(strings.TrimSpace(cfg.StorageBackend))
	switch cfg.StorageBackend {
	case string(storage.BackendVictoria):
		if cfg.VictoriaURL == "" {
			missing = append(missing, "--victoria-url")
		}
	case string(storage.BackendInflux):
		if cfg.InfluxURL == "" {
			missing = append(missing, "--influx-url")
		}
	default:
		fmt.Printf("Unsupported storage backend: %s\n", cfg.StorageBackend)
		os.Exit(1)
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
		Backend:     storage.Backend(cfg.StorageBackend),
		Measurement: cfg.InfluxMeasurement,
		Influx: storage.InfluxConfig{
			Org:     cfg.InfluxOrg,
			Bucket:  cfg.InfluxBucket,
			URL:     cfg.InfluxURL,
			Token:   cfg.InfluxToken,
			UseGzip: cfg.InfluxUseGZip,
		},
		Victoria: storage.VictoriaConfig{
			URL:      cfg.VictoriaURL,
			Bucket:   cfg.InfluxBucket,
			Tenant:   cfg.VictoriaTenant,
			Token:    cfg.VictoriaToken,
			Username: cfg.VictoriaUsername,
			Password: cfg.VictoriaPassword,
			UseGzip:  cfg.VictoriaUseGZip,
		},
	}
}
