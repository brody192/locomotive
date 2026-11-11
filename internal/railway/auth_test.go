package railway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Railway has issued tokens as UUIDs and, more recently, as prefixed strings (rlwy_acct_…,
// rlwy_wksp_…); each must reach the API exactly as given.
var tokenCases = []struct {
	name  string
	token string
	want  string
}{
	{"legacy uuid token", "0b3f6a52-7c1e-4d7a-9a51-2f4c8e9d1a6b", "Bearer 0b3f6a52-7c1e-4d7a-9a51-2f4c8e9d1a6b"},
	{"account token", "rlwy_acct_3kQ9xV7mN2pL8rT4wY6zB1cD5fG0hJ", "Bearer rlwy_acct_3kQ9xV7mN2pL8rT4wY6zB1cD5fG0hJ"},
	{"workspace token", "rlwy_wksp_8nR2tY5uI9oP3aS7dF1gH4jK6lZ0xC", "Bearer rlwy_wksp_8nR2tY5uI9oP3aS7dF1gH4jK6lZ0xC"},
}

// captureAuthorization starts a server that records the Authorization header of the
// first request and answers with the given status.
func captureAuthorization(t *testing.T, status int) (*httptest.Server, <-chan string) {
	t.Helper()

	got := make(chan string, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case got <- r.Header.Get("Authorization"):
		default:
		}

		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	t.Cleanup(srv.Close)

	return srv, got
}

func awaitHeader(t *testing.T, got <-chan string) string {
	t.Helper()

	select {
	case header := <-got:
		return header
	case <-time.After(5 * time.Second):
		t.Fatal("no request reached the server")
		return ""
	}
}

func TestHTTPRequestsAuthenticateWithTheTokenAsIssued(t *testing.T) {
	for _, tc := range tokenCases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := captureAuthorization(t, http.StatusOK)

			client, err := NewClient(AuthToken(tc.token), BaseURL(srv.URL), BaseSubscriptionURL(""))
			if err != nil {
				t.Fatalf("NewClient returned error: %v", err)
			}

			var res struct{}
			_ = client.Client.Exec(context.Background(), "query { __typename }", &res, nil)

			if header := awaitHeader(t, got); header != tc.want {
				t.Errorf("Authorization header = %q, want %q", header, tc.want)
			}
		})
	}
}

func TestWebSocketHandshakeAuthenticatesWithTheTokenAsIssued(t *testing.T) {
	for _, tc := range tokenCases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := captureAuthorization(t, http.StatusUnauthorized)

			client, err := NewClient(
				AuthToken(tc.token),
				BaseURL(""),
				BaseSubscriptionURL("ws"+strings.TrimPrefix(srv.URL, "http")),
			)
			if err != nil {
				t.Fatalf("NewClient returned error: %v", err)
			}

			if _, err := client.CreateWebSocketSubscription(context.Background(), struct{}{}); err == nil {
				t.Fatal("expected the rejected handshake to return an error")
			}

			if header := awaitHeader(t, got); header != tc.want {
				t.Errorf("Authorization header = %q, want %q", header, tc.want)
			}
		})
	}
}

func TestNewClientRejectsAnEmptyToken(t *testing.T) {
	_, err := NewClient(AuthToken(""), BaseURL("http://localhost"), BaseSubscriptionURL(""))
	if err == nil || err.Error() != "auth token must not be empty" {
		t.Errorf("NewClient(AuthToken(\"\")) error = %v, want \"auth token must not be empty\"", err)
	}
}
