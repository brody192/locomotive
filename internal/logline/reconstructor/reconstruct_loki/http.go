package reconstruct_loki

import (
	"fmt"
	"slices"
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

		for key, value := range jsonBytesToAttributes("", logs[i].Log) {
			if slices.Contains(httpAttributesToSkip, key) {
				continue
			}

			streams, _ = sjson.Set(streams, fmt.Sprintf("streams.%d.values.0.2.%s", i, key), value)
		}
	}

	return unsafe.Slice(unsafe.StringData(streams), len(streams)), nil
}
