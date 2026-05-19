package reconstruct_json

type Config struct {
	TimestampAttribute   string
	MessageAttribute     string
	ReserverdAttributes  []string
	AdditionalFieldsFunc func(metadata map[string]string) map[string]any
}
