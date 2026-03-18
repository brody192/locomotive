package reconstruct_loki

const (
	lokiJSON   string = `{"streams":[]}`
	streamJSON string = `{"stream":{},"values":[[]]}`
)

var httpAttributesToSkip = []string{"timestamp", "path"}
