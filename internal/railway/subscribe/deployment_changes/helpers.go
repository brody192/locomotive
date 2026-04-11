package deployment_changes

import (
	"context"
	"slices"
	"time"

	"github.com/brody192/locomotive/internal/queue"
	"github.com/brody192/locomotive/internal/railway"
	"github.com/brody192/locomotive/internal/railway/gql/queries"
	"github.com/flexstack/uuid"
)

func fetchEnvironmentWithRetry(ctx context.Context, g *railway.GraphQLClient, environment *queries.EnvironmentData, variables map[string]any) error {
	fn := func(ctx context.Context) error {
		if err := g.Client.Exec(ctx, queries.EnvironmentQuery, environment, variables); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			return queue.Retryable(err)
		}

		return nil
	}

	return queue.RetryConstant(ctx,
		queue.Name("environment-data-fetch"),
		queue.MaxRetries(3600),
		queue.RetryInterval((1 * time.Second)),
		fn,
	)
}

func findSuccessfulDeploymentsIdsForWantedServiceIds(environment *queries.EnvironmentData, wantedServiceIds []uuid.UUID) []DeploymentIdWithInfo {
	successfulDeploymentsIdsForWantedServiceIds := []DeploymentIdWithInfo{}

	for _, deployment := range environment.Environment.Deployments.Edges {
		// Only consider successful deployments
		if deployment.Node.Status != "SUCCESS" {
			continue
		}

		// Only consider deployments for the specified trains
		if !slices.Contains(wantedServiceIds, deployment.Node.ServiceID) {
			continue
		}

		successfulDeploymentsIdsForWantedServiceIds = append(successfulDeploymentsIdsForWantedServiceIds, DeploymentIdWithInfo{
			ID:        deployment.Node.ID,
			CreatedAt: deployment.Node.CreatedAt,
		})
	}

	return successfulDeploymentsIdsForWantedServiceIds
}
