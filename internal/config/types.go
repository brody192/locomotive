package config

import (
	"net/url"
	"time"

	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
	"github.com/flexstack/uuid"
)

type (
	AdditionalHeaders map[string]string

	WebhookMode string
)

type WebhookConfig struct {
	ExpectedHostContains string
	ExpectedHeaders      []string

	Headers AdditionalHeaders

	EnvironmentLogReconstructorFunc func([]environment_logs.EnvironmentLogWithMetadata) ([]byte, error)
	HTTPLogReconstructorFunc        func([]http_logs.DeploymentHttpLogWithMetadata) ([]byte, error)
}

type config struct {
	RailwayApiKey uuid.UUID   `env:"RAILWAY_API_KEY,required,notEmpty"`
	EnvironmentId uuid.UUID   `env:"ENVIRONMENT_ID,required,notEmpty"`
	ServiceIds    []uuid.UUID `env:"SERVICE_IDS,required,notEmpty"`

	WebhookUrl        url.URL           `env:"WEBHOOK_URL,required,notEmpty"`
	AdditionalHeaders AdditionalHeaders `env:"ADDITIONAL_HEADERS"`
	WebhookMode       WebhookMode       `env:"WEBHOOK_MODE" envDefault:"json"`

	ReportStatusEvery time.Duration `env:"REPORT_STATUS_EVERY" envDefault:"1m"`

	EnableHttpLogs   bool `env:"ENABLE_HTTP_LOGS" envDefault:"false"`
	EnableDeployLogs bool `env:"ENABLE_DEPLOY_LOGS" envDefault:"true"`
}
