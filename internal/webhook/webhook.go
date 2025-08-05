package webhook

import (
	"fmt"

	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
	"github.com/brody192/locomotive/internal/webhook/generic"
)

func SendDeployLogsWebhook(logs []environment_logs.EnvironmentLogWithMetadata) error {
	if err := generic.SendWebhookForDeployLogs(logs, client); err != nil {
		return fmt.Errorf("failed to send generic webhook for deploy logs: %w", err)
	}

	return nil
}

func SendHttpLogsWebhook(logs []http_logs.DeploymentHttpLogWithMetadata) error {
	if err := generic.SendWebhookForHttpLogs(logs, client); err != nil {
		return fmt.Errorf("failed to send generic webhook for http logs: %w", err)
	}

	return nil
}
