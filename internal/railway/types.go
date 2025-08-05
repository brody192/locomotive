package railway

import (
	"github.com/flexstack/uuid"
	"github.com/hasura/go-graphql-client"
)

type GraphQLClient struct {
	AuthToken           uuid.UUID
	BaseSubscriptionURL string
	BaseURL             string
	Client              *graphql.Client
}
