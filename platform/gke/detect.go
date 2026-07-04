package gke

import (
	"context"

	"github.com/cocoonstack/cocoon-net/platform"
)

// Detect probes the GCE metadata endpoint to determine if running on GKE.
func Detect(ctx context.Context) bool {
	return platform.ProbeMetadata(ctx, detectionURL, map[string]string{"Metadata-Flavor": "Google"}, detectionTimeout)
}
