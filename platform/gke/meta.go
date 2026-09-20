package gke

import (
	"context"
	"fmt"
	"path"

	"github.com/cocoonstack/cocoon-net/platform"
)

func fetchMetadata(ctx context.Context) (instance, zone, project, subnet string, err error) {
	fetch := func(route string) (string, error) {
		return platform.FetchMetadata(ctx, metaBase+route, metadataHeaders, metadataTimeout)
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
	zone = path.Base(zoneURL)

	project, err = fetch("/project/project-id")
	if err != nil {
		return "", "", "", "", fmt.Errorf("fetch project id: %w", err)
	}

	subnetURL, err := fetch("/instance/network-interfaces/0/subnetwork")
	if err != nil {
		return "", "", "", "", fmt.Errorf("fetch subnetwork: %w", err)
	}
	// subnetURL format: "projects/PROJECT/regions/REGION/subnetworks/SUBNET"
	subnet = path.Base(subnetURL)

	return instance, zone, project, subnet, nil
}
