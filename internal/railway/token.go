package railway

import (
	"errors"
	"strings"

	"github.com/flexstack/uuid"
)

// ParseToken normalises a Railway API token read from configuration. Railway has issued
// tokens as UUIDs and, more recently, as prefixed strings (rlwy_acct_…, rlwy_wksp_…), so
// any non-UUID token is kept as is. A UUID token is written in its canonical lowercase
// form, as it was when the token was parsed as a UUID.
func ParseToken(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", errors.New("token must not be blank")
	}

	if u, err := uuid.FromString(token); err == nil {
		return u.String(), nil
	}

	return token, nil
}
