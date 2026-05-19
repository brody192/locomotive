package reconstruct_victorialogs

// https://docs.victoriametrics.com/victorialogs/data-ingestion/#http-parameters
//
// VictoriaLogs' JSON stream ingestion API expects each line to be a JSON
// object. By default it looks for the log message under "_msg" and the
// timestamp under "_time" (these can be remapped via _msg_field / _time_field
// query parameters, but we use the defaults to keep configuration minimal).

const (
	timestampAttribute = "_time"
	messageAttribute   = "_msg"
)
