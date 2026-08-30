package platform

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FetchMetadata GETs url with headers and returns the trimmed body; any status but 200 is an error.
func FetchMetadata(ctx context.Context, url string, headers map[string]string, timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request %s: %w", url, err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s: status %d", url, resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", url, err)
	}
	return strings.TrimSpace(string(b)), nil
}

// ProbeMetadata reports whether url answers 200 OK.
func ProbeMetadata(ctx context.Context, url string, headers map[string]string, timeout time.Duration) bool {
	_, err := FetchMetadata(ctx, url, headers, timeout)
	return err == nil
}
