package reconstruct_sentry

import (
	"time"

	"github.com/brody192/locomotive/internal/logline/reconstructor"
	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_sentry/sentry_attribute"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
	"github.com/tidwall/sjson"
)

func HttpLogsEnvelope(logs []http_logs.DeploymentHttpLogWithMetadata) ([]byte, error) {
	eventID := generateRandomHexString()
	timestamp := time.Now().Format(time.RFC3339Nano)

	firstLineData := []byte(LineOne)
	firstLineData, _ = sjson.SetBytes(firstLineData, "event_id", eventID)
	firstLineData, _ = sjson.SetBytes(firstLineData, "sent_at", timestamp)

	secondLineData := []byte(LineTwo)
	secondLineData, _ = sjson.SetBytes(secondLineData, "item_count", len(logs))

	thirdLineData := []byte(LineThree)
	thirdLineData, _ = sjson.SetBytes(thirdLineData, "event_id", eventID)
	thirdLineData, _ = sjson.SetBytes(thirdLineData, "timestamp", timestamp)

	items := make([][]byte, 0, len(logs))

	for i := range logs {
		item := []byte(Item)
		item, _ = sjson.SetBytes(item, "timestamp", logs[i].Timestamp.Format(time.RFC3339Nano))
		item, _ = sjson.SetBytes(item, "trace_id", generateRandomHexString())

		level, severityNumber := getLevelFromStatusCode(logs[i].StatusCode)
		item, _ = sjson.SetBytes(item, "level", level)
		item, _ = sjson.SetBytes(item, "severity_number", severityNumber)
		item, _ = sjson.SetBytes(item, "body", logs[i].Path)

		item, _ = sjson.SetRawBytes(item, "attributes.level", sentry_attribute.StringValue(level).RawJSON())

		item = applyJSONBytesAttributes(item, logs[i].Log)

		for key, value := range logs[i].Metadata {
			item, _ = sjson.SetRawBytes(item, ("attributes._metadata__" + key), sentry_attribute.StringValue(value).RawJSON())
		}

		items = append(items, item)
	}

	thirdLineData, _ = sjson.SetRawBytes(thirdLineData, "items", reconstructor.RawJSONArray(items))

	return reconstructor.RawJSONLines([][]byte{firstLineData, secondLineData, thirdLineData}), nil
}
