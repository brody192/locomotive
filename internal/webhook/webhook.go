package webhook

import (
	"fmt"

	"github.com/brody192/locomotive/internal/config"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
	"github.com/brody192/locomotive/internal/webhook/generic"
)

func SerializeDeployLogs(logs []environment_logs.EnvironmentLogWithMetadata) ([]byte, error) {
	payload, err := config.WebhookModeToConfig[config.Global.WebhookMode].EnvironmentLogReconstructorFunc(logs)
	if err != nil {
		return nil, fmt.Errorf("failed to reconstruct deploy log lines: %w", err)
	}
	return payload, nil
}

func SerializeHttpLogs(logs []http_logs.DeploymentHttpLogWithMetadata) ([]byte, error) {
	payload, err := config.WebhookModeToConfig[config.Global.WebhookMode].HTTPLogReconstructorFunc(logs)
	if err != nil {
		return nil, fmt.Errorf("failed to reconstruct http log lines: %w", err)
	}
	return payload, nil
}

func SendPayload(payload []byte) error {
	return generic.SendRawWebhook(payload, config.Global.WebhookUrl, config.Global.AdditionalHeaders, client)
}

func SendDeployLogsWebhook(logs []environment_logs.EnvironmentLogWithMetadata) (serializedLogs []byte, err error) {
	payload, err := SerializeDeployLogs(logs)
	if err != nil {
		return nil, err
	}
	return payload, SendPayload(payload)
}

func SendHttpLogsWebhook(logs []http_logs.DeploymentHttpLogWithMetadata) (serializedLogs []byte, err error) {
	payload, err := SerializeHttpLogs(logs)
	if err != nil {
		return nil, err
	}
	return payload, SendPayload(payload)
}
