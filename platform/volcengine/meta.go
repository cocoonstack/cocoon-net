package volcengine

import (
	"context"
	"fmt"

	"github.com/cocoonstack/cocoon-net/platform"
)

func fetchMeta(ctx context.Context, path string) (string, error) {
	val, err := platform.FetchMetadata(ctx, metadataBase+path, nil, metadataTimeout)
	if err != nil {
		return "", err
	}
	if val == "" {
		return "", fmt.Errorf("%s returned empty", path)
	}
	return val, nil
}
