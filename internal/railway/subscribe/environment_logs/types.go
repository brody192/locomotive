package environment_logs

import (
	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
)

type EnvironmentLogMetadata map[string]string
type EnvironmentLogWithMetadata struct {
	Log      subscriptions.EnvironmentLog
	Metadata EnvironmentLogMetadata
}
