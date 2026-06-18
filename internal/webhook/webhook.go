package webhook

import (
	"context"

	"github.com/brody192/locomotive/internal/config"
	"github.com/brody192/locomotive/internal/webhook/generic"
)

func SendPayload(ctx context.Context, payload []byte) error {
	return generic.SendRawWebhook(ctx, payload, config.Global.WebhookUrl, config.WebhookModeToConfig[config.Global.WebhookMode].DefaultHeaders, config.Global.AdditionalHeaders, client)
}
