package reconstructor

import (
	"bytes"
	"slices"
	"time"

	"github.com/araddon/dateparse"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/tidwall/gjson"
)

func IsCommonTimeStampAttribute(attribute string) bool {
	return slices.Contains(commonTimeStampAttributes, attribute)
}

// Try to extract a timestamp from the log attributes with `dateparse.ParseStrict`.
//
// If no timestamp is found, or if all timestamps fail to parse, a zero value is returned.
//
// Use with `cmp.Or` to fallback to a default timestamp.
func TryExtractTimestamp(log environment_logs.EnvironmentLogWithMetadata) time.Time {
	for _, attribute := range log.Log.Attributes {
		if IsCommonTimeStampAttribute(attribute.Key) {
			attribute.Value, _ = DecodeJSONString(attribute.Value)

			if t, err := dateparse.ParseStrict(attribute.Value); err == nil {
				return t
			}
		}
	}

	return time.Time{}
}

// DecodeJSONString decodes raw when it is a JSON string, which is how Railway's
// API encodes string attribute values. ok is false, and raw is returned
// unchanged, for any other value: a number, boolean, null, object or non-JSON.
func DecodeJSONString(raw string) (value string, ok bool) {
	if parsed := gjson.Parse(raw); parsed.Type == gjson.String && gjson.Valid(raw) {
		return parsed.String(), true
	}

	return raw, false
}

func RawJSONLines(elements [][]byte) []byte {
	var buf bytes.Buffer

	for i, el := range elements {
		if i > 0 {
			buf.WriteByte('\n')
		}

		buf.Write(el)
	}

	return buf.Bytes()
}

func RawJSONArray(elements [][]byte) []byte {
	var buf bytes.Buffer
	buf.WriteByte('[')

	for i, el := range elements {
		if i > 0 {
			buf.WriteByte(',')
		}

		buf.Write(el)
	}

	buf.WriteByte(']')
	return buf.Bytes()
}
