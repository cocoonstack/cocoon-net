package platform

import (
	"context"
	"io"
	"net/http"
	"time"
)

// ProbeMetadata GETs url with the given headers and reports whether the
// response was 200 OK, draining and closing the body either way. Used by
// each cloud platform's Detect() to probe its instance metadata endpoint.
func ProbeMetadata(ctx context.Context, url string, headers map[string]string, timeout time.Duration) bool {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
