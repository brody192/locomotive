package reconstruct_datadog

const timestampAttribute = "timestamp"

// We aren't including ddtags so that the application can log that info.
var reservedAttributes = []string{
	"ddsource",
	"service",
	"hostname",
	"host",
}
