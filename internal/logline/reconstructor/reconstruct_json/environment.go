package reconstruct_json

import (
	"cmp"
	"fmt"
	"slices"
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
	objects := make([][]byte, 0, len(logs))

	for i := range logs {
		logObject, err := environmentLogJson(logs[i], config)
		if err != nil {
			return nil, err
		}

		objects = append(objects, logObject)
	}

	return reconstructor.RawJSONArray(objects), nil
}

// reconstruct multiple deployment logs into json lines
func EnvironmentLogsJsonLines(logs []environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
	return EnvironmentLogsJsonLinesWithConfig(logs, Config{})
}

// reconstruct multiple deployment logs into json lines with a custom timestamp attribute
func EnvironmentLogsJsonLinesWithConfig(logs []environment_logs.EnvironmentLogWithMetadata, config Config) ([]byte, error) {
	objects := make([][]byte, 0, len(logs))

	for i := range logs {
		logObject, err := environmentLogJson(logs[i], config)
		if err != nil {
			return nil, err
		}

		objects = append(objects, logObject)
	}

	return reconstructor.RawJSONLines(objects), nil
}

// reconstruct a single deployment log into a raw json object
func environmentLogJson(log environment_logs.EnvironmentLogWithMetadata, config Config) ([]byte, error) {
	object := []byte(reconstructor.EmptyJSONObject)

	for key, value := range log.Metadata {
		object, _ = sjson.SetBytes(object, fmt.Sprintf("_metadata.%s", key), value)
	}

	object, _ = sjson.SetBytes(object, "message", util.StripAnsi(log.Log.Message))

	object, _ = sjson.SetBytes(object, "severity", log.Log.Severity)

	for i := range log.Log.Attributes {
		key := log.Log.Attributes[i].Key

		// if the attribute is a reserved attribute, add an underscore to the beginning of the key
		if slices.Contains(config.ReserverdAttributes, key) {
			key = fmt.Sprintf("_%s", key)
		}

		object, _ = sjson.SetRawBytes(object, key, []byte(log.Log.Attributes[i].Value))
	}

	if config.TimestampAttribute != "" {
		object, _ = sjson.SetBytes(object, config.TimestampAttribute, cmp.Or(reconstructor.TryExtractTimestamp(log), log.Log.Timestamp).Format(time.RFC3339Nano))
	}

	return object, nil
}
