package volcengine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// networkInterface is the JSON shape of a Volcengine ENI.
type networkInterface struct {
	NetworkInterfaceID string `json:"NetworkInterfaceId"`
	Type               string `json:"Type"`
	PrivateIPSets      struct {
		PrivateIPSet []struct {
			Primary          bool   `json:"Primary"`
			PrivateIPAddress string `json:"PrivateIpAddress"`
		} `json:"PrivateIpSet"`
	} `json:"PrivateIpSets"`
}

// fetchMeta fetches a value from the Volcengine instance metadata service.
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
