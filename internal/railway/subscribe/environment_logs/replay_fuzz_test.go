package environment_logs

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
	"github.com/flexstack/uuid"
)

// The oracle tracks records by their fixture identity, independently of the
// production fingerprint and replay cache. Each reconnect replays the records
// its requested time range includes, mixed with previously unseen arrivals.
func FuzzForwardReconnectReplay(f *testing.F) {
	f.Add(uint64(1))
	f.Add(uint64(42))
	f.Add(uint64(123456789))
	f.Fuzz(func(t *testing.T, seed uint64) {
		random := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
		startedAt := time.Date(2026, 11, 11, 16, 40, 0, 123456789, time.UTC)
		queue := make(chan []EnvironmentLogWithMetadata, 1)
		forwarder := &serviceLogForwarder{startedAt: startedAt, newestForwarded: startedAt, metadataMap: map[uuid.UUID]string{{}: "test"}, logTrack: queue}
		history := []subscriptions.EnvironmentLog{}
		delivered := map[string]bool{}
		for step := 0; step < 40; step++ {
			batch := []subscriptions.EnvironmentLog{}
			if random.IntN(3) == 0 {
				payload := forwarder.subscriptionPayload(uuid.UUID{}, nil)
				before, err := time.Parse(time.RFC3339Nano, payload.Variables.BeforeDate)
				if err != nil {
					t.Fatal(err)
				}
				for _, log := range history {
					if log.Timestamp.After(before) {
						batch = append(batch, log)
					}
				}
			}
			count := 1 + random.IntN(10)
			for i := 0; i < count; i++ {
				offset := time.Duration(1+step*2-random.IntN(20)) * time.Second
				if offset <= 0 {
					offset = time.Nanosecond
				}
				log := subscriptions.EnvironmentLog{Timestamp: startedAt.Add(offset), Message: fmt.Sprintf("record-%d", len(history)), Severity: "info", Attributes: []subscriptions.EnvironmentLogAttributes{{Key: "seed", Value: fmt.Sprint(seed)}}}
				history = append(history, log)
				batch = append(batch, log)
			}
			random.Shuffle(len(batch), func(i, j int) { batch[i], batch[j] = batch[j], batch[i] })
			for len(batch) > 0 {
				size := min(len(batch), 1+random.IntN(17))
				if err := forwarder.forward(context.Background(), logPayload(t, batch[:size]...)); err != nil {
					t.Fatal(err)
				}
				select {
				case fresh := <-queue:
					for _, log := range fresh {
						if delivered[log.Log.Message] {
							t.Fatalf("duplicate %s at step %d", log.Log.Message, step)
						}
						delivered[log.Log.Message] = true
					}
				default:
				}
				batch = batch[size:]
			}
			if len(delivered) != len(history) {
				t.Fatalf("step %d: delivered %d of %d records", step, len(delivered), len(history))
			}
		}
	})
}
