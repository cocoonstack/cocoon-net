package volcengine

import (
	"context"
	"io"
	"net/http"
	"time"
)

const metadataTimeout = 2 * time.Second

// Detect probes the Volcengine metadata endpoint.
func Detect(ctx context.Context) bool {
	client := &http.Client{Timeout: metadataTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataBase+"/instance-id", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
