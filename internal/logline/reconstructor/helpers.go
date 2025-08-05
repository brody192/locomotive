package reconstructor

import (
	"slices"
	"strconv"
	"time"

	"github.com/araddon/dateparse"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
)

// Try to extract a timestamp from the log attributes with `dateparse.ParseStrict`.
//
// If no timestamp is found, or if all timestamps fail to parse, a zero value is returned.
//
// Use with `cmp.Or` to fallback to a default timestamp.
func TryExtractTimestamp(log environment_logs.EnvironmentLogWithMetadata) time.Time {
	for _, attribute := range log.Log.Attributes {
		if IsCommonTimeStampAttribute(attribute.Key) {
			if s, err := strconv.Unquote(attribute.Value); err == nil {
				attribute.Value = s
			}

			if t, err := dateparse.ParseStrict(attribute.Value); err == nil {
				return t
			}
		}
	}

	return time.Time{}
}

func IsCommonTimeStampAttribute(attribute string) bool {
	return slices.Contains(commonTimeStampAttributes, attribute)
}
