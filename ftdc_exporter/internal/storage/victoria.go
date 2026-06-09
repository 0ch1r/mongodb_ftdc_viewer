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
	"time"
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
	if strings.TrimSpace(measurement) == "" {
		return "", fmt.Errorf("measurement must be provided")
	}
	var builder strings.Builder
	builder.WriteString(escapeMeasurement(measurement))

	if len(tags) > 0 {
		tagKeys := make([]string, 0, len(tags))
		for key := range tags {
			tagKeys = append(tagKeys, key)
		}
		sort.Strings(tagKeys)
		for _, key := range tagKeys {
			builder.WriteByte(',')
			builder.WriteString(escapeTagComponent(key))
			builder.WriteByte('=')
			builder.WriteString(escapeTagComponent(tags[key]))
		}
	}

	fieldKeys := make([]string, 0, len(p.Fields))
	for key := range p.Fields {
		fieldKeys = append(fieldKeys, key)
	}
	sort.Strings(fieldKeys)

	fieldCount := 0
	for _, key := range fieldKeys {
		value, ok := toLineProtocolValue(p.Fields[key])
		if !ok {
			continue
		}
		if fieldCount == 0 {
			builder.WriteByte(' ')
		} else {
			builder.WriteByte(',')
		}
		builder.WriteString(escapeFieldKey(key))
		builder.WriteByte('=')
		builder.WriteString(value)
		fieldCount++
	}

	if fieldCount == 0 {
		return "", fmt.Errorf("point has no encodable fields")
	}

	builder.WriteByte(' ')
	builder.WriteString(strconv.FormatInt(p.Timestamp.UnixNano(), 10))
	return builder.String(), nil
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
	replacer := strings.NewReplacer(",", "\\,", " ", "\\ ")
	return replacer.Replace(value)
}

func escapeTagComponent(value string) string {
	replacer := strings.NewReplacer(",", "\\,", " ", "\\ ", "=", "\\=")
	return replacer.Replace(value)
}

func escapeFieldKey(value string) string {
	replacer := strings.NewReplacer(",", "\\,", " ", "\\ ")
	return replacer.Replace(value)
}

func quoteFieldString(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	return "\"" + escaped + "\""
}
