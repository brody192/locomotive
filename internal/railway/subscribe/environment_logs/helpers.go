package environment_logs

import (
	"fmt"
	"strings"

	"github.com/brody192/locomotive/internal/logger"
	"github.com/flexstack/uuid"
)

func metadataName(metadataMap map[uuid.UUID]string, id uuid.UUID, label string) string {
	name, ok := metadataMap[id]
	if !ok {
		logger.Stdout.Warn(fmt.Sprintf("%s name could not be found", label))
		return "undefined"
	}

	return name
}

func buildServiceFilter(serviceIds []uuid.UUID) string {
	parts := make([]string, len(serviceIds))

	for i, serviceId := range serviceIds {
		parts[i] = "@service:" + serviceId.String()
	}

	return strings.Join(parts, " OR ")
}
