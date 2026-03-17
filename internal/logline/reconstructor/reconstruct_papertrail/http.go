package reconstruct_papertrail

import (
	"fmt"
	"time"

	"github.com/brody192/locomotive/internal/railway/subscribe"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
	"github.com/brody192/locomotive/internal/util"
	"github.com/tidwall/sjson"
)

// reconstruct multiple http logs into json lines with a custom timestamp attribute
func HttpLogsSyslog(logs []http_logs.DeploymentHttpLogWithMetadata) ([]byte, error) {
	lines := make([][]byte, 0, len(logs))

	for i := range logs {
		jsonLine, err := httpLogLineJson(logs[i])
		if err != nil {
			return nil, err
		}

		line := formatSyslogLine(
			// Severity
			getSeverityNumberFromStatusCode(logs[i].StatusCode),
			// Timestamp
			logs[i].Timestamp.Format(rfc5424time),
			// Hostname
			util.SanitizeString(logs[i].Metadata[subscribe.MetadataKeyProjectName]+"-"+util.SanitizeString(logs[i].Metadata[subscribe.MetadataKeyEnvironmentName])),
			// Service
			util.SanitizeString(logs[i].Metadata[subscribe.MetadataKeyServiceName]),
			// Message
			logs[i].Path,
			// Body (JSON object)
			jsonLine,
		)

		lines = append(lines, line)
	}

	return joinSyslogLines(lines), nil
}

// reconstruct a single http log into a raw json object
func httpLogLineJson(log http_logs.DeploymentHttpLogWithMetadata) ([]byte, error) {
	object := log.Log

	for key, value := range log.Metadata {
		object, _ = sjson.SetBytes(object, fmt.Sprintf("_metadata.%s", key), value)
	}

	object, _ = sjson.DeleteBytes(object, "path")

	object, _ = sjson.SetBytes(object, "timestamp", log.Timestamp.Format(time.RFC3339Nano))

	return object, nil
}
