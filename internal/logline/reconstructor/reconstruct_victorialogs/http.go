package reconstruct_victorialogs

import (
	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_json"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
)

func HttpLogsJsonLines(logs []http_logs.DeploymentHttpLogWithMetadata) ([]byte, error) {
	return reconstruct_json.HttpLogsJsonLinesWithConfig(logs, reconstruct_json.Config{
		TimestampAttribute: timestampAttribute,
		MessageAttribute:   messageAttribute,
	})
}
