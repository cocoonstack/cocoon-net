package gke

import (
	"context"
	"fmt"
	"strings"

	"github.com/cocoonstack/cocoon-net/platform"
)

func fetchMetadata(ctx context.Context) (instance, zone, project, subnet string, err error) {
	fetch := func(path string) (string, error) {
		return platform.FetchMetadata(ctx, metaBase+path, metadataHeaders, metadataTimeout)
	}

	instance, err = fetch("/instance/name")
	if err != nil {
		return "", "", "", "", fmt.Errorf("fetch instance name: %w", err)
	}

	zoneURL, err := fetch("/instance/zone")
	if err != nil {
		return "", "", "", "", fmt.Errorf("fetch zone: %w", err)
	}
	// the numeric project segment in zoneURL is not the project ID gcloud wants, so it is fetched separately
	parts := strings.Split(zoneURL, "/")
	zone = parts[len(parts)-1]

	project, err = fetch("/project/project-id")
	if err != nil {
		return "", "", "", "", fmt.Errorf("fetch project id: %w", err)
	}

	subnetURL, err := fetch("/instance/network-interfaces/0/subnetwork")
	if err != nil {
		return "", "", "", "", fmt.Errorf("fetch subnetwork: %w", err)
	}
	// subnetURL format: "projects/PROJECT/regions/REGION/subnetworks/SUBNET"
	subnetParts := strings.Split(subnetURL, "/")
	subnet = subnetParts[len(subnetParts)-1]

	return instance, zone, project, subnet, nil
}
