package reconstruct_papertrail

import (
	"bytes"
	"cmp"
	"fmt"
	"time"
	"unsafe"

	"github.com/brody192/locomotive/internal/logline/reconstructor"
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

		fmt.Fprintf(&lines, "<%d>%s %s %s: %s ",
			getSeverityNumberFromSeverity(logs[i].Log.Severity),
			cmp.Or(reconstructor.TryExtractTimestamp(logs[i]), logs[i].Log.Timestamp).Format(time.StampNano),
			(util.SanitizeString(logs[i].Metadata["project_name"] + "-" + util.SanitizeString(logs[i].Metadata["environment_name"]))),
			util.SanitizeString(logs[i].Metadata["service_name"]),
			logs[i].Log.Message,
		)

		lines.Write(logObject)

		if i < (len(logs) - 1) {
			lines.WriteByte('\n')
		}
	}

	return lines.Bytes(), nil
}

// reconstruct a single deployment log into a raw json object
func environmentLogJson(log environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
	object := `{}`

	for key, value := range log.Metadata {
		object, _ = sjson.Set(object, fmt.Sprintf("_metadata.%s", key), value)
	}

	object, _ = sjson.Set(object, "severity", log.Log.Severity)

	for i := range log.Log.Attributes {
		object, _ = sjson.SetRaw(object, log.Log.Attributes[i].Key, log.Log.Attributes[i].Value)
	}

	object, _ = sjson.Set(object, "timestamp", cmp.Or(reconstructor.TryExtractTimestamp(log), log.Log.Timestamp).Format(time.RFC3339Nano))

	return unsafe.Slice(unsafe.StringData(object), len(object)), nil
}
