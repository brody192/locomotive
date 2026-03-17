package reconstruct_datadog

import (
	"cmp"
	"fmt"
	"time"

	"github.com/brody192/locomotive/internal/logline/reconstructor"
	"github.com/brody192/locomotive/internal/railway/subscribe"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/util"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// reconstruct multiple deployment logs into a raw json array
func EnvironmentLogsJsonArray(logs []environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
	objects := make([][]byte, 0, len(logs))

	for i := range logs {
		object := []byte(reconstructor.EmptyJSONObject)

		for key, value := range logs[i].Metadata {
			object, _ = sjson.SetBytes(object, fmt.Sprintf("_metadata.%s", key), value)
		}

		object, _ = sjson.SetBytes(object, "severity", logs[i].Log.Severity)
		object, _ = sjson.SetBytes(object, "message", util.StripAnsi(logs[i].Log.Message))

		for _, attribute := range logs[i].Log.Attributes {
			object, _ = sjson.SetRawBytes(object, attribute.Key, []byte(attribute.Value))
		}

		for _, attribute := range reservedAttributes {
			attr := gjson.GetBytes(object, attribute)

			if attr.Exists() {
				object, _ = sjson.DeleteBytes(object, attribute)
				object, _ = sjson.SetBytes(object, fmt.Sprintf("_%s", attribute), attr.Value())
			}
		}

		timestamp := cmp.Or(reconstructor.TryExtractTimestamp(logs[i]), logs[i].Log.Timestamp).Format(time.RFC3339Nano)
		object, _ = sjson.SetBytes(object, "timestamp", timestamp)

		object, _ = sjson.SetBytes(object, "ddsource", "locomotive")
		object, _ = sjson.SetBytes(object, "service", util.SanitizeString(logs[i].Metadata[subscribe.MetadataKeyServiceName]))

		hostname := util.SanitizeString(logs[i].Metadata[subscribe.MetadataKeyProjectName] + "-" + util.SanitizeString(logs[i].Metadata[subscribe.MetadataKeyEnvironmentName]))
		object, _ = sjson.SetBytes(object, "hostname", hostname)
		object, _ = sjson.SetBytes(object, "host", hostname)

		objects = append(objects, object)
	}

	return reconstructor.RawJSONArray(objects), nil
}
