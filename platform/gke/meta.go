package gke

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func fetchMetadata(ctx context.Context) (instance, zone, project, subnet string, err error) {
	client := &http.Client{Timeout: metadataTimeout}

	fetch := func(path string) (string, error) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, metaBase+path, nil)
		if reqErr != nil {
			return "", reqErr
		}
		req.Header.Set("Metadata-Flavor", "Google")
		resp, doErr := client.Do(req)
		if doErr != nil {
			return "", doErr
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("metadata %s: status %d", path, resp.StatusCode)
		}
		b, readErr := io.ReadAll(resp.Body)
		return strings.TrimSpace(string(b)), readErr
	}

	instance, err = fetch("/instance/name")
	if err != nil {
		return "", "", "", "", fmt.Errorf("instance name: %w", err)
	}

	zoneURL, err := fetch("/instance/zone")
	if err != nil {
		return "", "", "", "", fmt.Errorf("zone: %w", err)
	}
	// zoneURL is "projects/PROJECT_NUMBER/zones/ZONE"; the numeric segment is
	// not the project ID gcloud accepts, so the ID is fetched separately below.
	parts := strings.Split(zoneURL, "/")
	zone = parts[len(parts)-1]

	project, err = fetch("/project/project-id")
	if err != nil {
		return "", "", "", "", fmt.Errorf("project-id: %w", err)
	}

	subnetURL, err := fetch("/instance/network-interfaces/0/subnetwork")
	if err != nil {
		return "", "", "", "", fmt.Errorf("subnetwork: %w", err)
	}
	// subnetURL format: "projects/PROJECT/regions/REGION/subnetworks/SUBNET"
	subnetParts := strings.Split(subnetURL, "/")
	subnet = subnetParts[len(subnetParts)-1]

	return instance, zone, project, subnet, nil
}
