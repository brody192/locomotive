package generic

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"

	"github.com/brody192/locomotive/internal/config"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
)

var acceptedStatusCodes = []int{
	http.StatusOK,
	http.StatusNoContent,
	http.StatusAccepted,
	http.StatusCreated,
}

func SendWebhookForDeployLogs(logs []environment_logs.EnvironmentLogWithMetadata, client *http.Client) error {
	payload, err := config.WebhookModeToConfig[config.Global.WebhookMode].EnvironmentLogReconstructorFunc(logs)
	if err != nil {
		return fmt.Errorf("failed to reconstruct deploy log lines: %w", err)
	}

	return sendRawWebhook(payload, config.Global.WebhookUrl, config.Global.AdditionalHeaders, client)
}

func SendWebhookForHttpLogs(logs []http_logs.DeploymentHttpLogWithMetadata, client *http.Client) error {
	payload, err := config.WebhookModeToConfig[config.Global.WebhookMode].HTTPLogReconstructorFunc(logs)
	if err != nil {
		return fmt.Errorf("failed to reconstruct http log lines: %w", err)
	}

	return sendRawWebhook(payload, config.Global.WebhookUrl, config.Global.AdditionalHeaders, client)
}

func sendRawWebhook(logs []byte, url url.URL, additionalHeaders config.AdditionalHeaders, client *http.Client) error {
	req, err := http.NewRequest(http.MethodPost, url.String(), bytes.NewReader(logs))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Default headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Keep-Alive", "timeout=5, max=1000")

	for key, value := range config.WebhookModeToConfig[config.Global.WebhookMode].Headers {
		req.Header.Set(key, value)
	}

	for key, value := range additionalHeaders {
		req.Header.Set(key, value)
	}

	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook request: %w", err)
	}

	defer res.Body.Close()

	if !slices.Contains(acceptedStatusCodes, res.StatusCode) {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("non success status code: %d", res.StatusCode)
		}

		return fmt.Errorf("non success status code: %d; with body: %s", res.StatusCode, body)
	}

	return nil
}
