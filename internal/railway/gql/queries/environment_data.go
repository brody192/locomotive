package queries

import (
	"time"

	"github.com/flexstack/uuid"
)

type EnvironmentData struct {
	Environment struct {
		Deployments struct {
			Edges []struct {
				Node struct {
					ServiceID uuid.UUID `json:"serviceId"`
					ProjectID uuid.UUID `json:"projectId"`
					Status    string    `json:"status"`
					CreatedAt time.Time `json:"createdAt"`
					ID        uuid.UUID `json:"id"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"deployments"`
		ServiceInstances struct {
			Edges []struct {
				Node struct {
					ServiceID uuid.UUID `json:"serviceId"`
					Domains   struct {
						ServiceDomains []struct {
							Domain string `json:"domain"`
						} `json:"serviceDomains"`
						CustomDomains []struct {
							Domain string `json:"domain"`
						} `json:"customDomains"`
					} `json:"domains"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"serviceInstances"`
		ProjectID uuid.UUID `json:"projectId"`
	} `json:"environment"`
}
