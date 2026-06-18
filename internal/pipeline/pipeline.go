package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/brody192/locomotive/internal/logger"
	"github.com/brody192/locomotive/internal/queue"
	"github.com/brody192/locomotive/internal/railway"
	"github.com/brody192/locomotive/internal/railway/subscribe/environment_logs"
	"github.com/brody192/locomotive/internal/railway/subscribe/http_logs"
	"github.com/brody192/locomotive/internal/webhook"
	"github.com/flexstack/uuid"
)

// Log is the set of log-entry types a pipeline can carry.
type Log interface {
	environment_logs.EnvironmentLogWithMetadata | http_logs.DeploymentHttpLogWithMetadata
}

// Pipeline names, used to label the dispatcher, subscription retries, and logs. The name
// is derived from the log type, so callers never pass it.
const (
	deployLogsName = "deploy-logs"
	httpLogsName   = "http-logs"
)

// Suffixes appended to a pipeline name to label its two sub-components.
const (
	dispatcherNameSuffix   = "-webhook"
	subscriptionNameSuffix = "-subscription"
)

// Typed parameters for New, mirroring queue.NewDispatcher: each field is its own distinct
// type, so a New(...) call is self-documenting, every field is required at compile time,
// and the arguments can't be passed in the wrong order.
type (
	clientParam      struct{ v *railway.GraphQLClient }
	environmentParam struct{ v uuid.UUID }
	servicesParam    struct{ v []uuid.UUID }
	processedParam   struct{ v *atomic.Int64 }
	failedParam      struct{ v *atomic.Int64 }

	serializeParam[T Log] struct {
		v func([]T) ([]byte, error)
	}
	subscribeParam[T Log] struct {
		v func(ctx context.Context, g *railway.GraphQLClient, track chan<- []T, environmentId uuid.UUID, serviceIds []uuid.UUID) error
	}
)

// Client is the Railway GraphQL client the pipeline subscribes with.
func Client(c *railway.GraphQLClient) clientParam { return clientParam{c} }

// EnvironmentID is the environment whose logs are streamed.
func EnvironmentID(id uuid.UUID) environmentParam { return environmentParam{id} }

// ServiceIDs are the services whose logs are streamed.
func ServiceIDs(ids []uuid.UUID) servicesParam { return servicesParam{ids} }

// Processed counts the log entries successfully shipped.
func Processed(p *atomic.Int64) processedParam { return processedParam{p} }

// Failed counts log entries received from Railway but never shipped — a serialize
// failure, or a drop after the dispatcher exhausts its retries/TTL.
func Failed(p *atomic.Int64) failedParam { return failedParam{p} }

// Serialize turns a batch of logs into the webhook payload bytes.
func Serialize[T Log](fn func([]T) ([]byte, error)) serializeParam[T] {
	return serializeParam[T]{fn}
}

// Subscribe streams logs into the provided track channel.
func Subscribe[T Log](fn func(ctx context.Context, g *railway.GraphQLClient, track chan<- []T, environmentId uuid.UUID, serviceIds []uuid.UUID) error) subscribeParam[T] {
	return subscribeParam[T]{fn}
}

// LogPipeline subscribes to a stream of logs of type T, serializes them, and ships them
// to the configured webhook. Build one with NewLogPipeline and start it with Run.
type LogPipeline[T Log] struct {
	client        *railway.GraphQLClient
	environmentId uuid.UUID
	serviceIds    []uuid.UUID
	serialize     func([]T) ([]byte, error)
	subscribe     func(ctx context.Context, g *railway.GraphQLClient, track chan<- []T, environmentId uuid.UUID, serviceIds []uuid.UUID) error
	processed     *atomic.Int64
	failed        *atomic.Int64
}

// New builds a Pipeline. Because each parameter is a distinct type (see
// queue.NewDispatcher), every field is required at compile time and the arguments can't
// be passed out of order. T is inferred from the Serialize/Subscribe arguments.
func NewLogPipeline[T Log](
	client clientParam,
	environment environmentParam,
	services servicesParam,
	serialize serializeParam[T],
	subscribe subscribeParam[T],
	processed processedParam,
	failed failedParam,
) LogPipeline[T] {
	return LogPipeline[T]{
		client:        client.v,
		environmentId: environment.v,
		serviceIds:    services.v,
		serialize:     serialize.v,
		subscribe:     subscribe.v,
		processed:     processed.v,
		failed:        failed.v,
	}
}

type webhookPayload struct {
	data     []byte
	logCount int
}

// Run starts the pipeline and blocks until ctx is cancelled or the subscription fails.
func (p LogPipeline[T]) Run(ctx context.Context) error {
	var zero T
	var name string
	switch any(zero).(type) {
	case environment_logs.EnvironmentLogWithMetadata:
		name = deployLogsName
	case http_logs.DeploymentHttpLogWithMetadata:
		name = httpLogsName
	default:
		return fmt.Errorf("unsupported pipeline log type %T", zero)
	}

	dispatcher, err := queue.NewDispatcher(
		queue.Name((name + dispatcherNameSuffix)),
		queue.MaxQueueSize(1000),
		queue.MaxRetries(5),
		queue.InitialBackoff((500 * time.Millisecond)),
		queue.MaxBackoff((30 * time.Second)),
		queue.BackoffMultiplier(2.0),
		queue.TTL((5 * time.Minute)),
		queue.Workers(4),
		func(ctx context.Context, wp webhookPayload) error {
			return webhook.SendPayload(ctx, wp.data)
		},
	)
	if err != nil {
		return fmt.Errorf("error creating %s dispatcher: %w", name, err)
	}

	dispatcher.OnSuccess = func(wp webhookPayload, _ int) {
		p.processed.Add(int64(wp.logCount))
	}

	dispatcher.OnDrop = func(wp webhookPayload, _ string) {
		p.failed.Add(int64(wp.logCount))
	}

	dispatcher.Start(ctx)
	defer dispatcher.Stop()

	pipeCtx, pipeCancel := context.WithCancel(ctx)
	defer pipeCancel()

	track := make(chan []T, 100)

	go func() {
		for {
			select {
			case <-pipeCtx.Done():
				return
			case logs := <-track:
				payload, err := p.serialize(logs)
				if err != nil {
					logger.Stderr.Error("failed to serialize logs", logger.ErrAttr(err))
					p.failed.Add(int64(len(logs)))
					continue
				}

				dispatcher.Enqueue(webhookPayload{
					data:     payload,
					logCount: len(logs),
				})
			}
		}
	}()

	if err := queue.RetryConstant(
		pipeCtx,
		queue.Name((name + subscriptionNameSuffix)),
		queue.MaxRetries(10),
		queue.RetryInterval((1 * time.Second)),
		func(ctx context.Context) error {
			if err := p.subscribe(ctx, p.client, track, p.environmentId, p.serviceIds); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}

				return queue.Retryable(err)
			}

			return nil
		},
	); err != nil {
		return err
	}

	logger.Stdout.Debug(fmt.Sprintf("%s subscription ended", name))

	return nil
}
