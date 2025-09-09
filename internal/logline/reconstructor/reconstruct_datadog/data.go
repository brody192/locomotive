package reconstruct_datadog

// We aren't including ddtags so that the application can log that info.
var reservedAttributes = []string{
	"ddsource",
	"service",
	"hostname",
	"host",
}
