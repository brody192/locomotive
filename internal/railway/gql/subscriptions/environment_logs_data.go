package subscriptions

import (
	"time"

	"github.com/flexstack/uuid"
)

type EnvironmentLogsSubscriptionPayload struct {
	Query     string                                `json:"query"`
	Variables *EnvironmentLogsSubscriptionVariables `json:"variables"`
}

type EnvironmentLogsSubscriptionVariables struct {
	EnvironmentId uuid.UUID `json:"environmentId"`
	Filter        string    `json:"filter"`
	BeforeLimit   int64     `json:"beforeLimit"`
	BeforeDate    string    `json:"beforeDate"`
}

type EnvironmentLogsData struct {
	Payload struct {
		Data struct {
			EnvironmentLogs []EnvironmentLog `json:"environmentLogs"`
		} `json:"data"`
	} `json:"payload"`
	Type SubscriptionType `json:"type"`
}

type EnvironmentLog struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Severity  string    `json:"severity"`
	Tags      struct {
		ProjectID            uuid.UUID `json:"projectId"`
		EnvironmentID        uuid.UUID `json:"environmentId"`
		ServiceID            uuid.UUID `json:"serviceId"`
		DeploymentID         uuid.UUID `json:"deploymentId"`
		DeploymentInstanceID uuid.UUID `json:"deploymentInstanceId"`
	}
	Attributes []EnvironmentLogAttributes `json:"attributes"`
}

type EnvironmentLogAttributes struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
