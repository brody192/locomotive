package reconstruct_loki

import (
	"fmt"
	"strconv"
	"unsafe"

	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
	"github.com/tidwall/sjson"
)

// https://grafana.com/docs/loki/latest/reference/loki-http-api/#ingest-logs

func HttpLogStreams(logs []http_logs.DeploymentHttpLogWithMetadata) ([]byte, error) {
	streams := lokiJSON

	for i := range logs {
		for key, value := range logs[i].Metadata {
			streams, _ = sjson.Set(streams, fmt.Sprintf("streams.%d.stream.%s", i, key), value)
		}

		timestamp := strconv.FormatInt(logs[i].Timestamp.UnixNano(), 10)

		streams, _ = sjson.Set(streams, fmt.Sprintf("streams.%d.values.0.0", i), timestamp)
		streams, _ = sjson.Set(streams, fmt.Sprintf("streams.%d.values.0.1", i), logs[i].Path)
		streams, _ = sjson.SetRaw(streams, fmt.Sprintf("streams.%d.values.0.2", i), unsafe.String(unsafe.SliceData(logs[i].Log), len(logs[i].Log)))

		streams, _ = sjson.Delete(streams, fmt.Sprintf("streams.%d.values.0.2.timestamp", i))
		streams, _ = sjson.Delete(streams, fmt.Sprintf("streams.%d.values.0.2.path", i))
	}

	return unsafe.Slice(unsafe.StringData(streams), len(streams)), nil
}
