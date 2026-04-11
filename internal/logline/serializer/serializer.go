package serializer

import (
	"fmt"

	"github.com/brody192/locomotive/internal/config"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
)

func DeployLogs(logs []environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
	payload, err := config.WebhookModeToConfig[config.Global.WebhookMode].EnvironmentLogReconstructorFunc(logs)
	if err != nil {
		return nil, fmt.Errorf("failed to reconstruct deploy log lines: %w", err)
	}
	return payload, nil
}

func HttpLogs(logs []http_logs.DeploymentHttpLogWithMetadata) ([]byte, error) {
	payload, err := config.WebhookModeToConfig[config.Global.WebhookMode].HTTPLogReconstructorFunc(logs)
	if err != nil {
		return nil, fmt.Errorf("failed to reconstruct http log lines: %w", err)
	}
	return payload, nil
}
