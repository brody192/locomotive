package reconstruct_datadog

import (
	"github.com/brody192/locomotive/internal/railway/subscribe"
	"github.com/brody192/locomotive/internal/util"
)

func additionalFields(metadata map[string]string) map[string]any {
	hostname := util.SanitizeString(metadata[subscribe.MetadataKeyProjectName] + "-" + util.SanitizeString(metadata[subscribe.MetadataKeyEnvironmentName]))

	return map[string]any{
		"ddsource": "locomotive",
		"service":  util.SanitizeString(metadata[subscribe.MetadataKeyServiceName]),
		"hostname": hostname,
		"host":     hostname,
	}
}
