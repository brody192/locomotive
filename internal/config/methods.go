package config

import (
	"fmt"
	"strings"
)

func (h *AdditionalHeaders) UnmarshalText(envByte []byte) error {
	if h == nil {
		return fmt.Errorf("AdditionalHeaders is nil")
	}

	envString := string(envByte)

	envStringTrimmed := strings.TrimSpace(envString)

	if envStringTrimmed == "" {
		return fmt.Errorf("AdditionalHeaders is empty")
	}

	headers := make(map[string]string)

	headerPairs := strings.Split(envStringTrimmed, ";")

	for _, header := range headerPairs {
		keyValue := strings.SplitN(header, "=", 2)

		if len(keyValue) != 2 {
			return fmt.Errorf("header key value pair must be in format k=v; found %s", header)
		}

		headers[strings.TrimSpace(keyValue[0])] = strings.TrimSpace(keyValue[1])
	}

	*h = headers

	return nil
}

func (h *AdditionalHeaders) Keys() []string {
	keys := make([]string, 0, len(*h))

	for key := range *h {
		keys = append(keys, key)
	}

	return keys
}
