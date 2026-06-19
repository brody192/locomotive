package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/brody192/locomotive/internal/logger"
	"github.com/caarlos0/env/v11"
	"github.com/flexstack/uuid"
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

	if err := env.ParseWithOptions(&Global, env.Options{
		Prefix: "LOCOMOTIVE_",
		FuncMap: map[reflect.Type]env.ParserFunc{
			reflect.TypeFor[uuid.UUID](): func(envVar string) (any, error) {
				return uuid.FromString(strings.TrimSpace(envVar))
			},
			reflect.TypeFor[[]uuid.UUID](): func(envVar string) (any, error) {
				envVarSplit := strings.Split(envVar, ",")

				uuids := []uuid.UUID{}

				for _, envVarSplitItem := range envVarSplit {
					envVarSplitItemTrimmed := strings.TrimSpace(envVarSplitItem)

					if envVarSplitItemTrimmed == "" {
						continue
					}

					uuid, err := uuid.FromString(envVarSplitItemTrimmed)
					if err != nil {
						return nil, err
					}

					uuids = append(uuids, uuid)
				}

				return uuids, nil
			},
			reflect.TypeFor[bool](): func(envVar string) (any, error) {
				return strconv.ParseBool(strings.TrimSpace(envVar))
			},
			reflect.TypeFor[url.URL](): func(envVar string) (any, error) {
				envVarTrimmed := strings.TrimSpace(envVar)

				if !schemeRegex.MatchString(envVarTrimmed) {
					logger.Stderr.Warn("found webhook url without scheme, adding default scheme: https")
					envVarTrimmed = "https://" + envVarTrimmed
				}

				if u, err := url.ParseRequestURI(envVarTrimmed); err != nil {
					return nil, err
				} else {
					return *u, nil
				}
			},
		},
	}); err != nil {
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
		logger.Stderr.Error("error parsing environment variables", logger.ErrorsAttr(errors...))
		os.Exit(1)
	}

	Global.WebhookMode = WebhookMode(strings.ToLower(strings.TrimSpace(string(Global.WebhookMode))))

	if _, ok := WebhookModeToConfig[Global.WebhookMode]; !ok {
		logger.Stderr.Warn(fmt.Sprintf("invalid or unsupported webhook mode: %s, using default mode: %s", Global.WebhookMode, DefaultWebhookMode))
		Global.WebhookMode = DefaultWebhookMode
	}

	hostAttrs := []any{
		slog.Any("configured_mode", Global.WebhookMode),
		slog.String("webhook_host", Global.WebhookUrl.Hostname()),
	}
	hostMisconfigured := false

	for mode, config := range WebhookModeToConfig {
		if mode == Global.WebhookMode {
			if len(config.ExpectedHostContains) > 0 && !containsAnyHost(Global.WebhookUrl.Hostname(), config.ExpectedHostContains) {
				hostAttrs = append(hostAttrs, slog.String("expected_host_contains", strings.Join(config.ExpectedHostContains, " OR ")))
				hostMisconfigured = true
			}
		} else {
			if len(config.ExpectedHostContains) > 0 && containsAnyHost(Global.WebhookUrl.Hostname(), config.ExpectedHostContains) {
				hostAttrs = append(hostAttrs, slog.Any("suggested_mode", mode))
				hostMisconfigured = true
				break
			}
		}
	}

	if hostMisconfigured {
		logger.Stderr.Warn("possible webhook misconfiguration", hostAttrs...)
	}

	headerAttrs := []any{
		slog.Any("configured_mode", Global.WebhookMode),
		slog.Any("configured_headers", Global.AdditionalHeaders.Keys()),
	}
	headerMisconfigured := false

	if len(WebhookModeToConfig[Global.WebhookMode].ExpectedHeaders) > 0 {
		missingHeaders := []string{}

		for _, expectedHeader := range WebhookModeToConfig[Global.WebhookMode].ExpectedHeaders {
			if !headersContainFold(Global.AdditionalHeaders, expectedHeader) {
				missingHeaders = append(missingHeaders, expectedHeader)
			}
		}

		if len(missingHeaders) > 0 {
			headerAttrs = append(headerAttrs, slog.Any("missing_headers", missingHeaders))
			headerMisconfigured = true
		}
	}

	// Only warn about headers when the host looked fine, to avoid a double warning.
	if headerMisconfigured && !hostMisconfigured {
		logger.Stderr.Warn("possible webhook header misconfiguration", headerAttrs...)
	}
}
