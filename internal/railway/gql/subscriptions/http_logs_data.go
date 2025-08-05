package subscriptions

import (
	"encoding/json"

	"github.com/flexstack/uuid"
)

type HttpLogsSubscriptionPayload struct {
	Query     string                         `json:"query"`
	Variables *HttpLogsSubscriptionVariables `json:"variables"`
}

type HttpLogsSubscriptionVariables struct {
	AfterDate    *string   `json:"afterDate"`
	AnchorDate   *string   `json:"anchorDate"`
	BeforeDate   string    `json:"beforeDate"`
	BeforeLimit  int64     `json:"beforeLimit"`
	DeploymentId uuid.UUID `json:"deploymentId"`
	Filter       string    `json:"filter"`
}

type HttpLogsData struct {
	ID      uuid.UUID        `json:"id"`
	Type    SubscriptionType `json:"type"`
	Payload struct {
		Data struct {
			// we are keeping this as any because Railway may add or remove fields
			HTTPLogs []json.RawMessage `json:"httpLogs"`
		} `json:"data"`
	} `json:"payload"`
}
