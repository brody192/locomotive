package config

import "strings"

func containsAnyHost(hostname string, expectedHosts []string) bool {
	for _, expectedHost := range expectedHosts {
		if strings.Contains(hostname, expectedHost) {
			return true
		}
	}

	return false
}

// headersContainFold reports whether headers has a key matching target, compared
// case-insensitively.
func headersContainFold(headers AdditionalHeaders, target string) bool {
	for key := range headers {
		if strings.EqualFold(key, target) {
			return true
		}
	}

	return false
}
