package reconstruct_sentry

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_sentry/sentry_attribute"
	"github.com/brody192/locomotive/internal/railway/subscribe"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func generateRandomHexString() string {
	var buf [16]byte
	binary.NativeEndian.PutUint64(buf[:8], rand.Uint64())
	binary.NativeEndian.PutUint64(buf[8:], rand.Uint64())
	return hex.EncodeToString(buf[:])
}

func getSeverityNumber(severity string) int {
	severity = strings.ToLower(severity)

	switch severity {
	case "debug":
		return 5
	case "info":
		return 9
	case "warn", "warning":
		return 13
	case "error", "err":
		return 17
	case "fatal":
		return 21
	default:
		return 9 // default to info
	}
}

func normalizeLevel(level string) string {
	level = strings.ToLower(level)

	switch level {
	case "warning":
		return "warn"
	case "err":
		return "error"
	default:
		return level
	}
}

func getLevelFromStatusCode(statusCode int64) (string, int) {
	if statusCode >= 400 && statusCode <= 499 {
		return "warn", 13
	}

	if statusCode >= 500 && statusCode <= 599 {
		return "error", 17
	}

	return "info", 9
}

var (
	rawBoolTrue  = sentry_attribute.BoolValue(true).RawJSON()
	rawBoolFalse = sentry_attribute.BoolValue(false).RawJSON()
	rawNullStr   = sentry_attribute.StringValue("null").RawJSON()
)

// applyStringAttribute infers the type of value and sets it directly on item
// as a Sentry attribute under the "attributes." path prefix.
func applyStringAttribute(item []byte, key, value string) []byte {
	if i, err := strconv.ParseInt(value, 10, 64); err == nil {
		item, _ = sjson.SetRawBytes(item, attributePath(key), sentry_attribute.Int64Value(i).RawJSON())
		return item
	}

	if f, err := strconv.ParseFloat(value, 64); err == nil && isFinite(f) {
		item, _ = sjson.SetRawBytes(item, attributePath(key), sentry_attribute.Float64Value(f).RawJSON())
		return item
	}

	if b, err := strconv.ParseBool(value); err == nil {
		if b {
			item, _ = sjson.SetRawBytes(item, attributePath(key), rawBoolTrue)
		} else {
			item, _ = sjson.SetRawBytes(item, attributePath(key), rawBoolFalse)
		}
		return item
	}

	if gjson.Valid(value) {
		parsed := gjson.Parse(value)
		return flattenToItem(item, key, parsed)
	}

	item = setStringAttribute(item, key, value)
	return item
}

// applyJSONBytesAttributes parses jsonBytes and flattens all fields directly
// onto item as Sentry attributes.
func applyJSONBytesAttributes(item []byte, jsonBytes []byte) []byte {
	if !gjson.ValidBytes(jsonBytes) {
		return item
	}
	return flattenToItem(item, "", gjson.ParseBytes(jsonBytes))
}

// flattenToItem recursively walks a gjson.Result and sets each leaf value
// directly on item as a raw Sentry attribute JSON object.
func flattenToItem(item []byte, prefix string, value gjson.Result) []byte {
	switch value.Type {
	case gjson.JSON:
		// An empty container has no leaves, so send it as text rather than drop the key.
		if prefix != "" && isEmptyContainer(value) {
			empty := "[]"
			if value.IsObject() {
				empty = "{}"
			}
			item = setStringAttribute(item, prefix, empty)
			break
		}

		switch {
		case value.IsObject():
			value.ForEach(func(key, val gjson.Result) bool {
				newKey := prefix + "__" + key.String()
				if prefix == "" {
					newKey = logAttributeKey(key.String())
				}
				item = flattenToItem(item, newKey, val)
				return true
			})
		case value.IsArray():
			value.ForEach(func(key, val gjson.Result) bool {
				item = flattenToItem(item, prefix+"["+key.String()+"]", val)
				return true
			})
		}
	case gjson.String:
		item = setStringAttribute(item, prefix, value.String())
	case gjson.Number:
		// gjson turns an overflowing number into ±Inf, which sjson would write as a
		// bare +Inf and make Sentry reject the whole envelope, so keep its raw text.
		if !isFinite(value.Num) {
			item = setStringAttribute(item, prefix, value.Raw)
			break
		}
		item, _ = sjson.SetRawBytes(item, attributePath(prefix), sentry_attribute.Float64Value(value.Num).RawJSON())
	case gjson.True:
		item, _ = sjson.SetRawBytes(item, attributePath(prefix), rawBoolTrue)
	case gjson.False:
		item, _ = sjson.SetRawBytes(item, attributePath(prefix), rawBoolFalse)
	case gjson.Null:
		item, _ = sjson.SetRawBytes(item, attributePath(prefix), rawNullStr)
	}
	return item
}

// templateAttributes holds the names of the attributes every item starts with.
var templateAttributes = func() map[string]struct{} {
	names := make(map[string]struct{})
	gjson.Get(Item, "attributes").ForEach(func(key, _ gjson.Result) bool {
		names[key.String()] = struct{}{}
		return true
	})
	return names
}()

// logAttributeKey prefixes a log's attribute key with "_" when it would
// otherwise replace one of locomotive's own template attributes.
func logAttributeKey(key string) string {
	if _, ok := templateAttributes[key]; ok {
		return "_" + key
	}
	return key
}

// attributePath returns the sjson path of the attribute named key, escaped so
// that characters like '.', '*' or '|' stay part of the name instead of being
// read as path syntax.
func attributePath(key string) string {
	escaped := gjson.Escape(key)
	// sjson reads a leading ':' as a flag and strips it, which gjson.Escape allows.
	if strings.HasPrefix(escaped, ":") {
		escaped = `\` + escaped
	}
	return "attributes." + escaped
}

// applyMetadataAttributes sets each metadata entry on item as a "_metadata__"
// string attribute and the Railway environment name as sentry.environment.
func applyMetadataAttributes(item []byte, metadata map[string]string) []byte {
	for key, value := range metadata {
		item = setStringAttribute(item, "_metadata__"+key, value)
	}

	if env := metadata[subscribe.MetadataKeyEnvironmentName]; env != "" {
		item = setStringAttribute(item, "sentry.environment", env)
	}

	return item
}

// setStringAttribute sets value on item as the Sentry string attribute named key.
func setStringAttribute(item []byte, key, value string) []byte {
	item, _ = sjson.SetRawBytes(item, attributePath(key), sentry_attribute.StringValue(value).RawJSON())
	return item
}

func isEmptyContainer(value gjson.Result) bool {
	empty := true
	value.ForEach(func(_, _ gjson.Result) bool {
		empty = false
		return false
	})
	return empty
}

// isFinite reports whether f can be encoded as a JSON number.
func isFinite(f float64) bool {
	return !math.IsInf(f, 0) && !math.IsNaN(f)
}
