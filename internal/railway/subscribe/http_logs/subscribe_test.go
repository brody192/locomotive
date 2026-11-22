package http_logs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/brody192/locomotive/internal/railway"
	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
	"github.com/coder/websocket"
	"github.com/flexstack/uuid"
)

func TestHTTPLogsForwardReplayAcrossReconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	startedAt := time.Now().UTC().Truncate(time.Millisecond).Add(123456 * time.Nanosecond)
	newest := startedAt.Add(2 * time.Minute)
	deploymentID := uuid.Must(uuid.NewV4())
	metadataDeploymentCache.Set(deploymentID, DeploymentHttpLogMetadata{})
	t.Cleanup(func() { metadataDeploymentCache.Delete(deploymentID) })

	newLog := func(id string, timestamp time.Time) map[string]any {
		return map[string]any{"requestId": id, "timestamp": timestamp, "path": "/", "httpStatus": 200}
	}
	initial := newLog("initial", newest)
	firstWithoutID := newLog(missingHTTPRequestID, newest)
	firstWithoutID["path"] = "/first"
	secondWithoutID := newLog(missingHTTPRequestID, newest)
	secondWithoutID["path"] = "/second"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"graphql-transport-ws"}})
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		if _, _, err := conn.Read(ctx); err != nil {
			t.Error(err)
			return
		}
		if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_ack"}`)); err != nil {
			t.Error(err)
			return
		}
		readSubscription := func(want time.Time) (string, bool) {
			var request struct {
				ID      string                                    `json:"id"`
				Payload subscriptions.HttpLogsSubscriptionPayload `json:"payload"`
			}
			_, message, err := conn.Read(ctx)
			if err != nil {
				t.Error(err)
				return "", false
			}
			if err := json.Unmarshal(message, &request); err != nil {
				t.Error(err)
				return "", false
			}
			if request.Payload.Variables == nil || request.Payload.Variables.BeforeDate != want.Format(time.RFC3339Nano) {
				t.Errorf("wanted exclusive bound %s, got %s", want, message)
				return "", false
			}
			return request.ID, true
		}
		write := func(message any) bool {
			body, err := json.Marshal(message)
			if err == nil {
				err = conn.Write(ctx, websocket.MessageText, body)
			}
			if err != nil {
				t.Error(err)
			}
			return err == nil
		}
		sendLogs := func(id string, logs ...map[string]any) bool {
			return write(map[string]any{
				"id": id, "type": "next",
				"payload": map[string]any{"data": map[string]any{"httpLogs": logs}},
			})
		}

		id, ok := readSubscription(startedAt.Truncate(time.Millisecond))
		if !ok || !sendLogs(id, newLog("before-start", startedAt.Add(-time.Nanosecond)), initial, firstWithoutID) {
			return
		}
		// Backboard can replay while the original socket is still open.
		older := newLog("older", newest.Add(-time.Second))
		if !sendLogs(id, older, initial) || !write(map[string]string{"id": id, "type": "complete"}) {
			return
		}
		id, ok = readSubscription(newest.Add(-httpLogsReconnectOverlap).Truncate(time.Millisecond))
		if !ok {
			return
		}
		equal := newLog("equal", newest)
		sendLogs(id, newLog("missed", newest.Add(-2*time.Second)), older, initial, equal, equal, firstWithoutID, secondWithoutID)
		<-ctx.Done()
	}))
	defer func() {
		cancel()
		server.Close()
	}()

	client := &railway.GraphQLClient{BaseSubscriptionURL: "ws" + strings.TrimPrefix(server.URL, "http")}
	track := make(chan []DeploymentHttpLogWithMetadata, 3)
	done := make(chan error, 1)
	go func() { done <- getHttpLogs(ctx, client, deploymentID, startedAt, 0, track) }()
	for _, want := range [][]string{{"initial", "/first"}, {"older"}, {"missed", "equal", "/second"}} {
		select {
		case batch := <-track:
			var got []string
			for _, log := range batch {
				id, err := getStringAttributeFromHttpLog(log.Log, "requestId")
				if err != nil {
					t.Fatal(err)
				}
				if id == missingHTTPRequestID {
					id = log.Path
				}
				got = append(got, id)
			}
			if !slices.Equal(got, want) {
				t.Fatalf("forwarded %v, want %v", got, want)
			}
		case err := <-done:
			t.Fatalf("subscription ended early: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatalf("subscription ended with %v, want cancellation", err)
	}
}
