package http_logs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	cache "github.com/Code-Hex/go-generics-cache"
	"github.com/flexstack/uuid"
	"github.com/tidwall/gjson"

	"github.com/brody192/locomotive/internal/railway"
	"github.com/brody192/locomotive/internal/railway/gql/queries"
)

var metadataDeploymentCache = cache.New[uuid.UUID, DeploymentHttpLogMetadata]()

func getMetadataForDeployment(ctx context.Context, g *railway.GraphQLClient, deploymentId uuid.UUID) (DeploymentHttpLogMetadata, error) {
	if cached, ok := metadataDeploymentCache.Get(deploymentId); ok {
		return cached, nil
	}

	if g.Client == nil {
		return DeploymentHttpLogMetadata{}, errors.New("client is nil")
	}

	deployment := &queries.Deployment{}

	variables := map[string]any{
		"id": deploymentId,
	}

	if err := g.Client.Exec(ctx, queries.DeploymentQuery, &deployment, variables); err != nil {
		return DeploymentHttpLogMetadata{}, err
	}

	metadata := DeploymentHttpLogMetadata{}

	metadata["service_name"] = deployment.Deployment.Service.Name
	metadata["service_id"] = deployment.Deployment.Service.ID.String()

	metadata["environment_name"] = deployment.Deployment.Environment.Name
	metadata["environment_id"] = deployment.Deployment.Environment.ID.String()

	metadata["project_name"] = deployment.Deployment.Service.Project.Name
	metadata["project_id"] = deployment.Deployment.Service.Project.ID.String()

	metadata["deployment_id"] = deploymentId.String()

	metadataDeploymentCache.Set(deploymentId, metadata, cache.WithExpiration((10 * time.Minute)))

	return metadata, nil
}

// getTimeStampFromHttpLog is a helper function to get the timestamp from an HttpLog since we use the `any` type to keep it flexible
func getTimeStampAttributeFromHttpLog(h json.RawMessage) (time.Time, error) {
	timestampStr, err := getStringAttributeFromHttpLog(h, "timestamp")
	if err != nil {
		return time.Time{}, err
	}

	t, err := time.Parse(time.RFC3339Nano, timestampStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse timestamp '%s' as RFC3339Nano format: %w", timestampStr, err)
	}

	return t, nil
}

func getStringAttributeFromHttpLog(h json.RawMessage, attribute string) (string, error) {
	if h == nil {
		return "", fmt.Errorf("HttpLog is nil")
	}

	r := gjson.GetBytes(h, attribute)
	if !r.Exists() {
		return "", fmt.Errorf("attribute %s not found in the http log", attribute)
	}

	if r.Type != gjson.String {
		return "", fmt.Errorf("attribute %s is not a string in the http log", attribute)
	}

	return r.String(), nil
}

func getInt64AttributeFromHttpLog(h json.RawMessage, attribute string) (int64, error) {
	if h == nil {
		return 0, fmt.Errorf("HttpLog is nil")
	}

	r := gjson.GetBytes(h, attribute)
	if !r.Exists() {
		return 0, fmt.Errorf("attribute %s not found in the http log", attribute)
	}

	if r.Type != gjson.Number {
		return 0, fmt.Errorf("attribute %s is not a number in the http log", attribute)
	}

	return r.Int(), nil
}
