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

// findSuccessfulDeploymentsIdsForWantedServiceIds returns the IDs of every successful
// deployment for the wanted services. All of them are tailed (not just the latest):
// during a rollover the previous deployment is still draining requests and emitting
// logs while the new one comes up, so tailing only the newest would drop those.
func findSuccessfulDeploymentsIdsForWantedServiceIds(environment *queries.EnvironmentData, wantedServiceIds []uuid.UUID) []uuid.UUID {
	deploymentIds := []uuid.UUID{}

	for _, deployment := range environment.Environment.Deployments.Edges {
		if deployment.Node.Status != "SUCCESS" {
			continue
		}

		if !slices.Contains(wantedServiceIds, deployment.Node.ServiceID) {
			continue
		}

		deploymentIds = append(deploymentIds, deployment.Node.ID)
	}

	return deploymentIds
}
