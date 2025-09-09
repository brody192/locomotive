package reconstruct_loki

const lokiJSON string = `{"streams":[{"stream":{},"values": [[]]}]}`

var httpAttributesToSkip = []string{"timestamp", "path"}
