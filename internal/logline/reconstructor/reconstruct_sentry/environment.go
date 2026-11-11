package reconstruct_sentry

import (
	"cmp"
	"time"

	"github.com/tidwall/sjson"

	"github.com/brody192/locomotive/internal/logline/reconstructor"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/util"
)

func EnvironmentLogsEnvelope(logs []environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
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
		item, _ = sjson.SetBytes(item, "timestamp", cmp.Or(reconstructor.TryExtractTimestamp(logs[i]), logs[i].Log.Timestamp).Format(time.RFC3339Nano))
		item, _ = sjson.SetBytes(item, "trace_id", generateRandomHexString())
		item, _ = sjson.SetBytes(item, "level", normalizeLevel(logs[i].Log.Severity))
		item, _ = sjson.SetBytes(item, "severity_number", getSeverityNumber(logs[i].Log.Severity))
		item, _ = sjson.SetBytes(item, "body", util.StripAnsi(logs[i].Log.Message))

		for _, attribute := range logs[i].Log.Attributes {
			// We have already extracted the common timestamp attribute and set it on the current item
			if reconstructor.IsCommonTimeStampAttribute(attribute.Key) {
				continue
			}

			key := logAttributeKey(attribute.Key)

			// Railway's API returns the values JSON-encoded, so a quoted value was
			// logged as a string and must not have its type guessed again.
			if s, ok := reconstructor.DecodeJSONString(attribute.Value); ok {
				item = setStringAttribute(item, key, s)
				continue
			}

			item = applyStringAttribute(item, key, attribute.Value)
		}

		item = applyMetadataAttributes(item, logs[i].Metadata)

		items = append(items, item)
	}

	thirdLineData, _ = sjson.SetRawBytes(thirdLineData, "items", reconstructor.RawJSONArray(items))

	return reconstructor.RawJSONLines([][]byte{firstLineData, secondLineData, thirdLineData}), nil
}
