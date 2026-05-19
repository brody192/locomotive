package reconstruct_json

import (
	"fmt"
	"time"

	"github.com/brody192/locomotive/internal/logline/reconstructor"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// reconstruct multiple http logs into a raw json array
func HttpLogsJsonArray(logs []http_logs.DeploymentHttpLogWithMetadata) ([]byte, error) {
	return HttpLogsJsonArrayWithConfig(logs, Config{})
}

// reconstruct multiple http logs into a raw json array with a custom timestamp attribute
func HttpLogsJsonArrayWithConfig(logs []http_logs.DeploymentHttpLogWithMetadata, config Config) ([]byte, error) {
	objects := make([][]byte, 0, len(logs))

	for i := range logs {
		logObject, err := httpLogLineJson(logs[i], config)
		if err != nil {
			return nil, err
		}

		objects = append(objects, logObject)
	}

	return reconstructor.RawJSONArray(objects), nil
}

// reconstruct multiple http logs into json lines
func HttpLogsJsonLines(logs []http_logs.DeploymentHttpLogWithMetadata) ([]byte, error) {
	return HttpLogsJsonLinesWithConfig(logs, Config{})
}

// reconstruct multiple http logs into json lines with a custom timestamp attribute
func HttpLogsJsonLinesWithConfig(logs []http_logs.DeploymentHttpLogWithMetadata, config Config) ([]byte, error) {
	objects := make([][]byte, 0, len(logs))

	for i := range logs {
		logObject, err := httpLogLineJson(logs[i], config)
		if err != nil {
			return nil, err
		}

		objects = append(objects, logObject)
	}

	return reconstructor.RawJSONLines(objects), nil
}

// reconstruct a single http log into a raw json object
func httpLogLineJson(log http_logs.DeploymentHttpLogWithMetadata, config Config) ([]byte, error) {
	object := log.Log

	for key, value := range log.Metadata {
		object, _ = sjson.SetBytes(object, fmt.Sprintf("_metadata.%s", key), value)
	}

	messageAttribute := config.MessageAttribute
	if messageAttribute == "" {
		messageAttribute = "message"
	}

	object, _ = sjson.SetBytes(object, messageAttribute, log.Path)

	for _, attribute := range config.ReserverdAttributes {
		attr := gjson.GetBytes(object, attribute)

		if attr.Exists() {
			object, _ = sjson.DeleteBytes(object, attribute)
			object, _ = sjson.SetBytes(object, fmt.Sprintf("_%s", attribute), attr.Value())
		}
	}

	if config.TimestampAttribute != "" {
		object, _ = sjson.SetBytes(object, config.TimestampAttribute, log.Timestamp.Format(time.RFC3339Nano))
	}

	if config.AdditionalFieldsFunc != nil {
		for key, value := range config.AdditionalFieldsFunc(log.Metadata) {
			object, _ = sjson.SetBytes(object, key, value)
		}
	}

	return object, nil
}
