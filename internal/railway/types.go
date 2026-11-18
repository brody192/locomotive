package railway

import (
	"github.com/hasura/go-graphql-client"
)

// Token is a Railway API token.
type Token string

type GraphQLClient struct {
	AuthToken           Token
	BaseSubscriptionURL string
	BaseURL             string
	Client              *graphql.Client
}
