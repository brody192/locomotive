package reconstruct_betterstack

import (
	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_json"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
)

func EnvironmentLogsJsonArray(logs []environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
	return reconstruct_json.EnvironmentLogsJsonArrayWithConfig(logs, reconstruct_json.Config{
		TimestampAttribute: timestampAttribute,
	})
}

func EnvironmentLogsJsonLines(logs []environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
	return reconstruct_json.EnvironmentLogsJsonLinesWithConfig(logs, reconstruct_json.Config{
		TimestampAttribute: timestampAttribute,
	})
}
