package reconstruct_json

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/brody192/locomotive/internal/logline/reconstructor"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/util"
	"github.com/tidwall/sjson"
)

// reconstruct multiple deployment logs into a raw json array
func EnvironmentLogsJsonArray(logs []environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
	return EnvironmentLogsJsonArrayWithConfig(logs, Config{})
}

// reconstruct multiple deployment logs into a raw json array with a custom timestamp attribute
func EnvironmentLogsJsonArrayWithConfig(logs []environment_logs.EnvironmentLogWithMetadata, config Config) ([]byte, error) {
	array := `[]`

	for i := range logs {
		logObject, err := environmentLogJson(logs[i], config)
		if err != nil {
			return nil, err
		}

		array, _ = sjson.SetRaw(array, strconv.Itoa(i), string(logObject))
	}

	return []byte(array), nil
}

// reconstruct multiple deployment logs into json lines
func EnvironmentLogsJsonLines(logs []environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
	return EnvironmentLogsJsonLinesWithConfig(logs, Config{})
}

// reconstruct multiple deployment logs into json lines with a custom timestamp attribute
func EnvironmentLogsJsonLinesWithConfig(logs []environment_logs.EnvironmentLogWithMetadata, config Config) ([]byte, error) {
	lines := bytes.Buffer{}

	for i := range logs {
		logObject, err := environmentLogJson(logs[i], config)
		if err != nil {
			return nil, err
		}

		lines.Write(logObject)

		if i < (len(logs) - 1) {
			lines.WriteByte('\n')
		}
	}

	return lines.Bytes(), nil
}

// reconstruct a single deployment log into a raw json object
func environmentLogJson(log environment_logs.EnvironmentLogWithMetadata, config Config) ([]byte, error) {
	object := `{}`

	for key, value := range log.Metadata {
		object, _ = sjson.Set(object, fmt.Sprintf("_metadata.%s", key), value)
	}

	object, _ = sjson.Set(object, "message", util.StripAnsi(log.Log.Message))

	object, _ = sjson.Set(object, "severity", log.Log.Severity)

	for i := range log.Log.Attributes {
		key := log.Log.Attributes[i].Key

		// if the attribute is a reserved attribute, add an underscore to the beginning of the key
		if slices.Contains(config.ReserverdAttributes, key) {
			key = fmt.Sprintf("_%s", key)
		}

		object, _ = sjson.SetRaw(object, key, log.Log.Attributes[i].Value)
	}

	if config.TimestampAttribute != "" {
		object, _ = sjson.Set(object, config.TimestampAttribute, cmp.Or(reconstructor.TryExtractTimestamp(log), log.Log.Timestamp).Format(time.RFC3339Nano))
	}

	return []byte(object), nil
}
