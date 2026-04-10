package config

import (
	"regexp"

	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_axiom"
	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_betterstack"
	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_datadog"
	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_json"
	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_loki"
	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_otel"
	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_papertrail"
	"github.com/brody192/locomotive/internal/logline/reconstructor/reconstruct_sentry"
)

var schemeRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`)

const (
	WebhookModeJson        WebhookMode = "json"
	WebhookModeJsonl       WebhookMode = "jsonl"
	WebhookModePapertrail  WebhookMode = "papertrail"
	WebhookModeDatadog     WebhookMode = "datadog"
	WebhookModeAxiom       WebhookMode = "axiom"
	WebhookModeBetterstack WebhookMode = "betterstack"
	WebhookModeLoki        WebhookMode = "loki"
	WebhookModeSentry      WebhookMode = "sentry"
	WebhookModeOtelHTTP    WebhookMode = "otel_http"
	WebhookModeSignoz      WebhookMode = "signoz"

	DefaultWebhookMode = WebhookModeJson
)

var WebhookModeToConfig = map[WebhookMode]WebhookConfig{
	WebhookModeJson: {
		Headers:                         map[string]string{},
		EnvironmentLogReconstructorFunc: reconstruct_json.EnvironmentLogsJsonArray,
		HTTPLogReconstructorFunc:        reconstruct_json.HttpLogsJsonArray,
	},
	WebhookModeJsonl: {
		Headers: map[string]string{
			"Content-Type": "application/json-lines",
		},
		EnvironmentLogReconstructorFunc: reconstruct_json.EnvironmentLogsJsonLines,
		HTTPLogReconstructorFunc:        reconstruct_json.HttpLogsJsonLines,
	},
	WebhookModeLoki: {
		ExpectedHostContains:            []string{"loki", "grafana"},
		Headers:                         map[string]string{},
		EnvironmentLogReconstructorFunc: reconstruct_loki.EnvironmentLogStreams,
		HTTPLogReconstructorFunc:        reconstruct_loki.HttpLogStreams,
	},
	WebhookModePapertrail: {
		ExpectedHostContains: []string{"solarwinds"},
		ExpectedHeaders:      []string{"Authorization"},
		Headers: map[string]string{
			// Note: Papertrail accepts JSON Lines, yet only accepts the JSON content type.
			"Content-Type": "application/json",
		},
		EnvironmentLogReconstructorFunc: reconstruct_papertrail.EnvironmentLogsJsonLines,
		HTTPLogReconstructorFunc:        reconstruct_papertrail.HttpLogsJsonLines,
	},
	WebhookModeDatadog: {
		ExpectedHostContains:            []string{"datadog"},
		ExpectedHeaders:                 []string{"DD-API-KEY", "DD-APPLICATION-KEY"},
		Headers:                         map[string]string{},
		EnvironmentLogReconstructorFunc: reconstruct_datadog.EnvironmentLogsJsonArray,
		HTTPLogReconstructorFunc:        reconstruct_datadog.HttpLogsJsonArray,
	},
	WebhookModeAxiom: {
		ExpectedHostContains:            []string{"axiom"},
		ExpectedHeaders:                 []string{"Authorization"},
		Headers:                         map[string]string{},
		EnvironmentLogReconstructorFunc: reconstruct_axiom.EnvironmentLogsJsonArray,
		HTTPLogReconstructorFunc:        reconstruct_axiom.HttpLogsJsonArray,
	},
	WebhookModeBetterstack: {
		ExpectedHostContains:            []string{"betterstack"},
		ExpectedHeaders:                 []string{"Authorization"},
		Headers:                         map[string]string{},
		EnvironmentLogReconstructorFunc: reconstruct_betterstack.EnvironmentLogsJsonArray,
		HTTPLogReconstructorFunc:        reconstruct_betterstack.HttpLogsJsonArray,
	},
	WebhookModeSentry: {
		ExpectedHostContains: []string{"sentry"},
		ExpectedHeaders:      []string{"X-Sentry-Auth"},
		Headers: map[string]string{
			"Content-Type": "application/x-sentry-envelope",
		},
		EnvironmentLogReconstructorFunc: reconstruct_sentry.EnvironmentLogsEnvelope,
		HTTPLogReconstructorFunc:        reconstruct_sentry.HttpLogsEnvelope,
	},
	WebhookModeOtelHTTP: {
		Headers:                         map[string]string{},
		EnvironmentLogReconstructorFunc: reconstruct_otel.EnvironmentLogsOtel,
		HTTPLogReconstructorFunc:        reconstruct_otel.HttpLogsOtel,
	},
	WebhookModeSignoz: {
		ExpectedHostContains:            []string{"signoz.cloud"},
		ExpectedHeaders:                 []string{"signoz-ingestion-key"},
		Headers:                         map[string]string{},
		EnvironmentLogReconstructorFunc: reconstruct_otel.EnvironmentLogsOtel,
		HTTPLogReconstructorFunc:        reconstruct_otel.HttpLogsOtel,
	},
}
