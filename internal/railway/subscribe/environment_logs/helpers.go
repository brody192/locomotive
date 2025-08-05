package environment_logs

import "github.com/flexstack/uuid"

// helper function to build a service filter string from provided service ids
func buildServiceFilter(serviceIds []uuid.UUID) string {
	var filterString string

	for i, serviceId := range serviceIds {
		filterString += "@service:" + serviceId.String()
		if i < len(serviceIds)-1 {
			filterString += " OR "
		}
	}

	return filterString
}
