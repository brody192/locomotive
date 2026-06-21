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

	return queue.RetryBackoff(ctx,
		queue.Name("environment-data-fetch"),
		queue.MaxRetries(120), // give up after ~1h of (capped) backoff, letting the subscription restart
		queue.InitialBackoff(1*time.Second),
		queue.MaxBackoff(30*time.Second),
		queue.BackoffMultiplier(2),
		queue.BackoffJitter(0.5),
		fn,
	)
}

// findSuccessfulDeploymentsIdsForWantedServiceIds returns the IDs of every successful
// deployment for the wanted services that also have a domain. All matching deployments
// are tailed (not just the latest): during a rollover the previous deployment is still
// draining requests and emitting logs while the new one comes up, so tailing only the
// newest would drop those.
//
// Only services with at least one domain (service or custom) receive HTTP traffic, so
// only they emit HTTP logs — there's nothing to tail for a domainless service. This set
// is recomputed on every environment change, so a domain added later brings its
// deployment in on the next refresh.
func findSuccessfulDeploymentsIdsForWantedServiceIds(environment *queries.EnvironmentData, wantedServiceIds []uuid.UUID) []uuid.UUID {
	servicesWithDomains := make(map[uuid.UUID]struct{})
	for _, instance := range environment.Environment.ServiceInstances.Edges {
		if len(instance.Node.Domains.ServiceDomains) > 0 || len(instance.Node.Domains.CustomDomains) > 0 {
			servicesWithDomains[instance.Node.ServiceID] = struct{}{}
		}
	}

	deploymentIds := []uuid.UUID{}

	for _, deployment := range environment.Environment.Deployments.Edges {
		if deployment.Node.Status != "SUCCESS" {
			continue
		}

		if !slices.Contains(wantedServiceIds, deployment.Node.ServiceID) {
			continue
		}

		if _, ok := servicesWithDomains[deployment.Node.ServiceID]; !ok {
			continue
		}

		deploymentIds = append(deploymentIds, deployment.Node.ID)
	}

	return deploymentIds
}
