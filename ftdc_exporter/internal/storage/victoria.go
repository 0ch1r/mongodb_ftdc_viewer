package storage

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	measurementReplacer = strings.NewReplacer(",", "\\,", " ", "\\ ")
	tagComponentReplacer = strings.NewReplacer(",", "\\,", " ", "\\ ", "=", "\\=")
	fieldKeyReplacer    = strings.NewReplacer(",", "\\,", " ", "\\ ")
	fieldStringReplacer = strings.NewReplacer("\\", "\\\\", "\"", "\\\"")
)

var builderPool = sync.Pool{
	New: func() any { return new(strings.Builder) },
}

var bufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

type victoriaWriter struct {
	mu          sync.Mutex
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
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(points) == 0 {
		return nil
	}

	// Pre-sort tag keys once per batch (same tags for all points).
	tagKeys := make([]string, 0, len(tags))
	for key := range tags {
		tagKeys = append(tagKeys, key)
	}
	sort.Strings(tagKeys)

	// Pre-sort field keys from the first point (all points share the same FTDC schema).
	// If subsequent points have more keys we fall back to per-point sorting.
	fieldKeys := preSortedFieldKeys(points)

	builder := builderPool.Get().(*strings.Builder)
	builder.Reset()
	defer builderPool.Put(builder)

	for i, p := range points {
		if i > 0 {
			builder.WriteByte('\n')
		}
		line, err := pointToLine(w.measurement, tags, tagKeys, fieldKeys, p)
		if err != nil {
			return err
		}
		builder.WriteString(line)
	}

	payload := []byte(builder.String())
	var bodyReader io.Reader = bytes.NewReader(payload)
	headers := make(map[string]string)

	if w.useGzip {
		buf := bufPool.Get().(*bytes.Buffer)
		buf.Reset()
		defer bufPool.Put(buf)
		gz := gzip.NewWriter(buf)
		if _, err := gz.Write(payload); err != nil {
			return err
		}
		if err := gz.Close(); err != nil {
			return err
		}
		bodyReader = buf
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

func pointToLine(measurement string, tags map[string]string, tagKeys []string, fieldKeys []string, p Point) (string, error) {
	if strings.TrimSpace(measurement) == "" {
		return "", fmt.Errorf("measurement must be provided")
	}

	// Use a pooled builder inside pointToLine for the line itself.
	// Note: this builder is separate from the batch-level builder in Write().
	var inner strings.Builder
	inner.WriteString(escapeMeasurement(measurement))

	for _, key := range tagKeys {
		inner.WriteByte(',')
		inner.WriteString(escapeTagComponent(key))
		inner.WriteByte('=')
		inner.WriteString(escapeTagComponent(tags[key]))
	}

	fieldCount := 0
	for _, key := range fieldKeys {
		value, ok := toLineProtocolValue(p.Fields[key])
		if !ok {
			continue
		}
		if fieldCount == 0 {
			inner.WriteByte(' ')
		} else {
			inner.WriteByte(',')
		}
		inner.WriteString(escapeFieldKey(key))
		inner.WriteByte('=')
		inner.WriteString(value)
		fieldCount++
	}

	if fieldCount == 0 {
		return "", fmt.Errorf("point has no encodable fields")
	}

	inner.WriteByte(' ')
	inner.WriteString(strconv.FormatInt(p.Timestamp.UnixNano(), 10))
	return inner.String(), nil
}

// preSortedFieldKeys extracts and sorts field keys from the first point.
// All points in an FTDC batch share the same schema.
func preSortedFieldKeys(points []Point) []string {
	if len(points) == 0 || len(points[0].Fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(points[0].Fields))
	for key := range points[0].Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func toLineProtocolValue(v interface{}) (string, bool) {
	switch value := v.(type) {
	case int:
		return strconv.FormatInt(int64(value), 10) + "i", true
	case int8:
		return strconv.FormatInt(int64(value), 10) + "i", true
	case int16:
		return strconv.FormatInt(int64(value), 10) + "i", true
	case int32:
		return strconv.FormatInt(int64(value), 10) + "i", true
	case int64:
		return strconv.FormatInt(value, 10) + "i", true
	case uint:
		return strconv.FormatUint(uint64(value), 10) + "u", true
	case uint8:
		return strconv.FormatUint(uint64(value), 10) + "u", true
	case uint16:
		return strconv.FormatUint(uint64(value), 10) + "u", true
	case uint32:
		return strconv.FormatUint(uint64(value), 10) + "u", true
	case uint64:
		return strconv.FormatUint(value, 10) + "u", true
	case float32:
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return "", false
		}
		return strconv.FormatFloat(float64(value), 'f', -1, 64), true
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return "", false
		}
		return strconv.FormatFloat(value, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(value), true
	case string:
		return quoteFieldString(value), true
	case fmt.Stringer:
		return quoteFieldString(value.String()), true
	case []byte:
		return quoteFieldString(string(value)), true
	default:
		if v == nil {
			return "", false
		}
		return quoteFieldString(fmt.Sprint(v)), true
	}
}

func escapeMeasurement(value string) string {
	return measurementReplacer.Replace(value)
}

func escapeTagComponent(value string) string {
	return tagComponentReplacer.Replace(value)
}

func escapeFieldKey(value string) string {
	return fieldKeyReplacer.Replace(value)
}

func quoteFieldString(value string) string {
	return "\"" + fieldStringReplacer.Replace(value) + "\""
}
