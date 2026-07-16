package volcengine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func fetchMeta(ctx context.Context, path string) (string, error) {
	client := &http.Client{Timeout: metadataTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataBase+path, nil)
	if err != nil {
		return "", fmt.Errorf("create request for %s: %w", path, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s: status %d", path, resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	val := strings.TrimSpace(string(b))
	if val == "" {
		return "", fmt.Errorf("%s returned empty", path)
	}
	return val, nil
}
