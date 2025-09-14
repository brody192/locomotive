package railway

import (
	"context"

	"github.com/brody192/locomotive/internal/railway/gql/queries"
	"github.com/flexstack/uuid"
)

func VerifyAllServicesExistWithinEnvironment(g *GraphQLClient, services []uuid.UUID, environmentID uuid.UUID) (bool, []uuid.UUID, []uuid.UUID, error) {
	environment := &queries.EnvironmentData{}

	variables := map[string]any{
		"id": environmentID,
	}

	if err := g.Client.Exec(context.Background(), queries.EnvironmentQuery, &environment, variables); err != nil {
		return false, nil, nil, err
	}

	foundServices := []uuid.UUID{}
	missingServices := []uuid.UUID{}

	for _, service := range services {
		found := false

		for _, edge := range environment.Environment.Deployments.Edges {
			if edge.Node.ServiceID == service {
				found = true
				break
			}
		}

		if found {
			foundServices = append(foundServices, service)
		} else {
			missingServices = append(missingServices, service)
		}
	}

	return (len(missingServices) == 0), foundServices, missingServices, nil
}
