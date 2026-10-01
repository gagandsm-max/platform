package supportability

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	componentName = "platform-sensor"
	maxSafeInt64  = int64(9007199254740991)
)

var (
	boundedIDRE      = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
	uuidRE           = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	protocolResultRE = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
	safeKeyRE        = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
)

// NewServiceInstanceID returns a non-authoritative identity unique to this
// Sensor process. It is deliberately independent of Sensor/tenant authority.
func NewServiceInstanceID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "sensor-" + hex.EncodeToString(b[:])
	}
	// Entropy failure must not prevent monitoring from starting. This fallback is
	// process-local supportability identity only; it is never security authority.
	return fmt.Sprintf("sensor-%d-%d", os.Getpid(), time.Now().UTC().UnixNano())
}

// NewJSONLogger creates the BUILD-005 canonical Sensor JSON logger. Unknown or
// unsafe attributes are omitted rather than creating non-canonical top-level
// fields or leaking raw dependency errors.
func NewJSONLogger(w io.Writer, level slog.Leveler, version, serviceInstanceID string) *slog.Logger {
	if strings.TrimSpace(version) == "" {
		version = "unknown"
	}
	if strings.TrimSpace(serviceInstanceID) == "" {
		serviceInstanceID = NewServiceInstanceID()
	}
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: replaceAttr,
	})
	return slog.New(handler).With(
		"component", componentName,
		"version", version,
		"service_instance_id", serviceInstanceID,
	)
}

func replaceAttr(groups []string, attr slog.Attr) slog.Attr {
	// Nested/grouped attributes are not permitted directly. Bounded safe context
	// is accepted only as one sanitized top-level safe_context map, which lets us
	// enforce the 16-key limit deterministically.
	if len(groups) != 0 {
		return slog.Attr{}
	}

	attr.Value = attr.Value.Resolve()
	switch attr.Key {
	case slog.TimeKey:
		if attr.Value.Kind() != slog.KindTime {
			return slog.Attr{}
		}
		return slog.Time("ts", attr.Value.Time().UTC())
	case slog.LevelKey:
		level, ok := attr.Value.Any().(slog.Level)
		if !ok {
			return slog.Attr{}
		}
		return slog.String("level", canonicalLevel(level))
	case slog.MessageKey:
		return slog.String("message", boundedMessage(attr.Value.String()))
	case "error_code":
		return validatedStringAttr("event_code", attr.Value, 64, boundedIDRE)
	case "component":
		if attr.Value.Kind() != slog.KindString || attr.Value.String() != componentName {
			return slog.Attr{}
		}
		return slog.String("component", componentName)
	case "version":
		return validatedPlainStringAttr("version", attr.Value, 64)
	case "service_instance_id":
		return validatedStringAttr("service_instance_id", attr.Value, 128, boundedIDRE)
	case "sensor_id":
		return validatedPlainStringAttr("sensor_id", attr.Value, 255)
	case "agent_id":
		return validatedPlainStringAttr("agent_id", attr.Value, 128)
	case "node_id", "correlation_id", "request_id", "job_id", "entity_id":
		return validatedStringAttr(attr.Key, attr.Value, 128, boundedIDRE)
	case "tenant_id":
		return validatedStringAttr("tenant_id", attr.Value, 36, uuidRE)
	case "event_code":
		return validatedStringAttr("event_code", attr.Value, 64, boundedIDRE)
	case "protocol", "result":
		return validatedStringAttr(attr.Key, attr.Value, 32, protocolResultRE)
	case "retry_count":
		return retryCountAttr(attr.Value)
	case "safe_context":
		return safeContextAttr(attr.Value)
	default:
		// In particular: legacy service/time/msg and raw error attributes are
		// dropped. Callers may retain bounded non-secret diagnostics only via
		// canonical fields or sanitized safe_context.
		return slog.Attr{}
	}
}

func canonicalLevel(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "ERROR"
	case level >= slog.LevelWarn:
		return "WARN"
	case level >= slog.LevelInfo:
		return "INFO"
	case level >= slog.LevelDebug:
		return "DEBUG"
	default:
		return "TRACE"
	}
}

