package reconstruct_victorialogs

import (
	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_json"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
)

func EnvironmentLogsJsonLines(logs []environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
	return reconstruct_json.EnvironmentLogsJsonLinesWithConfig(logs, reconstruct_json.Config{
		TimestampAttribute: timestampAttribute,
		MessageAttribute:   messageAttribute,
	})
}
