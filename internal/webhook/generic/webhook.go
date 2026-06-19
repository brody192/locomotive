package generic

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/brody192/locomotive/internal/config"
)

var acceptedStatusCodes = []int{
	http.StatusOK,
	http.StatusNoContent,
	http.StatusAccepted,
	http.StatusCreated,
}

const maxErrorBodySize = 8192

func SendRawWebhook(ctx context.Context, logs []byte, url url.URL, defaultHeaders map[string]string, additionalHeaders config.AdditionalHeaders, client *http.Client) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url.String(), bytes.NewReader(logs))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Default headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Keep-Alive", "timeout=5, max=1000")

	for key, value := range defaultHeaders {
		req.Header.Set(key, value)
	}

	for key, value := range additionalHeaders {
		req.Header.Set(key, value)
	}

	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook request: %w", err)
	}

	defer res.Body.Close()

	if !slices.Contains(acceptedStatusCodes, res.StatusCode) {
		body, err := io.ReadAll(io.LimitReader(res.Body, maxErrorBodySize))
		io.Copy(io.Discard, res.Body)
		bodyStr := strings.TrimSpace(string(body))
		if err != nil || len(bodyStr) == 0 {
			return fmt.Errorf("non success status code: %d", res.StatusCode)
		}

		return fmt.Errorf("non success status code: %d; with body: %s", res.StatusCode, bodyStr)
	}

	io.Copy(io.Discard, res.Body)

	return nil
}
