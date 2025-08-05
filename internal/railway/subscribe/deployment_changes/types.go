package deployment_changes

import (
	"time"

	"github.com/flexstack/uuid"
)

type DeploymentIdWithInfo struct {
	ID        uuid.UUID
	CreatedAt time.Time
}
