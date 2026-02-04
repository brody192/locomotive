package otel

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
	"github.com/brody192/locomotive/internal/util"
	otellog "go.opentelemetry.io/otel/log"
	"github.com/tidwall/gjson"
)

// EmitEnvironmentLogs sends environment/deploy logs via OTLP.
func EmitEnvironmentLogs(ctx context.Context, logs []environment_logs.EnvironmentLogWithMetadata) error {
	if Provider == nil {
		return nil
	}

	logger := Provider.Logger("locomotive")

	for _, l := range logs {
		record := transformEnvironmentLog(l)
		logger.Emit(ctx, record)
	}

	return nil
}

// EmitHttpLogs sends HTTP logs via OTLP.
func EmitHttpLogs(ctx context.Context, logs []http_logs.DeploymentHttpLogWithMetadata) error {
	if Provider == nil {
		return nil
	}

	logger := Provider.Logger("locomotive")

	for _, l := range logs {
		record := transformHttpLog(l)
		logger.Emit(ctx, record)
	}

	return nil
}

func transformEnvironmentLog(l environment_logs.EnvironmentLogWithMetadata) otellog.Record {
	var record otellog.Record

	record.SetTimestamp(l.Log.Timestamp)
	record.SetObservedTimestamp(time.Now())
	record.SetBody(otellog.StringValue(util.StripAnsi(l.Log.Message)))

	// Set severity
	severityText, severityNumber := parseSeverity(l.Log.Severity)
	record.SetSeverity(otellog.Severity(severityNumber))
	record.SetSeverityText(severityText)

	var attrs []otellog.KeyValue

	// Add Railway metadata
	for key, value := range l.Metadata {
		attrs = append(attrs, otellog.String("railway."+key, value))
	}

	// Add tags as attributes
	if l.Log.Tags.ProjectID.String() != "" {
		attrs = append(attrs, otellog.String("railway.project.id", l.Log.Tags.ProjectID.String()))
	}
	if l.Log.Tags.EnvironmentID.String() != "" {
		attrs = append(attrs, otellog.String("railway.environment.id", l.Log.Tags.EnvironmentID.String()))
	}
	if l.Log.Tags.ServiceID.String() != "" {
		attrs = append(attrs, otellog.String("railway.service.id", l.Log.Tags.ServiceID.String()))
	}
	if l.Log.Tags.DeploymentID.String() != "" {
		attrs = append(attrs, otellog.String("railway.deployment.id", l.Log.Tags.DeploymentID.String()))
	}
	if l.Log.Tags.DeploymentInstanceID.String() != "" {
		attrs = append(attrs, otellog.String("railway.deployment.instance.id", l.Log.Tags.DeploymentInstanceID.String()))
	}

	// Add log attributes
	for _, attr := range l.Log.Attributes {
		attrs = append(attrs, otellog.String(attr.Key, attr.Value))
	}

	record.AddAttributes(attrs...)
	return record
}

func transformHttpLog(l http_logs.DeploymentHttpLogWithMetadata) otellog.Record {
	var record otellog.Record

	record.SetTimestamp(l.Timestamp)
	record.SetObservedTimestamp(time.Now())
	record.SetBody(otellog.StringValue(l.Path))

	// Set severity based on HTTP status code
	severityText, severityNumber := httpStatusToSeverity(l.StatusCode)
	record.SetSeverity(otellog.Severity(severityNumber))
	record.SetSeverityText(severityText)

	var attrs []otellog.KeyValue

	// Add Railway metadata
	for key, value := range l.Metadata {
		attrs = append(attrs, otellog.String("railway."+key, value))
	}

	// Add HTTP status
	attrs = append(attrs, otellog.Int64("http.status_code", l.StatusCode))
	attrs = append(attrs, otellog.String("http.path", l.Path))

	// Parse the raw log JSON and extract fields
	logStr := string(l.Log)
	result := gjson.Parse(logStr)
	
	result.ForEach(func(key, value gjson.Result) bool {
		k := key.String()
		// Skip fields we've already handled
		if k == "path" || k == "httpStatus" || k == "_metadata" {
			return true
		}
		
		switch value.Type {
		case gjson.String:
			attrs = append(attrs, otellog.String(k, value.String()))
		case gjson.Number:
			if strings.Contains(value.Raw, ".") {
				attrs = append(attrs, otellog.Float64(k, value.Float()))
			} else {
				attrs = append(attrs, otellog.Int64(k, value.Int()))
			}
		case gjson.True, gjson.False:
			attrs = append(attrs, otellog.Bool(k, value.Bool()))
		default:
			// For complex types, serialize to JSON string
			if jsonBytes, err := json.Marshal(value.Value()); err == nil {
				attrs = append(attrs, otellog.String(k, string(jsonBytes)))
			}
		}
		return true
	})

	record.AddAttributes(attrs...)
	return record
}

func parseSeverity(level string) (string, int) {
	switch strings.ToLower(level) {
	case "trace":
		return "TRACE", 1
	case "debug":
		return "DEBUG", 5
	case "info":
		return "INFO", 9
	case "warn", "warning":
		return "WARN", 13
	case "error", "err":
		return "ERROR", 17
	case "fatal", "critical":
		return "FATAL", 21
	default:
		if n, err := strconv.Atoi(level); err == nil {
			return level, n
		}
		return "INFO", 9
	}
}

func httpStatusToSeverity(statusCode int64) (string, int) {
	switch {
	case statusCode >= 500:
		return "ERROR", 17
	case statusCode >= 400:
		return "WARN", 13
	default:
		return "INFO", 9
	}
}
