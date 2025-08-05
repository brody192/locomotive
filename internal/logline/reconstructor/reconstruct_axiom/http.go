package reconstruct_axiom

import (
	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_json"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
)

func HttpLogsJsonArray(logs []http_logs.DeploymentHttpLogWithMetadata) ([]byte, error) {
	return reconstruct_json.HttpLogsJsonArrayWithConfig(logs, reconstruct_json.Config{
		TimestampAttribute: timestampAttribute,
	})
}

func HttpLogsJsonLines(logs []http_logs.DeploymentHttpLogWithMetadata) ([]byte, error) {
	return reconstruct_json.HttpLogsJsonLinesWithConfig(logs, reconstruct_json.Config{
		TimestampAttribute: timestampAttribute,
	})
}
