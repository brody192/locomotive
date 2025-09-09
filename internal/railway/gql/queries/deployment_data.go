package queries

import "github.com/flexstack/uuid"

type Deployment struct {
	Deployment struct {
		Service struct {
			Name    string    `json:"name"`
			ID      uuid.UUID `json:"id"`
			Project struct {
				Name string    `json:"name"`
				ID   uuid.UUID `json:"id"`
			} `json:"project"`
		} `json:"service"`
		Environment struct {
			Name string    `json:"name"`
			ID   uuid.UUID `json:"id"`
		} `json:"environment"`
	} `json:"deployment"`
}
