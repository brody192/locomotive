package reconstruct_sentry

const (
	LineOne   string = `{"event_id":"","sent_at":"","sdk":{"name":"locomotive","version":"2.0.0"}}`
	LineTwo   string = `{"type":"log","item_count":1,"content_type":"application/vnd.sentry.items.log+json"}`
	LineThree string = `{"event_id":"","level":"info","platform":"go","sdk":{"name":"locomotive","version":"2.0.0","integrations":[],"packages":[]},"server_name":"locomotive","user":{},"modules":{},"items":[],"timestamp":""}`
	Item      string = `{"timestamp":"","trace_id":"","level":"","severity_number":9,"body":"","attributes":{"sentry.sdk.name":{"value":"locomotive","type":"string"},"sentry.sdk.version":{"value":"2.0.0","type":"string"},"sentry.server.address":{"value":"locomotive","type":"string"}}}`
)
