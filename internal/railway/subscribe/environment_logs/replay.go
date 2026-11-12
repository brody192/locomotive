package environment_logs

import (
	"cmp"
	"crypto/sha256"
	"encoding/json"
	"slices"
	"time"

	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
)

const (
	reconnectOverlap = 30 * time.Second
	maxReplayRecords = 5000
)

type logFingerprint [sha256.Size]byte

type forwardedLog struct {
	timestamp time.Time
	count     int
}

type logReplayCache struct {
	floor     time.Time
	recent    map[logFingerprint]forwardedLog
	remaining map[logFingerprint]int
}

// fingerprintLog uses every returned field, without retaining message bodies.
// Normalize timestamps and attribute order so wire formatting cannot hide a replay.
func fingerprintLog(log subscriptions.EnvironmentLog) (logFingerprint, error) {
	log.Timestamp = log.Timestamp.UTC()
	attributes := make([]subscriptions.EnvironmentLogAttributes, len(log.Attributes))
	copy(attributes, log.Attributes)
	slices.SortFunc(attributes, func(a, b subscriptions.EnvironmentLogAttributes) int {
		if order := cmp.Compare(a.Key, b.Key); order != 0 {
			return order
		}
		return cmp.Compare(a.Value, b.Value)
	})
	log.Attributes = attributes
	payload, err := json.Marshal(log)
	if err != nil {
		return logFingerprint{}, err
	}
	return sha256.Sum256(payload), nil
}

// resumeBefore starts a new replay budget. Count occurrences rather than
// treating all identical payloads as one record. Live arrivals do not create
// replay budgets, so matching legitimate lines on a healthy stream still pass.
func (c *logReplayCache) resumeBefore(startedAt, newest time.Time) time.Time {
	c.prune(startedAt, newest)
	c.remaining = make(map[logFingerprint]int, len(c.recent))
	for key, entry := range c.recent {
		c.remaining[key] = entry.count
	}
	return c.floor
}

// Commit only after fresh records enter the forwarding queue. Batches that
// contain only replayed records can consume their replay budget immediately.
func (c *logReplayCache) commit(replayed map[logFingerprint]int, added map[logFingerprint]forwardedLog, startedAt, newest time.Time) {
	for key, count := range replayed {
		if left := c.remaining[key] - count; left > 0 {
			c.remaining[key] = left
		} else {
			delete(c.remaining, key)
		}
	}
	if len(added) > 0 && c.recent == nil {
		c.recent = make(map[logFingerprint]forwardedLog)
	}
	for key, addition := range added {
		entry := c.recent[key]
		entry.timestamp = addition.timestamp
		entry.count += addition.count
		c.recent[key] = entry
	}
	c.prune(startedAt, newest)
}

func (c *logReplayCache) prune(startedAt, newest time.Time) {
	// Backboard's GraphQL resolver converts beforeDate through JavaScript
	// Date, truncating it to milliseconds. Retain the whole boundary
	// millisecond so reconnects cannot request already forgotten records.
	floor := newest.Add(-reconnectOverlap).Truncate(time.Millisecond)
	if startupFloor := startedAt.Truncate(time.Millisecond); floor.Before(startupFloor) {
		floor = startupFloor
	}
	c.advanceFloor(floor)

	count := 0
	for _, entry := range c.recent {
		count += entry.count
	}
	if count <= maxReplayRecords {
		return
	}
	entries := make([]forwardedLog, 0, len(c.recent))
	for _, entry := range c.recent {
		entries = append(entries, entry)
	}
	slices.SortFunc(entries, func(a, b forwardedLog) int {
		return a.timestamp.Compare(b.timestamp)
	})
	for _, entry := range entries {
		floor = entry.timestamp.Truncate(time.Millisecond)
		if floor.Before(entry.timestamp) {
			floor = floor.Add(time.Millisecond)
		}
		count -= entry.count
		if count <= maxReplayRecords {
			break
		}
	}
	// Exclude whole millisecond groups when forgetting them. Otherwise a future
	// reconnect could request forgotten records and forward them a second time.
	c.advanceFloor(floor)
}

func (c *logReplayCache) advanceFloor(floor time.Time) {
	if floor.After(c.floor) {
		c.floor = floor
	}
	for key, entry := range c.recent {
		if !entry.timestamp.After(c.floor) {
			delete(c.recent, key)
		}
	}
	// Keep the current connection's replay budget even when recent records
	// expire: a copy already requested by that connection can still arrive.
}
