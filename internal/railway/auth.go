package railway

import (
	"fmt"
	"net/http"

	"github.com/flexstack/uuid"
)

type authedTransport struct {
	token   uuid.UUID
	wrapped http.RoundTripper
}

func (t *authedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.token.String()))
	req.Header.Set("Content-Type", "application/json")

	return t.wrapped.RoundTrip(req)
}
