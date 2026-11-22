package http_logs

import (
	"crypto/sha256"
	"encoding/json"
	"maps"
	"slices"
	"time"
)

const (
	httpLogsReconnectOverlap = time.Minute
	httpLogsBatchLimit       = 5000
	// Backboard encodes the 16 zero bytes used for missing request IDs as base64url.
	missingHTTPRequestID = "AAAAAAAAAAAAAAAAAAAAAA"
)

type httpLogKey [sha256.Size]byte

func requestKey(log json.RawMessage) httpLogKey {
	if id, _ := getStringAttributeFromHttpLog(log, "requestId"); id != "" && id != missingHTTPRequestID {
		return sha256.Sum256([]byte(id))
	}
	return sha256.Sum256(log)
}

type httpLogReplay struct {
	floor  time.Time
	newest time.Time
	recent map[httpLogKey]time.Time
	// Keep the previous stream's IDs for this connection even if fresher
	// deliveries push them out of recent before the replay arrives.
	reconnecting map[httpLogKey]time.Time
}

func (r *httpLogReplay) resumeBefore() time.Time {
	r.reconnecting = maps.Clone(r.recent)
	return r.floor
}

func (r *httpLogReplay) seen(key httpLogKey) bool {
	_, recent := r.recent[key]
	_, reconnecting := r.reconnecting[key]
	return recent || reconnecting
}

func (r *httpLogReplay) commit(added map[httpLogKey]time.Time, newest time.Time) {
	r.newest = newest
	// GraphQL truncates beforeDate to milliseconds. Keep any IDs the
	// truncated exclusive lower bound could still include.
	r.advanceFloor(newest.Add(-httpLogsReconnectOverlap).Truncate(time.Millisecond))
	for key, timestamp := range added {
		if timestamp.After(r.floor) {
			r.recent[key] = timestamp
		}
	}
	if len(r.recent) <= httpLogsBatchLimit {
		return
	}

	timestamps := slices.Collect(maps.Values(r.recent))
	slices.SortFunc(timestamps, time.Time.Compare)
	cutoff := timestamps[len(timestamps)-httpLogsBatchLimit-1]
	// Round up so a reconnect cannot replay an evicted ID from that millisecond.
	floor := cutoff.Truncate(time.Millisecond)
	if floor.Before(cutoff) {
		floor = floor.Add(time.Millisecond)
	}
	r.advanceFloor(floor)
}

func (r *httpLogReplay) advanceFloor(floor time.Time) {
	if !floor.After(r.floor) {
		return
	}
	r.floor = floor
	for key, timestamp := range r.recent {
		if !timestamp.After(floor) {
			delete(r.recent, key)
		}
	}
}
