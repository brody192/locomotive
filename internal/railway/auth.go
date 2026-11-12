package railway

import (
	"fmt"
	"net/http"
)

type authedTransport struct {
	token   string
	wrapped http.RoundTripper
}

func (t *authedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.token))
	req.Header.Set("Content-Type", "application/json")

	return t.wrapped.RoundTrip(req)
}
