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
	RailwayApiKey uuid.UUID   `env:"LOCOMOTIVE_RAILWAY_API_KEY,required,notEmpty"`
	EnvironmentId uuid.UUID   `env:"LOCOMOTIVE_ENVIRONMENT_ID,required,notEmpty"`
	ServiceIds    []uuid.UUID `env:"LOCOMOTIVE_SERVICE_IDS,required,notEmpty"`

	WebhookUrl        url.URL           `env:"LOCOMOTIVE_WEBHOOK_URL,required,notEmpty"`
	AdditionalHeaders AdditionalHeaders `env:"LOCOMOTIVE_ADDITIONAL_HEADERS"`
	WebhookMode       WebhookMode       `env:"LOCOMOTIVE_WEBHOOK_MODE" envDefault:"json"`

	ReportStatusEvery time.Duration `env:"LOCOMOTIVE_REPORT_STATUS_EVERY" envDefault:"1m"`

	EnableHttpLogs   bool `env:"LOCOMOTIVE_ENABLE_HTTP_LOGS" envDefault:"false"`
	EnableDeployLogs bool `env:"LOCOMOTIVE_ENABLE_DEPLOY_LOGS" envDefault:"true"`
}
