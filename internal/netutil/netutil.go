// Package netutil provides shared networking utilities for LayerKit.
package netutil

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"time"
)

// NewHTTPClient returns a standardized HTTP client configured with timeouts
// and optimal connection pooling parameters suitable for repository indexing.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}
}

// NewHTTPClientWithCAs returns a standardized HTTP client loaded with a custom CA cert pool.
func NewHTTPClientWithCAs(caPEMs [][]byte, timeout time.Duration) *http.Client {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	for _, pem := range caPEMs {
		pool.AppendCertsFromPEM(pem)
	}

	transport := &http.Transport{
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			RootCAs: pool,
		},
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
