package reconstruct_loki

import (
	"cmp"
	"fmt"
	"strconv"
	"unsafe"

	"github.com/brody192/locomotive/internal/logline/reconstructor"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/util"
	"github.com/tidwall/sjson"
)

// https://grafana.com/docs/loki/latest/reference/loki-http-api/#ingest-logs

func EnvironmentLogStreams(logs []environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
	streams := lokiJSON

	for i := range logs {
		for key, value := range logs[i].Metadata {
			streams, _ = sjson.Set(streams, fmt.Sprintf("streams.%d.stream.%s", i, key), value)
		}

		timestamp := strconv.FormatInt(cmp.Or(reconstructor.TryExtractTimestamp(logs[i]), logs[i].Log.Timestamp).UnixNano(), 10)

		streams, _ = sjson.Set(streams, fmt.Sprintf("streams.%d.values.0.0", i), timestamp)
		streams, _ = sjson.Set(streams, fmt.Sprintf("streams.%d.values.0.1", i), util.StripAnsi(logs[i].Log.Message))

		for j := range logs[i].Log.Attributes {
			for key, value := range jsonToAttributes(logs[i].Log.Attributes[j].Key, logs[i].Log.Attributes[j].Value) {
				streams, _ = sjson.Set(streams, fmt.Sprintf("streams.%d.values.0.2.%s", i, key), value)
			}
		}
	}

	return unsafe.Slice(unsafe.StringData(streams), len(streams)), nil
}
