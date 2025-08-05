package reconstruct_datadog

import (
	"cmp"
	"fmt"
	"time"
	"unsafe"

	"github.com/brody192/locomotive/internal/logline/reconstructor"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/util"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// reconstruct multiple deployment logs into a raw json array
func EnvironmentLogsJsonArray(logs []environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
	array := `[]`

	for i := range logs {
		for key, value := range logs[i].Metadata {
			array, _ = sjson.Set(array, fmt.Sprintf("%d._metadata.%s", i, key), value)
		}

		array, _ = sjson.Set(array, fmt.Sprintf("%d.severity", i), logs[i].Log.Severity)
		array, _ = sjson.Set(array, fmt.Sprintf("%d.message", i), util.StripAnsi(logs[i].Log.Message))

		for _, attribute := range logs[i].Log.Attributes {
			array, _ = sjson.SetRaw(array, fmt.Sprintf("%d.%s", i, attribute.Key), attribute.Value)
		}

		for _, attribute := range reservedAttributes {
			attr := gjson.Get(array, fmt.Sprintf("%d.%s", i, attribute))

			if attr.Exists() {
				array, _ = sjson.Delete(array, fmt.Sprintf("%d.%s", i, attribute))
				array, _ = sjson.Set(array, fmt.Sprintf("%d._%s", i, attribute), attr.Value())
			}
		}

		timestamp := cmp.Or(reconstructor.TryExtractTimestamp(logs[i]), logs[i].Log.Timestamp).Format(time.RFC3339Nano)
		array, _ = sjson.Set(array, fmt.Sprintf("%d.timestamp", i), timestamp)

		array, _ = sjson.Set(array, fmt.Sprintf("%d.ddsource", i), "locomotive")
		array, _ = sjson.Set(array, fmt.Sprintf("%d.service", i), util.SanitizeString(logs[i].Metadata["service_name"]))

		hostname := util.SanitizeString(logs[i].Metadata["project_name"] + "-" + util.SanitizeString(logs[i].Metadata["environment_name"]))
		array, _ = sjson.Set(array, fmt.Sprintf("%d.hostname", i), hostname)
		array, _ = sjson.Set(array, fmt.Sprintf("%d.host", i), hostname)
	}

	return unsafe.Slice(unsafe.StringData(array), len(array)), nil
}
