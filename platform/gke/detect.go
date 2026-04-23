package gke

import (
	"context"
	"io"
	"net/http"
)

// Detect probes the GCE metadata endpoint to determine if running on GKE.
func Detect(ctx context.Context) bool {
	client := &http.Client{Timeout: detectionTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, detectionURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
