package otel

import (
	"context"
	"os"

	"github.com/brody192/locomotive/internal/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Provider holds the OTLP logger provider for emitting logs.
var Provider *log.LoggerProvider

// Config holds OTEL configuration.
type Config struct {
	Enabled         bool
	Endpoint        string
	ServiceName     string
	EnvironmentName string
}

// Setup initializes the OTLP log exporter and logger provider.
func Setup(ctx context.Context, cfg Config) error {
	if !cfg.Enabled {
		return nil
	}

	logger.Stdout.Info("Setting up OTEL log exporter",
		"endpoint", cfg.Endpoint,
		"service_name", cfg.ServiceName,
		"environment", cfg.EnvironmentName,
	)

	// Build resource with service attributes
	hostname, _ := os.Hostname()
	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.ServiceName),
		semconv.DeploymentEnvironmentName(cfg.EnvironmentName),
	}
	if hostname != "" {
		attrs = append(attrs, semconv.HostName(hostname))
	}

	res, err := resource.New(ctx, resource.WithAttributes(attrs...))
	if err != nil {
		return err
	}

	// Create gRPC connection with insecure credentials (internal network)
	exporter, err := otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint(cfg.Endpoint),
		otlploggrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	if err != nil {
		return err
	}

	Provider = log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(exporter)),
		log.WithResource(res),
	)

	return nil
}

// Shutdown gracefully shuts down the logger provider.
func Shutdown(ctx context.Context) error {
	if Provider == nil {
		return nil
	}
	return Provider.Shutdown(ctx)
}
