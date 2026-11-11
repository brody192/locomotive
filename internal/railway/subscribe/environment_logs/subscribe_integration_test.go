package environment_logs_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brody192/locomotive/internal/railway"
	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/coder/websocket"
	"github.com/flexstack/uuid"
)

func TestSubscribeToServiceLogsWebSocketReconnect(t *testing.T) {
	for _, ending := range []string{"complete", "eof", "deadline"} {
		t.Run(ending, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			projectID := uuid.Must(uuid.NewV4())
			environmentID := uuid.Must(uuid.NewV4())
			serviceID := uuid.Must(uuid.NewV4())
			otherServiceID := uuid.Must(uuid.NewV4())
			token := uuid.Must(uuid.NewV4())
			var connections, operations atomic.Int32
			var fixtureMu sync.Mutex
			var base time.Time
			var older, repeated subscriptions.EnvironmentLog
			errorsCh := make(chan error, 8)
			bounds := make(chan string, 4)
			expected := make(chan []subscriptions.EnvironmentLog, 1)
			firstSent := make(chan time.Time, 1)
			fail := func(err error) {
				if ctx.Err() == nil {
					select {
					case errorsCh <- err:
					default:
					}
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+token.String() {
					http.Error(w, "missing test token", http.StatusForbidden)
					return
				}
				if r.URL.Path == "/graphql" {
					var request struct{ Query string }
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						fail(err)
						return
					}
					var data any
					if strings.Contains(request.Query, "query environment(") {
						data = map[string]any{"environment": map[string]any{"projectId": projectID}}
					} else {
						data = map[string]any{"project": map[string]any{
							"id": projectID, "name": "test-project", "description": "",
							"environments": map[string]any{"edges": []any{map[string]any{"node": map[string]any{"id": environmentID, "name": "test-env"}}}},
							"services": map[string]any{"edges": []any{
								map[string]any{"node": map[string]any{"id": serviceID, "name": "first"}},
								map[string]any{"node": map[string]any{"id": otherServiceID, "name": "second"}},
							}},
						}}
					}
					w.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(w).Encode(map[string]any{"data": data}); err != nil {
						fail(err)
					}
					return
				}
				conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"graphql-transport-ws"}})
				if err != nil {
					fail(err)
					return
				}
				connections.Add(1)
				defer conn.CloseNow()
				_, init, err := conn.Read(ctx)
				if err != nil || string(init) != `{"type":"connection_init"}` {
					fail(fmt.Errorf("connection init: %q, %v", init, err))
					return
				}
				if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_ack"}`)); err != nil {
					fail(err)
					return
				}
				for {
					_, body, err := conn.Read(ctx)
					if err != nil {
						fail(err)
						return
					}
					var request struct {
						ID      string                                           `json:"id"`
						Type    string                                           `json:"type"`
						Payload subscriptions.EnvironmentLogsSubscriptionPayload `json:"payload"`
					}
					if err := json.Unmarshal(body, &request); err != nil {
						fail(err)
						return
					}
					if request.Type != "subscribe" || request.Payload.Variables == nil || request.Payload.Variables.BeforeLimit != 5000 || request.Payload.Variables.EnvironmentId != environmentID {
						fail(fmt.Errorf("unexpected subscription: %s", body))
						return
					}
					bounds <- request.Payload.Variables.BeforeDate
					write := func(logs ...subscriptions.EnvironmentLog) error {
						payload, err := json.Marshal(map[string]any{
							"id": request.ID, "type": "next",
							"payload": map[string]any{"data": map[string]any{"environmentLogs": logs}},
						})
						if err != nil {
							return err
						}
						return conn.Write(ctx, websocket.MessageText, payload)
					}
					operation := operations.Add(1)
					if operation == 1 {
						start, err := time.Parse(time.RFC3339Nano, request.Payload.Variables.BeforeDate)
						if err != nil {
							fail(err)
							return
						}
						fixtureMu.Lock()
						base = start.Truncate(time.Second).Add(123456789 * time.Nanosecond)
						older = subscriptions.EnvironmentLog{Timestamp: base.Add(50 * time.Second), Message: "older", Severity: "info"}
						older.Tags.ProjectID, older.Tags.EnvironmentID, older.Tags.ServiceID = projectID, environmentID, serviceID
						repeated = older
						repeated.Timestamp, repeated.Message = base.Add(time.Minute), "repeated"
						initial := []subscriptions.EnvironmentLog{older, repeated, repeated}
						fixtureMu.Unlock()
						firstSent <- time.Now()
						if err := write(initial...); err != nil {
							fail(err)
							return
						}
						switch ending {
						case "complete":
							complete, _ := json.Marshal(map[string]string{"id": request.ID, "type": "complete"})
							if err := conn.Write(ctx, websocket.MessageText, complete); err != nil {
								fail(err)
								return
							}
							continue
						case "deadline":
							conn.Close(websocket.StatusCode(4500), "deadline_exceeded")
							return
						default:
							return
						}
					}
					if operation != 2 {
						fail(fmt.Errorf("unexpected operation %d", operation))
						return
					}
					fixtureMu.Lock()
					late := older
					late.Timestamp, late.Message = base.Add(40*time.Second), "late"
					equal := repeated
					equal.Message = "different message"
					other := repeated
					other.Tags.ServiceID = otherServiceID
					want := []subscriptions.EnvironmentLog{older, repeated, repeated, late, repeated, equal, other}
					fixtureMu.Unlock()
					expected <- want
					if err := write(repeated); err != nil {
						fail(err)
						return
					}
					if err := write(older, repeated, late, repeated, equal, other); err != nil {
						fail(err)
						return
					}
					<-ctx.Done()
					return
				}
			}))
			defer server.Close()
			defer cancel()
			client, err := railway.NewClient(railway.AuthToken(token), railway.BaseURL(server.URL+"/graphql"), railway.BaseSubscriptionURL(strings.Replace(server.URL, "http://", "ws://", 1)+"/ws"))
			if err != nil {
				t.Fatal(err)
			}
			queue := make(chan []environment_logs.EnvironmentLogWithMetadata, 8)
			done := make(chan error, 1)
			go func() {
				done <- environment_logs.SubscribeToServiceLogs(ctx, client, queue, environmentID, []uuid.UUID{serviceID, otherServiceID})
			}()
			var delivered []subscriptions.EnvironmentLog
			var firstReceived time.Time
			for len(delivered) < 7 {
				select {
				case err := <-errorsCh:
					t.Fatal(err)
				case err := <-done:
					t.Fatalf("subscription stopped early: %v", err)
				case <-ctx.Done():
					t.Fatalf("received %d of 7 records: %v", len(delivered), ctx.Err())
				case batch := <-queue:
					if firstReceived.IsZero() {
						firstReceived = time.Now()
					}
					for _, log := range batch {
						if log.Metadata["project_name"] != "test-project" || log.Metadata["environment_name"] != "test-env" {
							t.Fatalf("metadata missing on forwarded log: %#v", log.Metadata)
						}
						delivered = append(delivered, log.Log)
					}
				}
			}
			key := func(log subscriptions.EnvironmentLog) string {
				return log.Timestamp.Format(time.RFC3339Nano) + "/" + log.Tags.ServiceID.String() + "/" + log.Message
			}
			counts := func(logs []subscriptions.EnvironmentLog) map[string]int {
				result := map[string]int{}
				for _, log := range logs {
					result[key(log)]++
				}
				return result
			}
			if want := <-expected; !reflect.DeepEqual(counts(delivered), counts(want)) {
				t.Fatalf("delivery differs: got %#v, want %#v", counts(delivered), counts(want))
			}
			<-bounds
			fixtureMu.Lock()
			wantBefore := base.Add(30 * time.Second).Truncate(time.Millisecond).Format(time.RFC3339Nano)
			fixtureMu.Unlock()
			if got := <-bounds; got != wantBefore {
				t.Fatalf("reconnect bound %q, want %q", got, wantBefore)
			}
			wantConnections := int32(2)
			if ending == "complete" {
				wantConnections = 1
			}
			if got := connections.Load(); got != wantConnections {
				t.Fatalf("got %d physical connections, want %d", got, wantConnections)
			}
			t.Logf("first batch forwarded %s after server write", firstReceived.Sub(<-firstSent))
			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("shutdown: %v", err)
			}
		})
	}
}
