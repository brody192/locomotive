package railway

import (
	"fmt"
	"net/http"

	"github.com/flexstack/uuid"
)

// RailwayAuthHeader returns the header name and value Railway expects for a token.
//
// Railway authenticates account and workspace tokens with an `Authorization: Bearer`
// header. It authenticates a project token with a `Project-Access-Token` header carrying
// the bare token and no scheme. The two forms are not interchangeable: Railway rejects a
// project token sent as a bearer token, and the API reference documents both.
//
// A project token is scoped to one environment within one project, which is all this
// program needs to read a log stream. An account token can perform any action its owner
// can perform in every workspace they can reach.
func RailwayAuthHeader(token uuid.UUID, projectToken bool) (name string, value string) {
	if projectToken {
		return "Project-Access-Token", token.String()
	}

	return "Authorization", fmt.Sprintf("Bearer %s", token.String())
}

type authedTransport struct {
	token        uuid.UUID
	projectToken bool
	wrapped      http.RoundTripper
}

func (t *authedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set(RailwayAuthHeader(t.token, t.projectToken))
	req.Header.Set("Content-Type", "application/json")

	return t.wrapped.RoundTrip(req)
}
