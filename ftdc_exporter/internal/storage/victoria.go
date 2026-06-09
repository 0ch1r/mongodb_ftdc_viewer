package storage

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	lineprotocol "github.com/influxdata/line-protocol"
)

type victoriaWriter struct {
	client      *http.Client
	endpoint    string
	measurement string
	bucket      string
	token       string
	username    string
	password    string
	tenant      string
	useGzip     bool
}

func newVictoriaWriter(measurement string, cfg VictoriaConfig) (Writer, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("victoriametrics URL must be provided")
	}

	endpoint, err := url.Parse(strings.TrimSuffix(cfg.URL, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid victoriametrics url: %w", err)
	}
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/api/v2/write"

	return &victoriaWriter{
		client:      &http.Client{Timeout: 30 * time.Second},
		endpoint:    endpoint.String(),
		measurement: measurement,
		bucket:      cfg.Bucket,
		token:       cfg.Token,
		username:    cfg.Username,
		password:    cfg.Password,
		tenant:      cfg.Tenant,
		useGzip:     cfg.UseGzip,
	}, nil
}

func (w *victoriaWriter) Write(ctx context.Context, tags map[string]string, points []Point) error {
	if len(points) == 0 {
		return nil
	}

	var builder strings.Builder
	for i, p := range points {
		if i > 0 {
			builder.WriteByte('\n')
		}
		line, err := pointToLine(w.measurement, tags, p)
		if err != nil {
			return err
		}
		builder.WriteString(line)
	}

	payload := []byte(builder.String())
	var bodyReader io.Reader = bytes.NewReader(payload)
	headers := make(map[string]string)

	if w.useGzip {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(payload); err != nil {
			return err
		}
		if err := gz.Close(); err != nil {
			return err
		}
		bodyReader = &buf
		headers["Content-Encoding"] = "gzip"
	}

	reqURL := w.buildURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bodyReader)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "text/plain")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	if w.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Token %s", w.token))
	}
	if w.username != "" {
		req.SetBasicAuth(w.username, w.password)
	}
	if w.tenant != "" {
		req.Header.Set("X-Scope-OrgID", w.tenant)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("victoriametrics write failed: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}

	return nil
}

func (w *victoriaWriter) Close() {}

func (w *victoriaWriter) buildURL() string {
	params := url.Values{}
	params.Set("precision", "ns")
	if w.bucket != "" {
		params.Set("bucket", w.bucket)
	}
	return fmt.Sprintf("%s?%s", w.endpoint, params.Encode())
}

func pointToLine(measurement string, tags map[string]string, p Point) (string, error) {
	point := influxdb2.NewPoint(measurement, tags, p.Fields, p.Timestamp)
	var builder strings.Builder
	encoder := lineprotocol.NewEncoder(&builder)
	encoder.SetFieldSortOrder(lineprotocol.SortFields)
	encoder.SetPrecision(time.Nanosecond)
	if _, err := encoder.Encode(point); err != nil {
		return "", err
	}
	return builder.String(), nil
}
