package reconstruct_otel

import (
	"testing"
	"time"

	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/tidwall/gjson"
)

func TestEnvironmentLogsOtel_shouldDecodeJSONEncodedAttributeValues(t *testing.T) {
	tests := []struct {
		name     string
		rawValue string
		want     string
	}{
		{"quoted string", `"hello"`, "hello"},
		{"escaped slash", `"http:\/\/x"`, "http://x"},
		{"unquoted number", "42", "42"},
		{"Go single-quoted rune", `'a'`, `'a'`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := []environment_logs.EnvironmentLogWithMetadata{{
				Log: subscriptions.EnvironmentLog{
					Timestamp:  time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC),
					Message:    "hello",
					Severity:   "info",
					Attributes: []subscriptions.EnvironmentLogAttributes{{Key: "field", Value: tt.rawValue}},
				},
			}}

			out, err := EnvironmentLogsOtel(logs)
			if err != nil {
				t.Fatalf("EnvironmentLogsOtel returned error: %v", err)
			}

			path := `resourceLogs.0.scopeLogs.0.logRecords.0.attributes.#(key=="field").value.stringValue`
			if got := gjson.GetBytes(out, path).String(); got != tt.want {
				t.Fatalf("attribute = %q, want %q", got, tt.want)
			}
		})
	}
}
