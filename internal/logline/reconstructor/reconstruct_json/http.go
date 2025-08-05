package reconstruct_json

import (
	"bytes"
	"cmp"
	"fmt"
	"strconv"
	"time"
	"unsafe"

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
	array := `[]`

	for i := range logs {
		logObject, err := httpLogLineJson(logs[i], config)
		if err != nil {
			return nil, err
		}

		array, _ = sjson.SetRaw(array, strconv.Itoa(i), unsafe.String(unsafe.SliceData(logObject), len(logObject)))
	}

	return unsafe.Slice(unsafe.StringData(array), len(array)), nil
}

// reconstruct multiple http logs into json lines
func HttpLogsJsonLines(logs []http_logs.DeploymentHttpLogWithMetadata) ([]byte, error) {
	return HttpLogsJsonLinesWithConfig(logs, Config{})
}

// reconstruct multiple http logs into json lines with a custom timestamp attribute
func HttpLogsJsonLinesWithConfig(logs []http_logs.DeploymentHttpLogWithMetadata, config Config) ([]byte, error) {
	lines := bytes.Buffer{}

	for i := range logs {
		jsonLine, err := httpLogLineJson(logs[i], config)
		if err != nil {
			return nil, err
		}

		lines.Write(jsonLine)

		if i < (len(logs) - 1) {
			lines.WriteByte('\n')
		}
	}

	return lines.Bytes(), nil
}

// reconstruct a single http log into a raw json object
func httpLogLineJson(log http_logs.DeploymentHttpLogWithMetadata, config Config) ([]byte, error) {
	object := unsafe.String(unsafe.SliceData(log.Log), len(log.Log))

	for key, value := range log.Metadata {
		object, _ = sjson.Set(object, fmt.Sprintf("_metadata.%s", key), value)
	}

	object, _ = sjson.Set(object, "message", log.Path)

	object, _ = sjson.Set(object, cmp.Or(config.TimestampAttribute, "timestamp"), log.Timestamp.Format(time.RFC3339Nano))

	for _, attribute := range config.ReserverdAttributes {
		attr := gjson.Get(object, attribute)

		if attr.Exists() {
			object, _ = sjson.Delete(object, attribute)
			object, _ = sjson.Set(object, fmt.Sprintf("_%s", attribute), attr.Value())
		}
	}

	return unsafe.Slice(unsafe.StringData(object), len(object)), nil
}
