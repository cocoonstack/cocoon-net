package volcengine

import (
	"context"
	"time"

	"github.com/cocoonstack/cocoon-net/platform"
)

const metadataTimeout = 2 * time.Second

// Detect probes the Volcengine metadata endpoint.
func Detect(ctx context.Context) bool {
	return platform.ProbeMetadata(ctx, metadataBase+"/instance-id", nil, metadataTimeout)
}
