package railway

import (
	"errors"
	"net/http"

	"github.com/flexstack/uuid"
	"github.com/hasura/go-graphql-client"
)

// Typed parameters for NewClient, mirroring queue.NewDispatcher: each is a distinct type
// so a NewClient(...) call is self-documenting, every field is required at compile time,
// and the arguments can't be passed in the wrong order.
type (
	authTokenParam           struct{ v uuid.UUID }
	baseURLParam             struct{ v string }
	baseSubscriptionURLParam struct{ v string }
)

// AuthToken is the Railway API token the client authenticates with.
func AuthToken(token uuid.UUID) authTokenParam { return authTokenParam{token} }

// BaseURL is the GraphQL HTTP endpoint.
func BaseURL(url string) baseURLParam { return baseURLParam{url} }

// BaseSubscriptionURL is the GraphQL WebSocket endpoint.
func BaseSubscriptionURL(url string) baseSubscriptionURLParam { return baseSubscriptionURLParam{url} }

// NewClient builds a GraphQLClient. Because each parameter is a distinct type (see
// queue.NewDispatcher), every field is required at compile time and can't be passed out
// of order.
func NewClient(
	authToken authTokenParam,
	baseURL baseURLParam,
	baseSubscriptionURL baseSubscriptionURLParam,
) (*GraphQLClient, error) {
	if authToken.v == uuid.Nil {
		return nil, errors.New("auth token must not be empty")
	}

	httpClient := &http.Client{
		Transport: &authedTransport{
			token:   authToken.v,
			wrapped: http.DefaultTransport,
		},
	}

	gqlClient := &GraphQLClient{
		AuthToken:           authToken.v,
		BaseURL:             baseURL.v,
		BaseSubscriptionURL: baseSubscriptionURL.v,
	}

	if gqlClient.BaseURL != "" {
		gqlClient.Client = graphql.NewClient(gqlClient.BaseURL, httpClient)
	}

	return gqlClient, nil
}
