package reconstruct_loki

import (
	"cmp"
	"strconv"

	"github.com/brody192/locomotive/internal/logline/reconstructor"
	"github.com/brody192/locomotive/internal/railway/subscribe"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/util"
	"github.com/tidwall/sjson"
)

// https://grafana.com/docs/loki/latest/reference/loki-http-api/#ingest-logs

func EnvironmentLogStreams(logs []environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
	streamObjects := make([][]byte, 0, len(logs))

	for i := range logs {
		stream := []byte(streamJSON)

		for key, value := range logs[i].Metadata {
			stream, _ = sjson.SetBytes(stream, "stream."+key, value)
		}

		stream, _ = sjson.SetBytes(stream, "stream.service_namespace", logs[i].Metadata[subscribe.MetadataKeyProjectName])

		timestamp := strconv.FormatInt(cmp.Or(reconstructor.TryExtractTimestamp(logs[i]), logs[i].Log.Timestamp).UnixNano(), 10)

		stream, _ = sjson.SetBytes(stream, "values.0.0", timestamp)
		stream, _ = sjson.SetBytes(stream, "values.0.1", util.StripAnsi(logs[i].Log.Message))

		for j := range logs[i].Log.Attributes {
			stream = applyJSONStringAttribute(stream, "values.0.2", logs[i].Log.Attributes[j].Key, logs[i].Log.Attributes[j].Value, nil)
		}

		streamObjects = append(streamObjects, stream)
	}

	result := []byte(lokiJSON)
	result, _ = sjson.SetRawBytes(result, "streams", reconstructor.RawJSONArray(streamObjects))

	return result, nil
}
