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
		item, _ = sjson.Set(item, "timestamp", cmp.Or(reconstructor.TryExtractTimestamp(logs[i]), logs[i].Log.Timestamp).Format(time.RFC3339Nano))
		item, _ = sjson.Set(item, "trace_id", generateRandomHexString())
		item, _ = sjson.Set(item, "level", normalizeLevel(logs[i].Log.Severity))
		item, _ = sjson.Set(item, "severity_number", getSeverityNumber(logs[i].Log.Severity))
		item, _ = sjson.Set(item, "body", util.StripAnsi(logs[i].Log.Message))

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
				item, _ = sjson.Set(item, fmt.Sprintf("attributes.%s", key), value)
			}
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
