package environment_logs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
	"github.com/flexstack/uuid"
)

func logPayload(t *testing.T, logs ...subscriptions.EnvironmentLog) []byte {
	t.Helper()
	data := subscriptions.EnvironmentLogsData{}
	data.Payload.Data.EnvironmentLogs = logs
	payload, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func queuedBatch(t *testing.T, queue <-chan []EnvironmentLogWithMetadata) []EnvironmentLogWithMetadata {
	t.Helper()
	select {
	case batch := <-queue:
		return batch
	default:
		t.Fatal("expected a forwarded batch")
		return nil
	}
}

func TestForwardAcceptsLateAndEqualTimestamps(t *testing.T) {
	startedAt := time.Date(2026, 11, 11, 16, 40, 0, 123456789, time.UTC)
	queue := make(chan []EnvironmentLogWithMetadata, 1)
	forwarder := &serviceLogForwarder{
		startedAt:       startedAt,
		newestForwarded: startedAt,
		metadataMap:     map[uuid.UUID]string{{}: "name"},
		logTrack:        queue,
	}

	newer := subscriptions.EnvironmentLog{Timestamp: startedAt.Add(time.Minute), Message: "newer"}
	if err := forwarder.forward(context.Background(), logPayload(t, newer)); err != nil {
		t.Fatal(err)
	}
	if batch := queuedBatch(t, queue); len(batch) != 1 || batch[0].Log.Message != "newer" {
		t.Fatalf("unexpected first batch: %#v", batch)
	}

	late := subscriptions.EnvironmentLog{Timestamp: startedAt.Add(10 * time.Second), Message: "late"}
	equal := subscriptions.EnvironmentLog{Timestamp: newer.Timestamp, Message: "same timestamp, different line"}
	beforeStartup := subscriptions.EnvironmentLog{Timestamp: startedAt.Add(-time.Nanosecond), Message: "old history"}
	atStartup := subscriptions.EnvironmentLog{Timestamp: startedAt, Message: "startup boundary"}
	if err := forwarder.forward(context.Background(), logPayload(t, beforeStartup, atStartup, late, equal, equal)); err != nil {
		t.Fatal(err)
	}
	batch := queuedBatch(t, queue)
	if len(batch) != 3 {
		t.Fatalf("want late line and both equal-timestamp occurrences, got %d lines", len(batch))
	}
	for i, want := range []string{late.Message, equal.Message, equal.Message} {
		if batch[i].Log.Message != want {
			t.Fatalf("line %d: got %q, want %q", i, batch[i].Log.Message, want)
		}
	}
	if !forwarder.newestForwarded.Equal(newer.Timestamp) {
		t.Fatal("late arrivals moved the reconnect position backwards")
	}
	wantResume := newer.Timestamp.Add(-30 * time.Second).Truncate(time.Millisecond)
	payload := forwarder.subscriptionPayload(uuid.UUID{}, nil)
	if payload.Variables.BeforeDate != wantResume.Format(time.RFC3339Nano) {
		t.Fatalf("got reconnect bound %q, want %q", payload.Variables.BeforeDate, wantResume.Format(time.RFC3339Nano))
	}
}

func TestForwardCancellationDoesNotAdvanceReconnectPosition(t *testing.T) {
	startedAt := time.Date(2026, 11, 11, 16, 40, 0, 123456789, time.UTC)
	queue := make(chan []EnvironmentLogWithMetadata, 1)
	forwarder := &serviceLogForwarder{
		startedAt: startedAt, newestForwarded: startedAt,
		metadataMap: map[uuid.UUID]string{{}: "name"}, logTrack: queue,
	}
	previous := subscriptions.EnvironmentLog{Timestamp: startedAt.Add(time.Minute), Message: "already queued"}
	if err := forwarder.forward(context.Background(), logPayload(t, previous)); err != nil {
		t.Fatal(err)
	}
	queuedBatch(t, queue)
	forwarder.subscriptionPayload(uuid.UUID{}, nil)
	forwarder.logTrack = make(chan []EnvironmentLogWithMetadata)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	next := subscriptions.EnvironmentLog{Timestamp: previous.Timestamp.Add(time.Second), Message: "not queued"}
	payload := logPayload(t, previous, next)
	err := forwarder.forward(ctx, payload)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context cancellation", err)
	}
	if !forwarder.newestForwarded.Equal(previous.Timestamp) {
		t.Fatal("a batch that was not queued advanced the reconnect position")
	}
	forwarder.logTrack = queue
	if err := forwarder.forward(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if batch := queuedBatch(t, queue); len(batch) != 1 || batch[0].Log.Message != next.Message {
		t.Fatalf("cancelled batch changed replay tracking: %#v", batch)
	}
}

func TestForwardSuppressesCountedReconnectReplay(t *testing.T) {
	startedAt := time.Date(2026, 11, 11, 16, 40, 0, 123456789, time.UTC)
	queue := make(chan []EnvironmentLogWithMetadata, 1)
	forwarder := &serviceLogForwarder{
		startedAt: startedAt, newestForwarded: startedAt,
		metadataMap: map[uuid.UUID]string{{}: "name", {15: 1}: "other-service"},
		logTrack:    queue,
	}
	older := subscriptions.EnvironmentLog{
		Timestamp: startedAt.Add(50 * time.Second), Message: "older",
		Attributes: []subscriptions.EnvironmentLogAttributes{{Key: "b", Value: "2"}, {Key: "a", Value: "1"}},
	}
	repeated := subscriptions.EnvironmentLog{Timestamp: startedAt.Add(time.Minute), Message: "repeated"}
	if err := forwarder.forward(context.Background(), logPayload(t, older, repeated, repeated)); err != nil {
		t.Fatal(err)
	}
	if batch := queuedBatch(t, queue); len(batch) != 3 {
		t.Fatalf("want all three original records, got %d", len(batch))
	}
	forwarder.subscriptionPayload(uuid.UUID{}, nil)
	if err := forwarder.forward(context.Background(), logPayload(t, repeated)); err != nil {
		t.Fatal(err)
	}
	if len(queue) != 0 {
		t.Fatal("a batch containing only replayed records was forwarded")
	}

	late := subscriptions.EnvironmentLog{Timestamp: startedAt.Add(40 * time.Second), Message: "late unseen line"}
	otherService := repeated
	otherService.Tags.ServiceID = uuid.UUID{15: 1}
	differentNano := repeated
	differentNano.Timestamp = repeated.Timestamp.Add(time.Nanosecond)
	if err := forwarder.forward(context.Background(), logPayload(t, repeated, late, otherService, differentNano)); err != nil {
		t.Fatal(err)
	}
	batch := queuedBatch(t, queue)
	if len(batch) != 3 || batch[0].Log.Message != late.Message || batch[1].Log.Tags.ServiceID != otherService.Tags.ServiceID || !batch[2].Log.Timestamp.Equal(differentNano.Timestamp) {
		t.Fatalf("want late line, distinct service and distinct nanosecond, got %#v", batch)
	}

	// Replay counts span batches. Attribute order does not change identity, and
	// a third identical occurrence beyond the two replayed copies is preserved.
	reordered := older
	reordered.Timestamp = older.Timestamp.In(time.FixedZone("offset", 3600))
	reordered.Attributes = []subscriptions.EnvironmentLogAttributes{older.Attributes[1], older.Attributes[0]}
	if err := forwarder.forward(context.Background(), logPayload(t, reordered, repeated)); err != nil {
		t.Fatal(err)
	}
	batch = queuedBatch(t, queue)
	if len(batch) != 1 || batch[0].Log.Message != repeated.Message {
		t.Fatalf("want only the additional occurrence, got %#v", batch)
	}
	// Once the replay budget is consumed, legitimate matching live records pass.
	if err := forwarder.forward(context.Background(), logPayload(t, repeated)); err != nil {
		t.Fatal(err)
	}
	if batch = queuedBatch(t, queue); len(batch) != 1 {
		t.Fatalf("want matching live occurrence, got %d records", len(batch))
	}
}

func TestReplayCacheEvictionNarrowsReconnectRange(t *testing.T) {
	startedAt := time.Date(2026, 11, 11, 16, 40, 0, 123456789, time.UTC)
	queue := make(chan []EnvironmentLogWithMetadata, 1)
	forwarder := &serviceLogForwarder{
		startedAt: startedAt, newestForwarded: startedAt,
		metadataMap: map[uuid.UUID]string{{}: "name"}, logTrack: queue,
	}
	logs := make([]subscriptions.EnvironmentLog, maxReplayRecords)
	for i := range logs {
		logs[i] = subscriptions.EnvironmentLog{Timestamp: startedAt.Add(time.Duration(i+1) * time.Millisecond), Message: "line"}
	}
	if err := forwarder.forward(context.Background(), logPayload(t, logs...)); err != nil {
		t.Fatal(err)
	}
	queuedBatch(t, queue)
	next := subscriptions.EnvironmentLog{Timestamp: startedAt.Add(time.Duration(maxReplayRecords+1) * time.Millisecond), Message: "new line"}
	if err := forwarder.forward(context.Background(), logPayload(t, next)); err != nil {
		t.Fatal(err)
	}
	queuedBatch(t, queue)
	payload := forwarder.subscriptionPayload(uuid.UUID{}, nil)
	if payload.Variables.BeforeDate != logs[0].Timestamp.Truncate(time.Millisecond).Add(time.Millisecond).Format(time.RFC3339Nano) {
		t.Fatalf("forgotten record remains in reconnect range: %q", payload.Variables.BeforeDate)
	}
	if len(forwarder.replay.recent) > maxReplayRecords || len(forwarder.replay.remaining) > maxReplayRecords {
		t.Fatal("replay cache exceeded its record budget")
	}
	// Cache eviction must not reintroduce a moving cutoff for live arrivals.
	late := subscriptions.EnvironmentLog{Timestamp: startedAt.Add(time.Nanosecond), Message: "unseen late line"}
	if err := forwarder.forward(context.Background(), logPayload(t, late)); err != nil {
		t.Fatal(err)
	}
	if batch := queuedBatch(t, queue); len(batch) != 1 || batch[0].Log.Message != late.Message {
		t.Fatalf("late line was discarded after cache eviction: %#v", batch)
	}
	newer := next
	newer.Timestamp = next.Timestamp.Add(time.Minute)
	if err := forwarder.forward(context.Background(), logPayload(t, newer)); err != nil {
		t.Fatal(err)
	}
	queuedBatch(t, queue)
	// Expiring the recent cache must not forget copies this connection already requested.
	if err := forwarder.forward(context.Background(), logPayload(t, logs[1])); err != nil {
		t.Fatal(err)
	}
	if len(queue) != 0 {
		t.Fatal("an outstanding replay was forwarded after its recent cache entry expired")
	}
}
