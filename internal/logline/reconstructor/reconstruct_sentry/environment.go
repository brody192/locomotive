package reconstruct_sentry

import (
	"bytes"
	"cmp"
	"fmt"
	"strconv"
	"time"

	"github.com/tidwall/sjson"

	"github.com/brody192/locomotive/internal/logline/reconstructor"
	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_sentry/sentry_attribute"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/util"
)

func EnvironmentLogsEnvelope(logs []environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
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

			// Railway's API returns the values as JSON strings, so we need to unquote them
			if s, err := strconv.Unquote(attribute.Value); err == nil {
				attribute.Value = s
			}

			for key, value := range stringToSentryAttributes(attribute.Key, attribute.Value) {
				item, _ = sjson.SetBytes(item, fmt.Sprintf("attributes.%s", key), value)
			}
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
