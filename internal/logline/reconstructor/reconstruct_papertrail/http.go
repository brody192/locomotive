package reconstruct_papertrail

import (
	"bytes"
	"fmt"
	"time"

	"github.com/brody192/locomotive/internal/railway/subscribe"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
	"github.com/brody192/locomotive/internal/util"
	"github.com/tidwall/sjson"
)

// reconstruct multiple http logs into json lines with a custom timestamp attribute
func HttpLogsJsonLines(logs []http_logs.DeploymentHttpLogWithMetadata) ([]byte, error) {
	lines := bytes.Buffer{}

	for i := range logs {
		jsonLine, err := httpLogLineJson(logs[i])
		if err != nil {
			return nil, err
		}

		fmt.Fprintf(&lines, "<%d>1 %s %s %s - - - %s",
			getSeverityNumberFromStatusCode(logs[i].StatusCode),
			logs[i].Timestamp.Format(rfc5424time),
			util.SanitizeString(logs[i].Metadata[subscribe.MetadataKeyProjectName]+"-"+util.SanitizeString(logs[i].Metadata[subscribe.MetadataKeyEnvironmentName])),
			util.SanitizeString(logs[i].Metadata[subscribe.MetadataKeyServiceName]),
			logs[i].Path,
		)

		lines.WriteByte(' ')
		lines.Write(jsonLine)

		if i < (len(logs) - 1) {
			lines.WriteByte('\n')
		}
	}

	return lines.Bytes(), nil
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
