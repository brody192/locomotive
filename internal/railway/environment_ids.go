package railway

import (
	"context"
	"fmt"

	"github.com/brody192/locomotive/internal/railway/gql/queries"
	"github.com/flexstack/uuid"
)

func FetchAllEnvironmentIDs(ctx context.Context, g *GraphQLClient) ([]uuid.UUID, error) {
	if g == nil || g.Client == nil {
		return nil, fmt.Errorf("graphql client is nil")
	}

	projectData := &queries.ProjectsData{}

	if err := g.Client.Exec(ctx, queries.ProjectsQuery, &projectData, nil); err != nil {
		return nil, err
	}

	environmentIDs := make([]uuid.UUID, 0)
	seen := make(map[uuid.UUID]struct{})

	for _, project := range projectData.Projects.Edges {
		for _, environment := range project.Node.Environments.Edges {
			if _, ok := seen[environment.Node.ID]; ok {
				continue
			}

			seen[environment.Node.ID] = struct{}{}
			environmentIDs = append(environmentIDs, environment.Node.ID)
		}
	}

	return environmentIDs, nil
}
