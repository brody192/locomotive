package deployment_changes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/brody192/locomotive/internal/logger"
	"github.com/brody192/locomotive/internal/railway"
	"github.com/brody192/locomotive/internal/railway/gql/queries"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_invalidation"
	"github.com/brody192/locomotive/internal/slice"
	"github.com/flexstack/uuid"
)

func SubscribeToDeploymentIdChanges(ctx context.Context, g *railway.GraphQLClient, deploymentIdSlice *slice.Sync[uuid.UUID], changeDetected chan<- struct{}, environmentId uuid.UUID, serviceIds []uuid.UUID) error {
	environment := &queries.EnvironmentData{}

	variables := map[string]any{
		"id": environmentId,
	}

	if err := fetchEnvironmentWithRetry(ctx, g, environment, variables); err != nil {
		return fmt.Errorf("error getting environment data: %w", err)
	}

	deploymentIdSlice.AppendMany(findSuccessfulDeploymentsIdsForWantedServiceIds(environment, serviceIds))

	select {
	case changeDetected <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}

	environmentHashTrack := make(chan string)
	errorChan := make(chan error, 1)

	go func() {
		if err := environment_invalidation.SubscribeToInvalidationRequests(ctx, g, environmentHashTrack, environmentId); err != nil {
			if errors.Is(err, context.Canceled) {
				errorChan <- ctx.Err()
				return
			}

			logger.Stderr.Error("error subscribing to invalidation requests",
				logger.ErrAttr(err),
			)

			errorChan <- err
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errorChan:
			return err
		case <-environmentHashTrack:
			environment := &queries.EnvironmentData{}

			if err := fetchEnvironmentWithRetry(ctx, g, environment, variables); err != nil {
				return fmt.Errorf("error getting environment data for new environment hash: %w", err)
			}

			latest := findSuccessfulDeploymentsIdsForWantedServiceIds(environment, serviceIds)

			if len(latest) == 0 {
				// Don't tear down everything if the environment momentarily reports none.
				continue
			}

			latestSet := make(map[uuid.UUID]struct{}, len(latest))
			for _, id := range latest {
				latestSet[id] = struct{}{}
			}

			current := deploymentIdSlice.Get()
			currentSet := make(map[uuid.UUID]struct{}, len(current))
			for _, id := range current {
				currentSet[id] = struct{}{}
			}

			deploymentsChanged := false

			// Add newly-successful deployments we aren't tracking yet.
			for _, id := range latest {
				if _, ok := currentSet[id]; !ok {
					deploymentIdSlice.Append(id)
					deploymentsChanged = true
				}
			}

			// Drop deployments no longer in the latest environment data.
			for _, id := range current {
				if _, ok := latestSet[id]; !ok {
					deploymentIdSlice.Delete(id)
					deploymentsChanged = true
				}
			}

			if deploymentsChanged {
				logger.Stdout.Debug("deployment id(s) changed for wanted service id(s)", slog.Any("deployment_ids", deploymentIdSlice.Get()))
				select {
				case changeDetected <- struct{}{}:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
}
