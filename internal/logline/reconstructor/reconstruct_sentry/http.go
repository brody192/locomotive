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

	firstLineData := LineOne
	firstLineData, _ = sjson.Set(firstLineData, "event_id", eventID)
	firstLineData, _ = sjson.Set(firstLineData, "sent_at", timestamp)

	jsonObject.WriteString(firstLineData)
	jsonObject.WriteByte('\n')

	secondLineData := LineTwo
	secondLineData, _ = sjson.Set(secondLineData, "item_count", len(logs))

	jsonObject.WriteString(secondLineData)
	jsonObject.WriteByte('\n')

	thirdLineData := LineThree
	thirdLineData, _ = sjson.Set(thirdLineData, "event_id", eventID)
	thirdLineData, _ = sjson.Set(thirdLineData, "timestamp", timestamp)

	for i := range logs {
		item := Item
		item, _ = sjson.Set(item, "timestamp", logs[i].Timestamp.Format(time.RFC3339Nano))
		item, _ = sjson.Set(item, "trace_id", generateRandomHexString())

		level, severityNumber := getLevelFromStatusCode(logs[i].StatusCode)
		item, _ = sjson.Set(item, "level", level)
		item, _ = sjson.Set(item, "severity_number", severityNumber)
		item, _ = sjson.Set(item, "body", logs[i].Path)

		item, _ = sjson.Set(item, "attributes.level", sentry_attribute.StringValue(level))

		for key, value := range jsonBytesToSentryAttributes(logs[i].Log) {
			item, _ = sjson.Set(item, fmt.Sprintf("attributes.%s", key), value)
		}

		for key, value := range logs[i].Metadata {
			item, _ = sjson.Set(item, fmt.Sprintf("attributes._metadata__%s", key), sentry_attribute.StringValue(value))
		}

		thirdLineData, _ = sjson.SetRaw(thirdLineData, fmt.Sprintf("items.%d", i), item)
	}

	jsonObject.WriteString(thirdLineData)
	jsonObject.WriteByte('\n')

	return jsonObject.Bytes(), nil
}
