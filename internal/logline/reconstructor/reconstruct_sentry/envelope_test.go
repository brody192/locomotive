package reconstruct_sentry

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/brody192/locomotive/internal/railway/gql/subscriptions"
	"github.com/brody192/locomotive/internal/railway/subscribe"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
	"github.com/tidwall/gjson"
)

// baseAttributeCount is the number of attributes every item carries from the Item template.
const baseAttributeCount = 5

// envelopeItems asserts that every line of a Sentry envelope is valid JSON,
// since Relay rejects the whole item container otherwise, and returns the log items.
func envelopeItems(t *testing.T, envelope []byte) []gjson.Result {
	t.Helper()

	lines := bytes.Split(envelope, []byte("\n"))
	for i, line := range lines {
		if !json.Valid(line) {
			t.Fatalf("envelope line %d is not valid JSON: %s", i, line)
		}
	}

	items := gjson.GetBytes(lines[len(lines)-1], "items").Array()
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	return items
}

func environmentItem(t *testing.T, attributes ...subscriptions.EnvironmentLogAttributes) gjson.Result {
	t.Helper()

	log := subscriptions.EnvironmentLog{
		Timestamp:  time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC),
		Message:    "hello",
		Severity:   "info",
		Attributes: attributes,
	}

	envelope, err := EnvironmentLogsEnvelope([]environment_logs.EnvironmentLogWithMetadata{{Log: log}})
	if err != nil {
		t.Fatalf("EnvironmentLogsEnvelope returned error: %v", err)
	}

	return envelopeItems(t, envelope)[0]
}

func httpItem(t *testing.T, logJSON string) gjson.Result {
	t.Helper()

	logs := []http_logs.DeploymentHttpLogWithMetadata{{
		Timestamp:  time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC),
		Log:        json.RawMessage(logJSON),
		Path:       "/",
		StatusCode: 200,
	}}

	envelope, err := HttpLogsEnvelope(logs)
	if err != nil {
		t.Fatalf("HttpLogsEnvelope returned error: %v", err)
	}

	return envelopeItems(t, envelope)[0]
}

// attribute looks the key up literally rather than through a path, so keys
// containing path syntax are checked as they arrive in Sentry.
func attribute(item gjson.Result, key string) string {
	return item.Get("attributes").Map()[key].Raw
}

func stringAttributeJSON(t *testing.T, value string) string {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%q) returned error: %v", value, err)
	}

	return `{"value":` + string(encoded) + `,"type":"string"}`
}

type valueCase struct {
	name     string
	rawValue string
	want     string
}

