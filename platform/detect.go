package platform

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	gceMetadataURL        = "http://metadata.google.internal/computeMetadata/v1/instance/zone"
	volcengineMetadataURL = "http://100.96.0.96/latest/meta-data/instance-id"
	metadataTimeout       = 2 * time.Second
)

// Detect auto-detects the cloud platform by probing metadata endpoints.
func Detect(ctx context.Context) (CloudPlatform, error) {
	client := &http.Client{Timeout: metadataTimeout}

	// Try GKE/GCE first.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, gceMetadataURL, nil)
	if err == nil {
		req.Header.Set("Metadata-Flavor", "Google")
		resp, doErr := client.Do(req)
		if doErr == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return &GKEPlatform{}, nil
			}
		}
	}

	// Try Volcengine.
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, volcengineMetadataURL, nil)
	if err == nil {
		resp2, err := client.Do(req2)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp2.Body)
			_ = resp2.Body.Close()
			if resp2.StatusCode == http.StatusOK {
				return &VolcenginePlatform{}, nil
			}
		}
	}

	return nil, fmt.Errorf("could not detect cloud platform — set --platform explicitly (gke|volcengine)")
}
