package webhook

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

var client *http.Client

func init() {
	client = &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 5 * time.Minute,
			}).DialContext,
			MaxConnsPerHost:       10,
			MaxIdleConns:          10,
			IdleConnTimeout:       5 * time.Minute,
			TLSHandshakeTimeout:   20 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
			ExpectContinueTimeout: 20 * time.Second,
			MaxIdleConnsPerHost:   10,
			DisableKeepAlives:     false,
			TLSClientConfig:       &tls.Config{},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