func runEnvironmentValueCases(t *testing.T, tests []valueCase) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := environmentItem(t, subscriptions.EnvironmentLogAttributes{Key: "field", Value: tt.rawValue})

			if got := attribute(item, "field"); got != tt.want {
				t.Fatalf("attribute = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestEnvironmentLogsEnvelope_shouldSendQuotedValuesAsStrings(t *testing.T) {
	// Railway sends attribute values JSON-encoded: a quoted value was logged as a string.
	tests := []struct {
		name     string
		rawValue string
		want     string
	}{
		{"overflowing exponent", `"7e145729"`, "7e145729"},
		{"negative overflowing exponent", `"-7e145729"`, "-7e145729"},
		{"finite exponent hash", `"1234e56"`, "1234e56"},
		{"NaN", `"NaN"`, "NaN"},
		{"Infinity", `"-Infinity"`, "-Infinity"},
		{"digits", `"42"`, "42"},
		{"decimal", `"3.14"`, "3.14"},
		{"boolean word", `"true"`, "true"},
		{"null word", `"null"`, "null"},
		{"stringified JSON object", `"{\"a\":1}"`, `{"a":1}`},
		{"plain", `"hello"`, "hello"},
		{"escaped slash", `"http:\/\/x"`, "http://x"},
		{"surrogate pair", `"\ud83d\ude00"`, "\U0001F600"},
		{"escaped quotes", `"\"quoted\""`, `"quoted"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := environmentItem(t, subscriptions.EnvironmentLogAttributes{Key: "field", Value: tt.rawValue})

			if got, want := attribute(item, "field"), stringAttributeJSON(t, tt.want); got != want {
				t.Fatalf("attribute = %s, want %s", got, want)
			}
		})
	}
}

func TestEnvironmentLogsEnvelope_shouldInferTypesOfUnquotedValues(t *testing.T) {
	runEnvironmentValueCases(t, []valueCase{
		{"integer", "42", `{"value":42,"type":"integer"}`},
		{"negative integer", "-7", `{"value":-7,"type":"integer"}`},
		{"float", "3.14", `{"value":3.14,"type":"double"}`},
		{"exponent float", "1e5", `{"value":100000,"type":"double"}`},
		{"true", "true", `{"value":true,"type":"boolean"}`},
		{"false", "false", `{"value":false,"type":"boolean"}`},
		{"null", "null", `{"value":"null","type":"string"}`},
		{"overflowing number", "7e145729", `{"value":"7e145729","type":"string"}`},
		{"negative overflowing number", "-7e145729", `{"value":"-7e145729","type":"string"}`},
		{"NaN", "NaN", `{"value":"NaN","type":"string"}`},
		{"Infinity", "Infinity", `{"value":"Infinity","type":"string"}`},
		{"not JSON", "plain text", `{"value":"plain text","type":"string"}`},
	})
}

func TestEnvironmentLogsEnvelope_shouldSendEmptyContainersAsStrings(t *testing.T) {
	runEnvironmentValueCases(t, []valueCase{
		{"empty object", "{}", `{"value":"{}","type":"string"}`},
		{"empty array", "[]", `{"value":"[]","type":"string"}`},
		{"empty object with whitespace", "{ }", `{"value":"{}","type":"string"}`},
		{"empty array with whitespace", "[\n]", `{"value":"[]","type":"string"}`},
	})
}

func TestEnvironmentLogsEnvelope_shouldFlattenJSONContainers(t *testing.T) {
	tests := []struct {
		name     string
		rawValue string
		key      string
		want     string
	}{
		{"object number field", `{"count":2,"name":"box"}`, "ctx__count", `{"value":2,"type":"double"}`},
		{"object string field", `{"count":2,"name":"box"}`, "ctx__name", `{"value":"box","type":"string"}`},
		{"array element", `["a","b"]`, "ctx[1]", `{"value":"b","type":"string"}`},
		{"overflowing number in object", `{"big":7e145729}`, "ctx__big", `{"value":"7e145729","type":"string"}`},
		{"empty array in object", `{"tags":[]}`, "ctx__tags", `{"value":"[]","type":"string"}`},
		{"empty object in array", `[{}]`, "ctx[0]", `{"value":"{}","type":"string"}`},
		{"dotted key in object", `{"user.id":"u1"}`, "ctx__user.id", `{"value":"u1","type":"string"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := environmentItem(t, subscriptions.EnvironmentLogAttributes{Key: "ctx", Value: tt.rawValue})

			if got := attribute(item, tt.key); got != tt.want {
				t.Fatalf("%s = %s, want %s", tt.key, got, tt.want)
			}
		})
	}
}

func TestEnvironmentLogsEnvelope_shouldNotLetLogAttributesOverrideTemplateAttributes(t *testing.T) {
	for _, key := range []string{"sentry.sdk.name", "sentry.sdk.version", "sentry.server.address", "sentry.environment", "sentry.origin"} {
		t.Run(key, func(t *testing.T) {
			template := attribute(environmentItem(t), key)

			item := environmentItem(t, subscriptions.EnvironmentLogAttributes{Key: key, Value: "5"})

			if got := attribute(item, key); got != template {
				t.Fatalf("attribute %q = %s, want the template's %s", key, got, template)
			}

			if got, want := attribute(item, "_"+key), `{"value":5,"type":"integer"}`; got != want {
				t.Fatalf("attribute %q = %s, want %s", "_"+key, got, want)
			}
		})
	}
}

func TestHttpLogsEnvelope_shouldNotLetLogFieldsOverrideTemplateAttributes(t *testing.T) {
	item := httpItem(t, `{"sentry.origin":"manual"}`)

	if got, want := attribute(item, "sentry.origin"), `{"value":"auto.log.locomotive","type":"string"}`; got != want {
		t.Fatalf("sentry.origin = %s, want %s", got, want)
	}

	if got, want := attribute(item, "_sentry.origin"), `{"value":"manual","type":"string"}`; got != want {
		t.Fatalf("_sentry.origin = %s, want %s", got, want)
	}
}

func TestEnvironmentLogsEnvelope_shouldKeepAttributeKeysLiteral(t *testing.T) {
	for _, key := range []string{"http.method", "a*b", "a?b", "a|b", "a#b", "a@b", ":colon", `a\b`, "a[0]"} {
		t.Run(key, func(t *testing.T) {
			item := environmentItem(t, subscriptions.EnvironmentLogAttributes{Key: key, Value: `"v"`})

			if got, want := attribute(item, key), `{"value":"v","type":"string"}`; got != want {
				t.Fatalf("attribute %q = %s, want %s", key, got, want)
			}

			if got, want := len(item.Get("attributes").Map()), baseAttributeCount+1; got != want {
				t.Fatalf("attribute count = %d, want %d: %s", got, want, item.Get("attributes").Raw)
			}
		})
	}
}

func TestEnvironmentLogsEnvelope_shouldSendTheRailwayEnvironmentAsAString(t *testing.T) {
	logs := []environment_logs.EnvironmentLogWithMetadata{{
		Log: subscriptions.EnvironmentLog{
			Timestamp:  time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC),
			Message:    "hello",
			Severity:   "info",
			Attributes: []subscriptions.EnvironmentLogAttributes{{Key: "sentry.environment", Value: "5"}},
		},
		Metadata: environment_logs.EnvironmentLogMetadata{subscribe.MetadataKeyEnvironmentName: "production"},
	}}

	envelope, err := EnvironmentLogsEnvelope(logs)
	if err != nil {
		t.Fatalf("EnvironmentLogsEnvelope returned error: %v", err)
	}

	item := envelopeItems(t, envelope)[0]
	if got, want := attribute(item, "sentry.environment"), `{"value":"production","type":"string"}`; got != want {
		t.Fatalf("sentry.environment = %s, want %s", got, want)
	}
}

func TestHttpLogsEnvelope_shouldFlattenLogFields(t *testing.T) {
	item := httpItem(t, `{"big":1e400,"negative":-1e400,"duration":12.5,"user.id":"u1","headers":{},"tags":[]}`)

	tests := []struct {
		key  string
		want string
	}{
		{"big", `{"value":"1e400","type":"string"}`},
		{"negative", `{"value":"-1e400","type":"string"}`},
		{"duration", `{"value":12.5,"type":"double"}`},
		{"user.id", `{"value":"u1","type":"string"}`},
		{"headers", `{"value":"{}","type":"string"}`},
		{"tags", `{"value":"[]","type":"string"}`},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := attribute(item, tt.key); got != tt.want {
				t.Fatalf("attribute = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestHttpLogsEnvelope_shouldAddNoAttributesForAnEmptyLog(t *testing.T) {
	item := httpItem(t, `{}`)

	// Only the template attributes plus the level derived from the status code.
	if got, want := len(item.Get("attributes").Map()), baseAttributeCount+1; got != want {
		t.Fatalf("attribute count = %d, want %d: %s", got, want, item.Get("attributes").Raw)
	}
}
