package environment_logs

import (
	"context"
	"testing"
	"time"

	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
	"github.com/flexstack/uuid"
)

func TestReconnectRespectsBackboardMillisecondBounds(t *testing.T) {
	startedAt := time.Date(2026, 11, 11, 16, 40, 0, 123456789, time.UTC)
	queue := make(chan []EnvironmentLogWithMetadata, 1)
	forwarder := &serviceLogForwarder{startedAt: startedAt, newestForwarded: startedAt, metadataMap: map[uuid.UUID]string{{}: "test"}, logTrack: queue}
	newest := subscriptions.EnvironmentLog{Timestamp: startedAt.Add(time.Minute), Message: "newest"}
	boundary := subscriptions.EnvironmentLog{Timestamp: newest.Timestamp.Add(-30*time.Second - time.Nanosecond), Message: "boundary"}
	if err := forwarder.forward(context.Background(), logPayload(t, boundary, newest)); err != nil {
		t.Fatal(err)
	}
	queuedBatch(t, queue)
	payload := forwarder.subscriptionPayload(uuid.UUID{}, nil)
	requested, err := time.Parse(time.RFC3339Nano, payload.Variables.BeforeDate)
	if err != nil {
		t.Fatal(err)
	}
	// The GraphQL resolver passes beforeDate through JavaScript Date, which
	// truncates it to milliseconds before the ClickHouse query is constructed.
	wireBound := requested.Truncate(time.Millisecond)
	replay := []subscriptions.EnvironmentLog{}
	for _, log := range []subscriptions.EnvironmentLog{boundary, newest} {
		if log.Timestamp.After(wireBound) {
			replay = append(replay, log)
		}
	}
	if err := forwarder.forward(context.Background(), logPayload(t, replay...)); err != nil {
		t.Fatal(err)
	}
	if len(queue) != 0 {
		t.Fatal("Backboard's rounded lower bound replayed a forgotten boundary record")
	}
}
