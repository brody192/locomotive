package reconstruct_sentry

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_sentry/sentry_attribute"
	"github.com/tidwall/gjson"
)

func generateRandomHexString() string {
	bytes := make([]byte, 16)

	_, err := rand.Read(bytes)
	if err != nil {
		panic("failed to generate random bytes: " + err.Error())
	}

	return hex.EncodeToString(bytes)
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

func stringToSentryAttributes(prefix string, value string) map[string]sentry_attribute.Value {
	if i, err := strconv.ParseInt(value, 10, 64); err == nil {
		return map[string]sentry_attribute.Value{prefix: sentry_attribute.Int64Value(i)}
	}

	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return map[string]sentry_attribute.Value{prefix: sentry_attribute.Float64Value(f)}
	}

	if b, err := strconv.ParseBool(value); err == nil {
		return map[string]sentry_attribute.Value{prefix: sentry_attribute.BoolValue(b)}
	}

	if gjson.Valid(value) {
		parsed := gjson.Parse(value)
		result := make(map[string]sentry_attribute.Value)

		flatten(prefix, parsed, result)

		return result
	}

	return map[string]sentry_attribute.Value{prefix: sentry_attribute.StringValue(value)}
}

func jsonBytesToSentryAttributes(jsonBytes []byte) map[string]sentry_attribute.Value {
	if !gjson.ValidBytes(jsonBytes) {
		return nil
	}

	parsed := gjson.ParseBytes(jsonBytes)
	result := make(map[string]sentry_attribute.Value)

	flatten("", parsed, result)

	return result
}

func flatten(prefix string, value gjson.Result, result map[string]sentry_attribute.Value) {
	switch value.Type {
	case gjson.JSON:
		switch {
		case value.IsObject():
			value.ForEach(func(key, val gjson.Result) bool {
				newKey := key.String()

				if prefix != "" {
					newKey = prefix + "__" + key.String()
				}

				flatten(newKey, val, result)

				return true
			})
		case value.IsArray():
			value.ForEach(func(key, val gjson.Result) bool {
				index := key.String()

				newKey := prefix + "[" + index + "]"

				flatten(newKey, val, result)

				return true
			})
		}
	case gjson.String:
		result[prefix] = sentry_attribute.StringValue(value.String())
	case gjson.Number:
		result[prefix] = sentry_attribute.Float64Value(value.Num)
	case gjson.True:
		result[prefix] = sentry_attribute.BoolValue(true)
	case gjson.False:
		result[prefix] = sentry_attribute.BoolValue(false)
	case gjson.Null:
		result[prefix] = sentry_attribute.StringValue("null")
	}
}
