package reconstruct_loki

import (
	"strconv"

	"github.com/brody192/locomotive/internal/logline/reconstructor"
	"github.com/brody192/locomotive/internal/railway/subscribe"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
	"github.com/tidwall/sjson"
)

// https://grafana.com/docs/loki/latest/reference/loki-http-api/#ingest-logs

func HttpLogStreams(logs []http_logs.DeploymentHttpLogWithMetadata) ([]byte, error) {
	streamObjects := make([][]byte, 0, len(logs))

	for i := range logs {
		stream := []byte(streamJSON)

		for key, value := range logs[i].Metadata {
			stream, _ = sjson.SetBytes(stream, "stream."+key, value)
		}

		stream, _ = sjson.SetBytes(stream, "stream.service_namespace", logs[i].Metadata[subscribe.MetadataKeyProjectName])

		timestamp := strconv.FormatInt(logs[i].Timestamp.UnixNano(), 10)

		stream, _ = sjson.SetBytes(stream, "values.0.0", timestamp)
		stream, _ = sjson.SetBytes(stream, "values.0.1", logs[i].Path)

		stream = applyJSONBytesAttributes(stream, "values.0.2", logs[i].Log, httpAttributesToSkip)

		streamObjects = append(streamObjects, stream)
	}

	result := []byte(lokiJSON)
	result, _ = sjson.SetRawBytes(result, "streams", reconstructor.RawJSONArray(streamObjects))

	return result, nil
}
