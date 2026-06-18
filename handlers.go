package main

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/brody192/locomotive/internal/logger"
	"github.com/brody192/locomotive/internal/queue"
	"github.com/brody192/locomotive/internal/webhook"
)

// Pipeline names, used to label each pipeline's dispatcher, subscription retries, and logs.
const (
	pipelineDeployLogs = "deploy-logs"
	pipelineHTTPLogs   = "http-logs"
)

// Suffixes appended to a pipeline name to label its two sub-components.
const (
	dispatcherNameSuffix   = "-webhook"
	subscriptionNameSuffix = "-subscription"
)

type webhookPayload struct {
	data     []byte
	logCount int
}

func runLogPipeline[T any](
	ctx context.Context,
	name string,
	serialize func([]T) ([]byte, error),
	subscribe func(ctx context.Context, track chan []T) error,
	processed *atomic.Int64,
) error {
	dispatcher, err := queue.NewDispatcher(
		queue.Name((name + dispatcherNameSuffix)),
		queue.MaxQueueSize(1000),
		queue.MaxRetries(5),
		queue.InitialBackoff((500 * time.Millisecond)),
		queue.MaxBackoff((30 * time.Second)),
		queue.BackoffMultiplier(2.0),
		queue.TTL((5 * time.Minute)),
		queue.Workers(4),
		func(ctx context.Context, p webhookPayload) error {
			return webhook.SendPayload(ctx, p.data)
		},
	)
	if err != nil {
		return fmt.Errorf("error creating %s dispatcher: %w", name, err)
	}

	dispatcher.OnSuccess = func(p webhookPayload, _ int) {
		processed.Add(int64(p.logCount))
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
				payload, err := serialize(logs)
				if err != nil {
					logger.Stderr.Error("failed to serialize logs", logger.ErrAttr(err))
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
			if err := subscribe(ctx, track); err != nil {
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
