package reconstruct_sentry

import (
	"bytes"
	"fmt"
	"time"

	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_sentry/sentry_attribute"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
	"github.com/tidwall/sjson"
)

func HttpLogsEnvelope(logs []http_logs.DeploymentHttpLogWithMetadata) ([]byte, error) {
	jsonObject := bytes.Buffer{}

	eventID := generateRandomHexString()
	timestamp := time.Now().Format(time.RFC3339Nano)

	firstLineData := []byte(LineOne)
	firstLineData, _ = sjson.SetBytes(firstLineData, "event_id", eventID)
	firstLineData, _ = sjson.SetBytes(firstLineData, "sent_at", timestamp)

	jsonObject.Write(firstLineData)
	jsonObject.WriteByte('\n')

	secondLineData := []byte(LineTwo)
	secondLineData, _ = sjson.SetBytes(secondLineData, "item_count", len(logs))

	jsonObject.Write(secondLineData)
	jsonObject.WriteByte('\n')

	thirdLineData := []byte(LineThree)
	thirdLineData, _ = sjson.SetBytes(thirdLineData, "event_id", eventID)
	thirdLineData, _ = sjson.SetBytes(thirdLineData, "timestamp", timestamp)

	for i := range logs {
		item := []byte(Item)
		item, _ = sjson.SetBytes(item, "timestamp", logs[i].Timestamp.Format(time.RFC3339Nano))
		item, _ = sjson.SetBytes(item, "trace_id", generateRandomHexString())

		level, severityNumber := getLevelFromStatusCode(logs[i].StatusCode)
		item, _ = sjson.SetBytes(item, "level", level)
		item, _ = sjson.SetBytes(item, "severity_number", severityNumber)
		item, _ = sjson.SetBytes(item, "body", logs[i].Path)

		item, _ = sjson.SetBytes(item, "attributes.level", sentry_attribute.StringValue(level))

		for key, value := range jsonBytesToSentryAttributes(logs[i].Log) {
			item, _ = sjson.SetBytes(item, fmt.Sprintf("attributes.%s", key), value)
		}

		for key, value := range logs[i].Metadata {
			item, _ = sjson.SetBytes(item, fmt.Sprintf("attributes._metadata__%s", key), sentry_attribute.StringValue(value))
		}

		thirdLineData, _ = sjson.SetRawBytes(thirdLineData, fmt.Sprintf("items.%d", i), item)
	}

	jsonObject.Write(thirdLineData)
	jsonObject.WriteByte('\n')

	return jsonObject.Bytes(), nil
}
