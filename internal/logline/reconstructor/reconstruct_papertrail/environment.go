package reconstruct_papertrail

import (
	"bytes"
	"cmp"
	"fmt"
	"time"

	"github.com/brody192/locomotive/internal/logline/reconstructor"
	"github.com/brody192/locomotive/internal/railway/subscribe"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/util"
	"github.com/tidwall/sjson"
)

// reconstruct multiple deployment logs into json lines with a custom timestamp attribute
func EnvironmentLogsJsonLines(logs []environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
	lines := bytes.Buffer{}

	for i := range logs {
		logObject, err := environmentLogJson(logs[i])
		if err != nil {
			return nil, err
		}

		fmt.Fprintf(&lines, "<%d>1 %s %s %s - - - %s",
			getSeverityNumberFromSeverity(logs[i].Log.Severity),
			cmp.Or(reconstructor.TryExtractTimestamp(logs[i]), logs[i].Log.Timestamp).Format(rfc5424time),
			util.SanitizeString(logs[i].Metadata[subscribe.MetadataKeyProjectName]+"-"+util.SanitizeString(logs[i].Metadata[subscribe.MetadataKeyEnvironmentName])),
			util.SanitizeString(logs[i].Metadata[subscribe.MetadataKeyServiceName]),
			logs[i].Log.Message,
		)

		lines.WriteByte(' ')
		lines.Write(logObject)

		if i < (len(logs) - 1) {
			lines.WriteByte('\n')
		}
	}

	return lines.Bytes(), nil
}

// reconstruct a single deployment log into a raw json object
func environmentLogJson(log environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
	object := []byte(reconstructor.EmptyJSONObject)

	for key, value := range log.Metadata {
		object, _ = sjson.SetBytes(object, fmt.Sprintf("_metadata.%s", key), value)
	}

	object, _ = sjson.SetBytes(object, "severity", log.Log.Severity)

	for i := range log.Log.Attributes {
		object, _ = sjson.SetRawBytes(object, log.Log.Attributes[i].Key, []byte(log.Log.Attributes[i].Value))
	}

	object, _ = sjson.SetBytes(object, "timestamp", cmp.Or(reconstructor.TryExtractTimestamp(log), log.Log.Timestamp).Format(time.RFC3339Nano))

	return object, nil
}
