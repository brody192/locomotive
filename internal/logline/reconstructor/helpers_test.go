package reconstructor

import (
	"testing"
	"time"

	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
)

func TestDecodeJSONString_shouldDecodeOnlyJSONStrings(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		want   string
		wantOK bool
	}{
		{"plain string", `"hello"`, "hello", true},
		{"empty string", `""`, "", true},
		{"escaped slash", `"http:\/\/x"`, "http://x", true},
		{"surrogate pair", `"\ud83d\ude00"`, "\U0001F600", true},
		{"escaped quotes", `"\"quoted\""`, `"quoted"`, true},
		{"number", "42", "42", false},
		{"object", `{"a":1}`, `{"a":1}`, false},
		{"null", "null", "null", false},
		{"Go single-quoted rune", `'a'`, `'a'`, false},
		{"Go raw string", "`a`", "`a`", false},
		{"Go-only escape", `"\x41"`, `"\x41"`, false},
		{"trailing data", `"a" b`, `"a" b`, false},
		{"unterminated", `"abc`, `"abc`, false},
		{"empty input", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := DecodeJSONString(tt.raw)

			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("DecodeJSONString(%q) = (%q, %v), want (%q, %v)", tt.raw, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestTryExtractTimestamp_shouldDecodeJSONEscapedValues(t *testing.T) {
	log := environment_logs.EnvironmentLogWithMetadata{
		Log: subscriptions.EnvironmentLog{
			Attributes: []subscriptions.EnvironmentLogAttributes{
				// JSON encoders may escape slashes, which strconv.Unquote rejects.
				{Key: "time", Value: `"2026\/11\/11 10:30:00 +0000"`},
			},
		},
	}

	want := time.Date(2026, 11, 11, 10, 30, 0, 0, time.UTC)
	if got := TryExtractTimestamp(log); !got.Equal(want) {
		t.Fatalf("TryExtractTimestamp() = %v, want %v", got, want)
	}
}
