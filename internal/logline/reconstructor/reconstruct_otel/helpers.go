package reconstruct_otel

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/brody192/locomotive/internal/railway/subscribe"
	"github.com/tidwall/gjson"
)

// MetadataProvider is implemented by any log-with-metadata type.
type MetadataProvider interface {
	GetMetadata() map[string]string
}

// buildLogsData groups logs by metadata, builds resource attributes, and
// assembles the OTLP logsData structure. The caller supplies a buildRecord
// callback that converts each log entry into a logRecord.
func buildLogsData[T MetadataProvider](logs []T, buildRecord func(log T, nowNano string) logRecord) logsData {
	type group struct {
		metadata map[string]string
		logs     []T
	}

	groups := []group{}
	groupIndex := map[string]int{}

	for i := range logs {
		key := metadataKey(logs[i].GetMetadata())

		if idx, ok := groupIndex[key]; ok {
			groups[idx].logs = append(groups[idx].logs, logs[i])
		} else {
			groupIndex[key] = len(groups)
			groups = append(groups, group{
				metadata: logs[i].GetMetadata(),
				logs:     []T{logs[i]},
			})
		}
	}

	data := logsData{
		ResourceLogs: make([]resourceLog, len(groups)),
	}

	now := time.Now()
	nowNano := strconv.FormatInt(now.UnixNano(), 10)

	for gi, g := range groups {
		attrs := buildResourceAttributes(g.metadata)

		records := make([]logRecord, len(g.logs))
		for li, log := range g.logs {
			records[li] = buildRecord(log, nowNano)
		}

		data.ResourceLogs[gi] = resourceLog{
			Resource: resource{Attributes: attrs},
			ScopeLogs: []scopeLog{{
				Scope:      scope{Name: "locomotive"},
				LogRecords: records,
			}},
		}
	}

	return data
}

// buildResourceAttributes converts metadata into OTLP resource attributes,
// mapping service_name and environment_name to their conventional keys.
func buildResourceAttributes(metadata map[string]string) []attribute {
	var attrs []attribute

	if serviceName, ok := metadata[subscribe.MetadataKeyServiceName]; ok {
		attrs = append(attrs, stringAttribute("service.name", serviceName))
	}

	if environmentName, ok := metadata[subscribe.MetadataKeyEnvironmentName]; ok {
		attrs = append(attrs, stringAttribute("deployment.environment.name", environmentName))
	}

	if projectName, ok := metadata[subscribe.MetadataKeyProjectName]; ok {
		attrs = append(attrs, stringAttribute("service.namespace", projectName))
	}

	for key, value := range metadata {
		if key == subscribe.MetadataKeyServiceName || key == subscribe.MetadataKeyEnvironmentName || key == subscribe.MetadataKeyProjectName {
			continue
		}
		attrs = append(attrs, stringAttribute(key, value))
	}

	return attrs
}

func getSeverityNumber(severity string) int {
	normalized := strings.ToLower(strings.TrimSpace(severity))
	switch normalized {
	case "warning":
		normalized = "warn"
	case "err":
		normalized = "error"
	case "critical":
		normalized = "fatal"
	}

	if n, ok := severityTextToNumber[normalized]; ok {
		return n
	}

	if n, ok := severityTextToNumber["info"]; ok {
		return n
	}

	return 0
}

func stringAttribute(key string, value string) attribute {
	return attribute{Key: key, Value: attributeValue{StringValue: &value}}
}

func intAttribute(key string, value int64) attribute {
	s := strconv.FormatInt(value, 10)
	return attribute{Key: key, Value: attributeValue{IntValue: &s}}
}

// jsonBytesToAttributes flattens a JSON object into OTLP string attributes.
func jsonBytesToAttributes(json []byte) []attribute {
	if !gjson.ValidBytes(json) {
		return nil
	}

	parsed := gjson.ParseBytes(json)
	var attrs []attribute

	flattenToAttributes("", parsed, &attrs)

	return attrs
}

func flattenToAttributes(prefix string, value gjson.Result, attrs *[]attribute) {
	switch value.Type {
	case gjson.JSON:
		switch {
		case value.IsObject():
			value.ForEach(func(key, val gjson.Result) bool {
				newKey := key.String()
				if prefix != "" {
					newKey = prefix + "." + key.String()
				}
				flattenToAttributes(newKey, val, attrs)
				return true
			})
		case value.IsArray():
			value.ForEach(func(key, val gjson.Result) bool {
				newKey := key.String()
				if prefix != "" {
					newKey = prefix + "." + key.String()
				}
				flattenToAttributes(newKey, val, attrs)
				return true
			})
		}
	default:
		if prefix == "" {
			return
		}
		*attrs = append(*attrs, stringAttribute(prefix, value.String()))
	}
}

func metadataKey(metadata map[string]string) string {
	keys := make([]string, 0, len(metadata))
	for k := range metadata {
		keys = append(keys, k)
	}

	// sort keys to ensure consistent order
	sort.Strings(keys)

	keyBuffer := strings.Builder{}

	for _, k := range keys {
		keyBuffer.WriteString(fmt.Sprintf("%d:%s|", len(k), k))

		v := metadata[k]
		keyBuffer.WriteString(fmt.Sprintf("%d:%s|", len(v), v))
	}

	return keyBuffer.String()
}
