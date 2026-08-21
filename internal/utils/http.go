package utils

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// FetchCACert downloads a CA certificate from the given URL.
// The response body is properly closed before returning, avoiding resource leaks.
func FetchCACert(ctx context.Context, client *http.Client, certURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, certURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create CA cert request for %s: %w", certURL, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch CA cert from %s: %w", certURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch CA cert from %s: status %d", certURL, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read CA cert from %s: %w", certURL, err)
	}
	return data, nil
}
