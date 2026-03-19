package queries

import "github.com/flexstack/uuid"

type ProjectsData struct {
	Projects struct {
		Edges []struct {
			Node struct {
				Environments struct {
					Edges []struct {
						Node struct {
							ID uuid.UUID `json:"id"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"environments"`
			} `json:"node"`
		} `json:"edges"`
	} `json:"projects"`
}
