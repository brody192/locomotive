package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/brody192/locomotive/internal/logger"
	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

var Global = config{}

func init() {
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			logger.Stderr.Error("error loading .env file", logger.ErrAttr(err))
			os.Exit(1)
		}
	}

	errors := []error{}

	if err := env.Parse(&Global); err != nil {
		if er, ok := err.(env.AggregateError); ok {
			errors = append(errors, er.Errors...)
		} else {
			errors = append(errors, err)
		}
	}

	if (!Global.EnableDeployLogs && !Global.EnableHttpLogs) && len(errors) == 0 {
		errors = append(errors, fmt.Errorf("at least one of ENABLE_DEPLOY_LOGS or ENABLE_HTTP_LOGS must be true"))
	}

	if len(errors) > 0 {
		logger.Stderr.Error("error parsing environment variables", logger.ErrAttr(errors[0]))
		os.Exit(1)
	}

	if _, ok := WebhookModeToConfig[Global.WebhookMode]; !ok {
		logger.Stderr.Warn(fmt.Sprintf("invalid or unsupported webhook mode: %s, using default mode: %s", Global.WebhookMode, DefaultWebhookMode))
		Global.WebhookMode = DefaultWebhookMode
	}

	hostAttrs := []any{
		slog.Any("configured_mode", Global.WebhookMode),
		slog.String("webhook_host", Global.WebhookUrl.Hostname()),
	}

	for mode, config := range WebhookModeToConfig {
		if mode == Global.WebhookMode {
			if !strings.Contains(Global.WebhookUrl.Hostname(), config.ExpectedHostContains) {
				hostAttrs = append(hostAttrs, slog.String("expected_host_contains", config.ExpectedHostContains))
			}
		} else {
			if config.ExpectedHostContains != "" && strings.Contains(Global.WebhookUrl.Hostname(), config.ExpectedHostContains) {
				hostAttrs = append(hostAttrs, slog.Any("suggested_mode", mode))
				break
			}
		}
	}

	// Warn if we added any validation attributes beyond the basic ones
	if len(hostAttrs) > 2 {
		logger.Stderr.Warn("possible webhook misconfiguration", hostAttrs...)
	}

	// Header validation with separate attributes and logging
	headerAttrs := []any{
		slog.Any("configured_mode", Global.WebhookMode),
		slog.Any("configured_headers", Global.AdditionalHeaders.Keys()),
	}

	if len(WebhookModeToConfig[Global.WebhookMode].ExpectedHeaders) > 0 {
		missingHeaders := []string{}

		for _, expectedHeader := range WebhookModeToConfig[Global.WebhookMode].ExpectedHeaders {
			if !func(expectedHeader string) bool {
				for configuredHeader := range Global.AdditionalHeaders {
					if strings.EqualFold(configuredHeader, expectedHeader) {
						return true
					}
				}

				return false
			}(expectedHeader) {
				missingHeaders = append(missingHeaders, expectedHeader)
			}
		}

		if len(missingHeaders) > 0 {
			headerAttrs = append(headerAttrs, slog.Any("missing_headers", missingHeaders))
		}
	}

	// Warn if we added any header validation attributes beyond the basic ones
	if len(headerAttrs) > 2 && len(hostAttrs) <= 2 {
		logger.Stderr.Warn("possible webhook header misconfiguration", headerAttrs...)
	}
}