func boundedMessage(message string) string {
	if strings.TrimSpace(message) == "" {
		return "-"
	}
	if utf8.RuneCountInString(message) <= 1024 {
		return message
	}
	runes := []rune(message)
	return string(runes[:1024])
}

func canonicalStringValue(value slog.Value) (string, bool) {
	value = value.Resolve()
	if value.Kind() == slog.KindString {
		return value.String(), true
	}
	if value.Kind() != slog.KindAny {
		return "", false
	}
	rv := reflect.ValueOf(value.Any())
	if !rv.IsValid() || rv.Kind() != reflect.String {
		return "", false
	}
	return rv.String(), true
}

func validatedStringAttr(key string, value slog.Value, maxLen int, pattern *regexp.Regexp) slog.Attr {
	s, ok := canonicalStringValue(value)
	if !ok || len(s) == 0 || len(s) > maxLen || !pattern.MatchString(s) {
		return slog.Attr{}
	}
	return slog.String(key, s)
}

func validatedPlainStringAttr(key string, value slog.Value, maxLen int) slog.Attr {
	s, ok := canonicalStringValue(value)
	if !ok || len(s) == 0 || len(s) > maxLen || containsControl(s) {
		return slog.Attr{}
	}
	return slog.String(key, s)
}

func retryCountAttr(value slog.Value) slog.Attr {
	switch value.Kind() {
	case slog.KindInt64:
		n := value.Int64()
		if n < 0 || n > maxSafeInt64 {
			return slog.Attr{}
		}
		return slog.Int64("retry_count", n)
	case slog.KindUint64:
		n := value.Uint64()
		if n > uint64(maxSafeInt64) {
			return slog.Attr{}
		}
		return slog.Uint64("retry_count", n)
	default:
		return slog.Attr{}
	}
}

func safeContextAttr(value slog.Value) slog.Attr {
	if value.Kind() != slog.KindAny {
		return slog.Attr{}
	}
	input, ok := value.Any().(map[string]any)
	if !ok {
		return slog.Attr{}
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 16 {
		keys = keys[:16]
	}
	output := make(map[string]any, len(keys))
	for _, key := range keys {
		if !safeKeyRE.MatchString(key) || len(key) > 64 || secretKey(key) {
			continue
		}
		if safe, ok := safePrimitive(input[key]); ok {
			output[key] = safe
		}
	}
	if len(output) == 0 {
		return slog.Attr{}
	}
	return slog.Any("safe_context", output)
}

func safePrimitive(value any) (any, bool) {
	switch v := value.(type) {
	case nil:
		return nil, true
	case string:
		if len(v) > 256 || containsControl(v) || looksCredentialLike(v) {
			return nil, false
		}
		return v, true
	case bool:
		return v, true
	case int:
		return v, true
	case int8:
		return v, true
	case int16:
		return v, true
	case int32:
		return v, true
	case int64:
		return v, true
	case uint:
		return v, true
	case uint8:
		return v, true
	case uint16:
		return v, true
	case uint32:
		return v, true
	case uint64:
		if v > uint64(maxSafeInt64) {
			return nil, false
		}
		return v, true
	case float32:
		return v, true
	case float64:
		return v, true
	default:
		return nil, false
	}
}

func secretKey(key string) bool {
	token := normalizeKey(key)
	for _, forbidden := range []string{
		"password", "passwd", "community", "token", "secret", "privatekey",
		"apikey", "clientsecret", "authorization", "cookie", "credential", "passphrase", "pwd",
	} {
		if strings.Contains(token, forbidden) {
			return true
		}
	}
	return false
}

func normalizeKey(key string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(key) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func looksCredentialLike(value string) bool {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "bearer ") || strings.HasPrefix(lower, "basic ") {
		return true
	}
	return strings.Contains(trimmed, "-----BEGIN") && strings.Contains(strings.ToUpper(trimmed), "PRIVATE KEY")
}

func containsControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
