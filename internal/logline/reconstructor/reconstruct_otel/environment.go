package reconstruct_otel

import (
	"cmp"
	"encoding/json"
	"strconv"

	"github.com/brody192/locomotive/internal/logline/reconstructor"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/util"
)

func EnvironmentLogsOtel(logs []environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
	data := buildLogsData(logs, func(log environment_logs.EnvironmentLogWithMetadata, nowNano string) logRecord {
		timestamp := cmp.Or(reconstructor.TryExtractTimestamp(log), log.Log.Timestamp)

		var logAttrs []attribute
		for _, a := range log.Log.Attributes {
			if reconstructor.IsCommonTimeStampAttribute(a.Key) {
				continue
			}
			v, _ := reconstructor.DecodeJSONString(a.Value)
			logAttrs = append(logAttrs, stringAttribute(a.Key, v))
		}

		return logRecord{
			TimeUnixNano:         strconv.FormatInt(timestamp.UnixNano(), 10),
			ObservedTimeUnixNano: nowNano,
			SeverityNumber:       getSeverityNumber(log.Log.Severity),
			SeverityText:         log.Log.Severity,
			Body:                 body{StringValue: util.StripAnsi(log.Log.Message)},
			Attributes:           logAttrs,
		}
	})

	return json.Marshal(data)
}
