// Package gke implements the CloudPlatform interface for Google Kubernetes Engine.
package gke

import (
	"time"

	"github.com/cocoonstack/cocoon-net/platform"
)

const (
	metaBase       = "http://metadata.google.internal/computeMetadata/v1"
	aliasRangeName = "cocoon-pods"
	nic0Name       = "nic0"

	detectionURL     = metaBase + "/instance/zone"
	detectionTimeout = 2 * time.Second
	metadataTimeout  = 5 * time.Second
)

var metadataHeaders = map[string]string{"Metadata-Flavor": "Google"}

var _ platform.CloudPlatform = (*GKE)(nil)

// GKE implements CloudPlatform for Google Kubernetes Engine.
type GKE struct{}

// New constructs a GKE platform handle.
func New() *GKE { return &GKE{} }

func (g *GKE) Name() string { return platform.PlatformGKE }
