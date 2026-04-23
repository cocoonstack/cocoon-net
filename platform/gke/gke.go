// Package gke implements the CloudPlatform interface for Google Kubernetes Engine.
//
// TECH DEBT: this package currently drives GCE alias-IP management by shelling
// out to the `gcloud` CLI (see gcloud.go). This is an architectural bridge:
// the subprocess path is opaque to the Go runtime (no typed errors, no retries,
// no auth-refresh hooks) and relies on the operator having `gcloud`
// installed & authenticated.
//
// TODO: migrate to the official GCP Go SDK
// (cloud.google.com/go/compute/apiv1) for instances.UpdateNetworkInterface
// and subnetworks.Patch. This removes the `gcloud` binary dependency and
// surfaces typed error details (quotas, permission issues, propagation
// delays) directly to callers.
package gke

import (
	"time"

	"github.com/cocoonstack/cocoon-net/platform"
)

const (
	metaBase       = "http://metadata.google.internal/computeMetadata/v1"
	aliasRangeName = "cocoon-pods"

	detectionURL     = metaBase + "/instance/zone"
	detectionTimeout = 2 * time.Second
	metadataTimeout  = 5 * time.Second
)

var _ platform.CloudPlatform = (*GKE)(nil)

// GKE implements CloudPlatform for Google Kubernetes Engine.
type GKE struct{}

// New constructs a GKE platform handle.
func New() *GKE { return &GKE{} }

// Name returns the platform identifier.
func (g *GKE) Name() string { return platform.PlatformGKE }
