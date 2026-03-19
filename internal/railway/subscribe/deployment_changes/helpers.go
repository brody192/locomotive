package deployment_changes

import (
	"github.com/brody192/locomotive/internal/railway/gql/queries"
)

func findSuccessfulDeploymentsIdsForWantedServiceIds(environment *queries.EnvironmentData) []DeploymentIdWithInfo {
	successfulDeploymentsIdsForWantedServiceIds := []DeploymentIdWithInfo{}

	for _, deployment := range environment.Environment.Deployments.Edges {
		// Only consider successful deployments
		if deployment.Node.Status != "SUCCESS" {
			continue
		}

		successfulDeploymentsIdsForWantedServiceIds = append(successfulDeploymentsIdsForWantedServiceIds, DeploymentIdWithInfo{
			ID:        deployment.Node.ID,
			CreatedAt: deployment.Node.CreatedAt,
		})
	}

	return successfulDeploymentsIdsForWantedServiceIds
}
