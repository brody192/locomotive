package reconstruct_sentry

import (
	"encoding/binary"
	"encoding/hex"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_sentry/sentry_attribute"
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
		item, _ = sjson.SetRawBytes(item, ("attributes." + key), sentry_attribute.Int64Value(i).RawJSON())
		return item
	}

	if f, err := strconv.ParseFloat(value, 64); err == nil {
		item, _ = sjson.SetRawBytes(item, ("attributes." + key), sentry_attribute.Float64Value(f).RawJSON())
		return item
	}

	if b, err := strconv.ParseBool(value); err == nil {
		if b {
			item, _ = sjson.SetRawBytes(item, ("attributes." + key), rawBoolTrue)
		} else {
			item, _ = sjson.SetRawBytes(item, ("attributes." + key), rawBoolFalse)
		}
		return item
	}

	if gjson.Valid(value) {
		parsed := gjson.Parse(value)
		return flattenToItem(item, key, parsed)
	}

	item, _ = sjson.SetRawBytes(item, ("attributes." + key), sentry_attribute.StringValue(value).RawJSON())
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
		switch {
		case value.IsObject():
			value.ForEach(func(key, val gjson.Result) bool {
				newKey := key.String()
				if prefix != "" {
					newKey = prefix + "__" + key.String()
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
		item, _ = sjson.SetRawBytes(item, ("attributes." + prefix), sentry_attribute.StringValue(value.String()).RawJSON())
	case gjson.Number:
		item, _ = sjson.SetRawBytes(item, ("attributes." + prefix), sentry_attribute.Float64Value(value.Num).RawJSON())
	case gjson.True:
		item, _ = sjson.SetRawBytes(item, "attributes."+prefix, rawBoolTrue)
	case gjson.False:
		item, _ = sjson.SetRawBytes(item, ("attributes." + prefix), rawBoolFalse)
	case gjson.Null:
		item, _ = sjson.SetRawBytes(item, ("attributes." + prefix), rawNullStr)
	}
	return item
}
