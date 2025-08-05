package reconstruct_datadog

import (
	"fmt"
	"strconv"
	"time"
	"unsafe"

	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
	"github.com/brody192/locomotive/internal/util"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// reconstruct multiple http logs into a raw json array
func HttpLogsJsonArray(logs []http_logs.DeploymentHttpLogWithMetadata) ([]byte, error) {
	array := `[]`

	for i := range logs {
		array, _ = sjson.SetRaw(array, strconv.Itoa(i), unsafe.String(unsafe.SliceData(logs[i].Log), len(logs[i].Log)))

		for key, value := range logs[i].Metadata {
			array, _ = sjson.Set(array, fmt.Sprintf("%d._metadata.%s", i, key), value)
		}

		array, _ = sjson.Set(array, fmt.Sprintf("%d.message", i), logs[i].Path)

		for _, attribute := range reservedAttributes {
			attr := gjson.Get(array, fmt.Sprintf("%d.%s", i, attribute))

			if attr.Exists() {
				array, _ = sjson.Delete(array, fmt.Sprintf("%d.%s", i, attribute))
				array, _ = sjson.Set(array, fmt.Sprintf("%d._%s", i, attribute), attr.Value())
			}
		}

		array, _ = sjson.Set(array, fmt.Sprintf("%d.timestamp", i), logs[i].Timestamp.Format(time.RFC3339Nano))

		array, _ = sjson.Set(array, fmt.Sprintf("%d.ddsource", i), "locomotive")
		array, _ = sjson.Set(array, fmt.Sprintf("%d.service", i), util.SanitizeString(logs[i].Metadata["service_name"]))

		hostname := util.SanitizeString(logs[i].Metadata["project_name"] + "-" + util.SanitizeString(logs[i].Metadata["environment_name"]))
		array, _ = sjson.Set(array, fmt.Sprintf("%d.hostname", i), hostname)
		array, _ = sjson.Set(array, fmt.Sprintf("%d.host", i), hostname)
	}

	return unsafe.Slice(unsafe.StringData(array), len(array)), nil
}
